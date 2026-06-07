package city

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/domain/combat"
	"backend/internal/domain/resource"
)

// provinceSeed deriva uma seed determinística do jogador para o layout das províncias no mapa.
func provinceSeed(playerID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(playerID))
	return h.Sum64()
}

// Erros de negócio do mapa/marcha (mapeáveis para HTTP 4xx).
var (
	ErrProvinceNotFound  = errors.New("província não encontrada")
	ErrProvinceConquered = errors.New("província já conquistada")
	ErrNoTroops          = errors.New("tropas insuficientes na guarnição")
	// ErrMarchCapacityExceeded: a expedição leva mais tropas que a capacidade de marcha da era.
	ErrMarchCapacityExceeded = errors.New("capacidade de marcha excedida")
)

// totalTroops soma o nº de unidades de um destacamento (tropas enviadas numa expedição).
func totalTroops(troops map[string]int) int {
	n := 0
	for _, c := range troops {
		n += c
	}
	return n
}

// Eventos agendados do mapa vivo.
const (
	EventTroopArrival = "troop.arrival"
	EventTroopReturn  = "troop.return"
)

type marchEventPayload struct {
	MarchID string `json:"march_id"`
}

// Province é a visão de domínio de uma província PvE.
type Province struct {
	ID        string           `json:"id"`
	NameKey   string           `json:"name_key"`
	Q         int              `json:"q"`
	R         int              `json:"r"`
	Ring      int              `json:"ring"`
	DefAttack int              `json:"def_attack"`
	DefHP     int              `json:"def_hp"`
	Reward    resource.Amounts `json:"reward"`
	Deposit   resource.Amounts `json:"deposit"` // renda passiva/hora se mantida (vem do config)
	Status    string           `json:"status"`
}

// March é a visão de domínio de uma marcha (exército a caminho/voltando).
type March struct {
	ID          string         `json:"id"`
	ProvinceID  string         `json:"province_id"`
	Status      string         `json:"status"` // outbound | returning | done
	AttackerWon *bool          `json:"attacker_won"`
	Troops      map[string]int `json:"troops"`
	Survivors   map[string]int `json:"survivors"`
	ArriveAt    time.Time      `json:"arrive_at"`
	ReturnAt    *time.Time     `json:"return_at"`
}

// ListProvinces devolve as províncias do jogador (gerando o anel 1 na primeira vez).
func (s *Service) ListProvinces(ctx context.Context, cityID string, now time.Time) ([]Province, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return nil, err
	}
	cityRow, err := s.q.GetCity(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureProvinces(ctx, cityRow); err != nil {
		return nil, err
	}
	rows, err := s.q.ListPlayerProvinces(ctx, cityRow.PlayerID)
	if err != nil {
		return nil, err
	}
	out := make([]Province, 0, len(rows))
	for _, p := range rows {
		out = append(out, provinceToDomain(p))
	}
	return out, nil
}

// ensureProvinces gera as províncias da Era 1 para o jogador caso ainda não existam.
// Serializa pelo lock da cidade (uma cidade por jogador) para evitar geração concorrente.
func (s *Service) ensureProvinces(ctx context.Context, cityRow db.City) error {
	n, err := s.q.CountPlayerProvinces(ctx, cityRow.PlayerID)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	if _, err := q.GetCityForUpdate(ctx, cityRow.ID); err != nil {
		return err
	}
	again, err := q.CountPlayerProvinces(ctx, cityRow.PlayerID)
	if err != nil {
		return err
	}
	if again > 0 {
		return tx.Commit(ctx) // outra requisição gerou
	}
	coords := config.PlaceEra1Provinces(provinceSeed(db.UUIDString(cityRow.PlayerID))) // layout espalhado por seed
	for i, t := range config.Era1Provinces {
		defAtk, defHP := t.DefenseAggregate() // agregado da composição de tropas (auto-resolve/exibição)
		c := coords[i]
		if _, err := q.InsertProvince(ctx, db.InsertProvinceParams{
			WorldID: cityRow.WorldID, PlayerID: cityRow.PlayerID, NameKey: t.NameKey,
			Q: int32(c.Q), R: int32(c.R), Ring: int16(t.Ring), DefAttack: int32(defAtk), DefHp: int32(defHP),
			RewardMatter: t.Reward.Matter, RewardEnergy: t.Reward.Energy, RewardKnowledge: t.Reward.Knowledge,
		}); err != nil {
			return fmt.Errorf("gerar província: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// StartMarch envia `troops` da guarnição contra uma província (marcha de ida = timer).
func (s *Service) StartMarch(ctx context.Context, cityID, provinceID string, troops map[string]int, now time.Time) (March, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return March{}, err
	}
	pid, err := db.ParseUUID(provinceID)
	if err != nil {
		return March{}, err
	}
	total := 0
	for ut, c := range troops {
		if c <= 0 {
			return March{}, ErrBadCount
		}
		if _, ok := config.UnitByKey(ut); !ok {
			return March{}, ErrUnitUnknown
		}
		total += c
	}
	if total == 0 {
		return March{}, ErrBadCount
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return March{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return March{}, fmt.Errorf("lock cidade: %w", err)
	}
	prov, err := q.GetProvinceForUpdate(ctx, pid)
	if err != nil {
		return March{}, ErrProvinceNotFound
	}
	if !sameUUID(prov.PlayerID, cityRow.PlayerID) {
		return March{}, ErrProvinceNotFound
	}
	if prov.Status == "conquered" {
		return March{}, ErrProvinceConquered
	}

	// Fila de expedições (províncias + nós do mundo) limitada por era — todas na mesma lane.
	n, err := activeExpeditions(ctx, q, id)
	if err != nil {
		return March{}, err
	}
	if n >= config.QueuesForEra(int(cityRow.Era)) {
		return March{}, ErrQueueFull
	}
	// Capacidade de marcha: máx. de tropas por expedição (cresce por era). Defesa usa tudo em casa;
	// só a ofensa é limitada. Ver [[design-combate-marcha]].
	if totalTroops(troops) > config.MarchCapForEra(int(cityRow.Era)) {
		return March{}, ErrMarchCapacityExceeded
	}

	garrison := map[string]int{}
	rows, err := q.ListCityTroops(ctx, id)
	if err != nil {
		return March{}, err
	}
	for _, t := range rows {
		garrison[t.UnitType] = int(t.Count)
	}
	for ut, c := range troops {
		if garrison[ut] < c {
			return March{}, ErrNoTroops
		}
	}
	// Retira as tropas da guarnição (vão na marcha).
	for ut, c := range troops {
		if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: id, UnitType: ut, Count: int32(-c)}); err != nil {
			return March{}, err
		}
	}

	dur := time.Duration(config.MarchSecondsTo(int(prov.Q), int(prov.R))) * time.Second
	arriveAt := now.Add(dur)
	troopsJSON, _ := json.Marshal(troops)
	m, err := q.InsertMarch(ctx, db.InsertMarchParams{
		WorldID: cityRow.WorldID, CityID: id, ProvinceID: pid, Troops: troopsJSON, DepartAt: now, ArriveAt: arriveAt,
	})
	if err != nil {
		return March{}, err
	}
	payload, _ := json.Marshal(marchEventPayload{MarchID: db.UUIDString(m.ID)})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventTroopArrival, FiresAt: arriveAt, Payload: payload}); err != nil {
		return March{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return March{}, fmt.Errorf("commit: %w", err)
	}
	return marchToDomain(m), nil
}

// ResolveArrivalEvent é o handler do scheduler para "troop.arrival".
func (s *Service) ResolveArrivalEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p marchEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.ResolveArrival(ctx, p.MarchID, now)
}

// ResolveArrival resolve o combate na chegada (auto-resolve), aplica conquista/recompensa e
// agenda a volta. Idempotente.
func (s *Service) ResolveArrival(ctx context.Context, marchID string, now time.Time) error {
	mid, err := db.ParseUUID(marchID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	m, err := q.GetMarchForUpdate(ctx, mid)
	if err != nil {
		return fmt.Errorf("buscar marcha: %w", err)
	}
	if m.Status != "outbound" {
		return tx.Commit(ctx) // já processado
	}
	prov, err := q.GetProvinceForUpdate(ctx, m.ProvinceID)
	if err != nil {
		return err
	}

	var troops map[string]int
	_ = json.Unmarshal(m.Troops, &troops)
	stacks := make([]combat.Stack, 0, len(troops))
	for ut, c := range troops {
		def, ok := config.UnitByKey(ut)
		if !ok {
			continue
		}
		stacks = append(stacks, combat.Stack{Key: ut, Attack: def.Attack, HP: def.HP, Count: c})
	}
	out := combat.AutoResolve(stacks, combat.Defender{Attack: int(prov.DefAttack), HP: int(prov.DefHp)})

	if out.AttackerWins && prov.Status != "conquered" {
		if err := q.SetProvinceConquered(ctx, db.SetProvinceConqueredParams{ID: prov.ID, ConqueredAt: pgTime(now)}); err != nil {
			return err
		}
		// Recompensa: soma ao estoque atual da cidade (taxa inalterada).
		cityRow, err := q.GetCityForUpdate(ctx, m.CityID)
		if err != nil {
			return err
		}
		cur := stateFromRow(cityRow).At(now)
		cur.Matter += prov.RewardMatter
		cur.Energy += prov.RewardEnergy
		cur.Knowledge += prov.RewardKnowledge
		if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
			ID: m.CityID, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
			MatterRate: cityRow.MatterRate, EnergyRate: cityRow.EnergyRate, KnowledgeRate: cityRow.KnowledgeRate, ResourcesUpdatedAt: now,
		}); err != nil {
			return err
		}
		// Recalcula a produção: a província conquistada passa a render seu depósito passivo.
		if err := recomputeProduction(ctx, q, m.CityID, now); err != nil {
			return err
		}
	}

	won := out.AttackerWins
	survJSON, _ := json.Marshal(out.Survivors)
	returnAt := now.Add(time.Duration(config.MarchSecondsTo(int(prov.Q), int(prov.R))) * time.Second)
	if err := q.SetMarchResult(ctx, db.SetMarchResultParams{ID: mid, AttackerWon: &won, Survivors: survJSON, ReturnAt: pgTime(returnAt)}); err != nil {
		return err
	}

	// Relatório de batalha (caixa de entrada do jogador).
	reward := resource.Amounts{}
	if won {
		reward = resource.Amounts{Matter: prov.RewardMatter, Energy: prov.RewardEnergy, Knowledge: prov.RewardKnowledge}
	}
	brJSON, _ := json.Marshal(battleReport{
		ProvinceID: db.UUIDString(prov.ID), ProvinceNameKey: prov.NameKey, AttackerWon: won,
		Sent: troops, Losses: out.Losses, Survivors: out.Survivors, Reward: reward,
	})
	if _, err := q.InsertReport(ctx, db.InsertReportParams{WorldID: prov.WorldID, PlayerID: prov.PlayerID, Type: reportTypeBattle, Payload: brJSON}); err != nil {
		return err
	}
	payload, _ := json.Marshal(marchEventPayload{MarchID: marchID})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventTroopReturn, FiresAt: returnAt, Payload: payload}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveReturnEvent é o handler do scheduler para "troop.return".
func (s *Service) ResolveReturnEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p marchEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.ResolveReturn(ctx, p.MarchID, now)
}

// ResolveReturn devolve os sobreviventes à guarnição e encerra a marcha. Idempotente.
func (s *Service) ResolveReturn(ctx context.Context, marchID string, now time.Time) error {
	mid, err := db.ParseUUID(marchID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	m, err := q.GetMarchForUpdate(ctx, mid)
	if err != nil {
		return fmt.Errorf("buscar marcha: %w", err)
	}
	if m.Status != "returning" {
		return tx.Commit(ctx) // já processado
	}
	var survivors map[string]int
	_ = json.Unmarshal(m.Survivors, &survivors)
	for ut, c := range survivors {
		if c > 0 {
			if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: m.CityID, UnitType: ut, Count: int32(c)}); err != nil {
				return err
			}
		}
	}
	if err := q.SetMarchDone(ctx, mid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func provinceToDomain(p db.Province) Province {
	dep := resource.Amounts{}
	if tpl, ok := config.ProvinceByKey(p.NameKey); ok {
		dep = tpl.Deposit
	}
	return Province{
		ID: db.UUIDString(p.ID), NameKey: p.NameKey, Q: int(p.Q), R: int(p.R), Ring: int(p.Ring),
		DefAttack: int(p.DefAttack), DefHP: int(p.DefHp),
		Reward:  resource.Amounts{Matter: p.RewardMatter, Energy: p.RewardEnergy, Knowledge: p.RewardKnowledge},
		Deposit: dep,
		Status:  p.Status,
	}
}

func marchToDomain(m db.March) March {
	dm := March{
		ID: db.UUIDString(m.ID), ProvinceID: db.UUIDString(m.ProvinceID), Status: m.Status,
		AttackerWon: m.AttackerWon, ArriveAt: m.ArriveAt,
	}
	_ = json.Unmarshal(m.Troops, &dm.Troops)
	if len(m.Survivors) > 0 {
		_ = json.Unmarshal(m.Survivors, &dm.Survivors)
	}
	if m.ReturnAt.Valid {
		t := m.ReturnAt.Time
		dm.ReturnAt = &t
	}
	return dm
}

func pgTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
