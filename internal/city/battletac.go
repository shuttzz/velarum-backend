package city

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/domain/battle"
	"backend/internal/domain/resource"
)

// Erros de negócio da batalha tática.
var (
	ErrBattleActive   = errors.New("já há uma batalha em andamento")
	ErrBattleNotFound = errors.New("batalha não encontrada")
	ErrInvalidAction  = errors.New("ação inválida na batalha")
)

const (
	battleW         = 6
	battleH         = 6
	battleMaxRounds = 12
	defenderHpPer   = 30
)

// BattleView é o que a API expõe: id, província, status e o estado completo (renderizado no cliente).
type BattleView struct {
	ID         string         `json:"id"`
	ProvinceID string         `json:"province_id"`
	Status     string         `json:"status"`
	State      *battle.Battle `json:"state"`
}

func toBattleView(row db.Battle, b *battle.Battle) BattleView {
	return BattleView{ID: db.UUIDString(row.ID), ProvinceID: db.UUIDString(row.ProvinceID), Status: row.Status, State: b}
}

// buildBattle monta o estado inicial: atacante (tropas enviadas) na coluna 0; defensor (derivado
// da província) na coluna oposta. Determinístico (sem aleatoriedade).
func buildBattle(troops map[string]int, prov db.Province) *battle.Battle {
	units := make([]*battle.Unit, 0, len(troops)+1)
	r := 1
	for _, def := range config.Era1Units { // ordem determinística
		c := troops[def.Key]
		if c <= 0 {
			continue
		}
		units = append(units, &battle.Unit{
			ID: "a:" + def.Key, Owner: battle.Attacker, Key: def.Key,
			Hp: c * def.HP, HpPer: def.HP, Attack: def.Attack, Defense: def.Defense,
			Move: def.Move, Range: def.Range, Pos: battle.Hex{Q: 0, R: r},
		})
		r += 2
		if r >= battleH {
			r = battleH - 1
		}
	}

	defHp := int(prov.DefHp)
	count := (defHp + defenderHpPer - 1) / defenderHpPer
	if count < 1 {
		count = 1
	}
	atk := int(prov.DefAttack) / count
	if atk < 1 {
		atk = 1
	}
	units = append(units, &battle.Unit{
		ID: "d:0", Owner: battle.Defender, Key: "guarda",
		Hp: defHp, HpPer: defenderHpPer, Attack: atk, Defense: 2, Move: 1, Range: 1,
		Pos: battle.Hex{Q: battleW - 1, R: battleH / 2},
	})

	occupied := make([]battle.Hex, 0, len(units))
	for _, u := range units {
		occupied = append(occupied, u.Pos)
	}
	tiles := tilesForProvince(db.UUIDString(prov.ID), occupied)
	return &battle.Battle{W: battleW, H: battleH, Turn: battle.Attacker, MaxRounds: battleMaxRounds, Acted: map[string]bool{}, Units: units, Tiles: tiles}
}

// Composição fixa dos tiles de Lacuna (2 de cada tipo); só as POSIÇÕES variam por província.
var battleTileTypes = []battle.TileType{
	battle.TileCover, battle.TileCover,
	battle.TileHazard, battle.TileHazard,
	battle.TileWarp, battle.TileWarp,
}

// tilesForProvince gera o layout de Lacuna de forma DETERMINÍSTICA a partir do id da província
// (seed = hash do UUID): mesma província → mesmo layout, sempre reproduzível/auditável (casa com
// o "seed persistido" do GDD §9), mas cada província tem um layout próprio e espalhado. As casas
// ficam nas colunas internas (1..W-2), evitando os spawns (atacante em q=0, defensor em q=W-1), e
// `occupied` (posições de unidades) é excluído — NENHUM tile nasce sob uma unidade (em especial a
// Fenda, que causa dano).
func tilesForProvince(provID string, occupied []battle.Hex) []battle.Tile {
	h := fnv.New64a()
	_, _ = h.Write([]byte(provID))
	rng := rand.New(rand.NewSource(int64(h.Sum64()))) //nolint:gosec // seed determinística, não-cripto

	taken := make(map[battle.Hex]bool, len(occupied))
	for _, p := range occupied {
		taken[p] = true
	}
	cand := make([]battle.Hex, 0, (battleW-2)*battleH)
	for q := 1; q < battleW-1; q++ {
		for r := 0; r < battleH; r++ {
			if hx := (battle.Hex{Q: q, R: r}); !taken[hx] {
				cand = append(cand, hx)
			}
		}
	}
	rng.Shuffle(len(cand), func(i, j int) { cand[i], cand[j] = cand[j], cand[i] })

	n := len(battleTileTypes)
	if n > len(cand) {
		n = len(cand)
	}
	tiles := make([]battle.Tile, n)
	for i := 0; i < n; i++ {
		tiles[i] = battle.Tile{Pos: cand[i], Type: battleTileTypes[i]}
	}
	return tiles
}

// StartBattle inicia uma batalha tática instanciada contra a província, comprometendo `troops`
// da guarnição. Só uma batalha ativa por cidade.
func (s *Service) StartBattle(ctx context.Context, cityID, provinceID string, troops map[string]int, now time.Time) (BattleView, error) {
	total := 0
	for ut, c := range troops {
		if c <= 0 {
			return BattleView{}, ErrBadCount
		}
		if _, ok := config.UnitByKey(ut); !ok {
			return BattleView{}, ErrUnitUnknown
		}
		total += c
	}
	if total == 0 {
		return BattleView{}, ErrBadCount
	}
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return BattleView{}, err
	}
	pid, err := db.ParseUUID(provinceID)
	if err != nil {
		return BattleView{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BattleView{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.GetActiveBattle(ctx, id); err == nil {
		return BattleView{}, ErrBattleActive
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return BattleView{}, err
	}

	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return BattleView{}, err
	}
	prov, err := q.GetProvinceForUpdate(ctx, pid)
	if err != nil {
		return BattleView{}, ErrProvinceNotFound
	}
	if !sameUUID(prov.PlayerID, cityRow.PlayerID) {
		return BattleView{}, ErrProvinceNotFound
	}
	if prov.Status == "conquered" {
		return BattleView{}, ErrProvinceConquered
	}

	garrison := map[string]int{}
	rows, err := q.ListCityTroops(ctx, id)
	if err != nil {
		return BattleView{}, err
	}
	for _, t := range rows {
		garrison[t.UnitType] = int(t.Count)
	}
	for ut, c := range troops {
		if garrison[ut] < c {
			return BattleView{}, ErrNoTroops
		}
	}
	for ut, c := range troops {
		if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: id, UnitType: ut, Count: int32(-c)}); err != nil {
			return BattleView{}, err
		}
	}

	b := buildBattle(troops, prov)
	stateJSON, _ := json.Marshal(b)
	sentJSON, _ := json.Marshal(troops)
	row, err := q.InsertBattle(ctx, db.InsertBattleParams{
		WorldID: cityRow.WorldID, PlayerID: cityRow.PlayerID, CityID: id, ProvinceID: pid, State: stateJSON, Sent: sentJSON,
	})
	if err != nil {
		return BattleView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BattleView{}, fmt.Errorf("commit: %w", err)
	}
	return toBattleView(row, b), nil
}

// GetBattle devolve o estado de uma batalha (para renderizar/retomar).
func (s *Service) GetBattle(ctx context.Context, cityID, battleID string) (BattleView, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return BattleView{}, err
	}
	bid, err := db.ParseUUID(battleID)
	if err != nil {
		return BattleView{}, err
	}
	row, err := s.q.GetBattle(ctx, bid)
	if err != nil {
		return BattleView{}, ErrBattleNotFound
	}
	if !sameUUID(row.CityID, id) {
		return BattleView{}, ErrBattleNotFound
	}
	var b battle.Battle
	if err := json.Unmarshal(row.State, &b); err != nil {
		return BattleView{}, err
	}
	return toBattleView(row, &b), nil
}

// BattleAct aplica uma ação do jogador (mover/atacar) numa unidade do atacante.
func (s *Service) BattleAct(ctx context.Context, cityID, battleID, unitID string, moveTo *battle.Hex, targetID string, now time.Time) (BattleView, error) {
	return s.mutateBattle(ctx, cityID, battleID, now, func(b *battle.Battle) error {
		return b.Act(unitID, moveTo, targetID)
	})
}

// BattleEndTurn encerra o turno do jogador e roda o turno da IA (defensor).
func (s *Service) BattleEndTurn(ctx context.Context, cityID, battleID string, now time.Time) (BattleView, error) {
	return s.mutateBattle(ctx, cityID, battleID, now, func(b *battle.Battle) error {
		b.EndTurn()
		b.AITurn()
		return nil
	})
}

// mutateBattle carrega a batalha (lock), aplica `apply`, resolve se acabou e persiste.
func (s *Service) mutateBattle(ctx context.Context, cityID, battleID string, now time.Time, apply func(*battle.Battle) error) (BattleView, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return BattleView{}, err
	}
	bid, err := db.ParseUUID(battleID)
	if err != nil {
		return BattleView{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BattleView{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	row, err := q.GetBattleForUpdate(ctx, bid)
	if err != nil {
		return BattleView{}, ErrBattleNotFound
	}
	if !sameUUID(row.CityID, id) {
		return BattleView{}, ErrBattleNotFound
	}
	var b battle.Battle
	if err := json.Unmarshal(row.State, &b); err != nil {
		return BattleView{}, err
	}
	if row.Status != "active" {
		return toBattleView(row, &b), nil // já resolvida — no-op
	}
	if err := apply(&b); err != nil {
		return BattleView{}, fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}

	status := "active"
	if b.Over {
		if err := s.resolveBattle(ctx, q, row, &b, now); err != nil {
			return BattleView{}, err
		}
		status = "resolved"
	}
	stateJSON, _ := json.Marshal(&b)
	if err := q.UpdateBattleState(ctx, db.UpdateBattleStateParams{ID: bid, State: stateJSON, Status: status}); err != nil {
		return BattleView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BattleView{}, fmt.Errorf("commit: %w", err)
	}
	row.Status = status
	return toBattleView(row, &b), nil
}

// resolveBattle aplica o resultado: sobreviventes voltam à guarnição; vitória conquista a
// província + recompensa; gera relatório de batalha.
func (s *Service) resolveBattle(ctx context.Context, q *db.Queries, row db.Battle, b *battle.Battle, now time.Time) error {
	survivors := map[string]int{}
	for _, u := range b.Units {
		if u.Owner == battle.Attacker && u.Count() > 0 {
			survivors[u.Key] += u.Count()
			if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: row.CityID, UnitType: u.Key, Count: int32(u.Count())}); err != nil {
				return err
			}
		}
	}

	prov, err := q.GetProvinceForUpdate(ctx, row.ProvinceID)
	if err != nil {
		return err
	}
	won := b.Winner == battle.Attacker
	reward := resource.Amounts{}
	if won && prov.Status != "conquered" {
		if err := q.SetProvinceConquered(ctx, db.SetProvinceConqueredParams{ID: prov.ID, ConqueredAt: pgTime(now)}); err != nil {
			return err
		}
		reward = resource.Amounts{Matter: prov.RewardMatter, Energy: prov.RewardEnergy, Knowledge: prov.RewardKnowledge}
		cityRow, err := q.GetCityForUpdate(ctx, row.CityID)
		if err != nil {
			return err
		}
		cur := stateFromRow(cityRow).At(now)
		cur.Matter += reward.Matter
		cur.Energy += reward.Energy
		cur.Knowledge += reward.Knowledge
		if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
			ID: row.CityID, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
			MatterRate: cityRow.MatterRate, EnergyRate: cityRow.EnergyRate, KnowledgeRate: cityRow.KnowledgeRate, ResourcesUpdatedAt: now,
		}); err != nil {
			return err
		}
	}

	var sent map[string]int
	_ = json.Unmarshal(row.Sent, &sent)
	losses := map[string]int{}
	for k, c := range sent {
		losses[k] = c - survivors[k]
	}
	brJSON, _ := json.Marshal(battleReport{
		ProvinceID: db.UUIDString(prov.ID), ProvinceNameKey: prov.NameKey, AttackerWon: won,
		Sent: sent, Losses: losses, Survivors: survivors, Reward: reward,
	})
	_, err = q.InsertReport(ctx, db.InsertReportParams{WorldID: prov.WorldID, PlayerID: prov.PlayerID, Type: reportTypeBattle, Payload: brJSON})
	return err
}
