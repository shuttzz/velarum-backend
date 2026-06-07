package city

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/domain/combat"
	"backend/internal/domain/resource"
)

// SW3 — SAQUE PvP: marcha da SUA cidade até a cidade de outro jogador → no destino, combate de
// saque (combat.ResolvePvP: guarnição + Torre(ataque) + Muralha(HP) do defensor) → vitória rouba o
// EXCEDENTE não-protegido (resource.Raidable), limitado pela CARGA dos sobreviventes → volta com
// loot. Ambos os lados perdem tropas. Sem forecast (névoa). Escudo de novato 4d (cai ao 1º ataque).

const (
	EventRaidArrival = "raid.arrival"
	EventRaidReturn  = "raid.return"
)

const (
	reportTypeRaidPvP   = "raid_pvp"  // atacante: resultado do saque
	reportTypeDefense   = "defense"   // defensor: fui saqueado
	reportTypeIncoming  = "incoming"  // defensor: ATAQUE A CAMINHO (alerta, com névoa)
)

var (
	ErrTargetCityNotFound = errors.New("cidade alvo não encontrada")
	ErrCannotRaidSelf     = errors.New("não dá para saquear a própria cidade")
	ErrDefenderShielded   = errors.New("alvo sob escudo de proteção")
)

type raidEventPayload struct {
	RaidID string `json:"raid_id"`
}

// Raid é a visão de domínio de um saque (ataque a cidade).
type Raid struct {
	ID             string           `json:"id"`
	AttackerCityID string           `json:"attacker_city_id"`
	DefenderCityID string           `json:"defender_city_id"`
	DefenderName   string           `json:"defender_name"`
	Status         string           `json:"status"` // outbound | returning | done
	AttackerWon    *bool            `json:"attacker_won"`
	Troops         map[string]int   `json:"troops"`
	Survivors      map[string]int   `json:"survivors"`
	Loot           resource.Amounts `json:"loot"`
	ArriveAt       time.Time        `json:"arrive_at"`
	ReturnAt       *time.Time       `json:"return_at"`
}

// IncomingRaid é a visão do DEFENSOR de um ataque a caminho — com NÉVOA: sabe quem e quando chega,
// mas NÃO a composição de tropas (isso só com batedor/espionagem).
type IncomingRaid struct {
	AttackerName string    `json:"attacker_name"`
	ArriveAt     time.Time `json:"arrive_at"`
}

// raidReportPvP é o relatório do ATACANTE.
type raidReportPvP struct {
	DefenderName string           `json:"defender_name"`
	Won          bool             `json:"won"`
	Loot         resource.Amounts `json:"loot"`
	Sent         map[string]int   `json:"sent"`
	Losses       map[string]int   `json:"losses"`
}

// defenseReport é o relatório do DEFENSOR (foi saqueado).
type defenseReport struct {
	AttackerName  string           `json:"attacker_name"`
	AttackerWon   bool             `json:"attacker_won"` // o atacante venceu? (você foi saqueado)
	Stolen        resource.Amounts `json:"stolen"`
	DefenderLosses map[string]int  `json:"defender_losses"`
}

// incomingReport alerta o defensor de um ataque a caminho.
type incomingReport struct {
	AttackerName string    `json:"attacker_name"`
	ArriveAt     time.Time `json:"arrive_at"`
}

// StartRaid envia `troops` da guarnição para saquear a cidade `defenderCityID`.
func (s *Service) StartRaid(ctx context.Context, attackerCityID, defenderCityID string, troops map[string]int, now time.Time) (Raid, error) {
	aid, err := db.ParseUUID(attackerCityID)
	if err != nil {
		return Raid{}, err
	}
	did, err := db.ParseUUID(defenderCityID)
	if err != nil {
		return Raid{}, err
	}
	if attackerCityID == defenderCityID {
		return Raid{}, ErrCannotRaidSelf
	}
	total := 0
	for ut, c := range troops {
		if c <= 0 {
			return Raid{}, ErrBadCount
		}
		if _, ok := config.UnitByKey(ut); !ok {
			return Raid{}, ErrUnitUnknown
		}
		total += c
	}
	if total == 0 {
		return Raid{}, ErrBadCount
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Raid{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	attacker, err := q.GetCityForUpdate(ctx, aid)
	if err != nil {
		return Raid{}, fmt.Errorf("lock cidade: %w", err)
	}
	defender, err := q.GetCity(ctx, did)
	if err != nil {
		return Raid{}, ErrTargetCityNotFound
	}
	// Escudo de novato do defensor → não pode ser saqueado.
	defPlayer, err := q.GetPlayer(ctx, defender.PlayerID)
	if err != nil {
		return Raid{}, err
	}
	if defPlayer.ShieldUntil.Valid && now.Before(defPlayer.ShieldUntil.Time) {
		return Raid{}, ErrDefenderShielded
	}

	// Fila de expedições (províncias + nós + saques) limitada por era.
	n, err := activeExpeditions(ctx, q, aid)
	if err != nil {
		return Raid{}, err
	}
	if n >= config.QueuesForEra(int(attacker.Era)) {
		return Raid{}, ErrQueueFull
	}
	// Capacidade de marcha: máx. de tropas por expedição (cresce por era). Ver [[design-combate-marcha]].
	if totalTroops(troops) > config.MarchCapForEra(int(attacker.Era)) {
		return Raid{}, ErrMarchCapacityExceeded
	}

	garrison := map[string]int{}
	rows, err := q.ListCityTroops(ctx, aid)
	if err != nil {
		return Raid{}, err
	}
	for _, t := range rows {
		garrison[t.UnitType] = int(t.Count)
	}
	for ut, c := range troops {
		if garrison[ut] < c {
			return Raid{}, ErrNoTroops
		}
	}
	for ut, c := range troops {
		if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: aid, UnitType: ut, Count: int32(-c)}); err != nil {
			return Raid{}, err
		}
	}
	// 1º ataque derruba o escudo de novato do ATACANTE (não dá pra saquear escondido atrás do escudo).
	if err := q.DropPlayerShield(ctx, attacker.PlayerID); err != nil {
		return Raid{}, err
	}

	dur := time.Duration(config.MarchSecondsBetween(int(attacker.CoordX), int(attacker.CoordY), int(defender.CoordX), int(defender.CoordY))) * time.Second
	arriveAt := now.Add(dur)
	troopsJSON, _ := json.Marshal(troops)
	r, err := q.InsertRaid(ctx, db.InsertRaidParams{
		WorldID: attacker.WorldID, AttackerCityID: aid, DefenderCityID: did, Troops: troopsJSON, DepartAt: now, ArriveAt: arriveAt,
	})
	if err != nil {
		return Raid{}, err
	}
	payload, _ := json.Marshal(raidEventPayload{RaidID: db.UUIDString(r.ID)})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventRaidArrival, FiresAt: arriveAt, Payload: payload}); err != nil {
		return Raid{}, err
	}

	// ALERTA DE INCOMING ao defensor (com névoa: quem + quando, sem tropas).
	atkPlayer, err := q.GetPlayer(ctx, attacker.PlayerID)
	if err != nil {
		return Raid{}, err
	}
	irJSON, _ := json.Marshal(incomingReport{AttackerName: atkPlayer.Username, ArriveAt: arriveAt})
	if _, err := q.InsertReport(ctx, db.InsertReportParams{WorldID: defender.WorldID, PlayerID: defender.PlayerID, Type: reportTypeIncoming, Payload: irJSON}); err != nil {
		return Raid{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Raid{}, fmt.Errorf("commit: %w", err)
	}
	out := raidToDomain(r)
	out.DefenderName = defPlayer.Username
	return out, nil
}

// ResolveRaidArrivalEvent é o handler do scheduler para "raid.arrival".
func (s *Service) ResolveRaidArrivalEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p raidEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.ResolveRaidArrival(ctx, p.RaidID, now)
}

// ResolveRaidArrival resolve o combate de saque no destino. Idempotente.
func (s *Service) ResolveRaidArrival(ctx context.Context, raidID string, now time.Time) error {
	rid, err := db.ParseUUID(raidID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	r, err := q.GetRaidForUpdate(ctx, rid)
	if err != nil {
		return fmt.Errorf("buscar saque: %w", err)
	}
	if r.Status != "outbound" {
		return tx.Commit(ctx)
	}
	defender, err := q.GetCityForUpdate(ctx, r.DefenderCityID)
	if err != nil {
		return err
	}
	attacker, err := q.GetCity(ctx, r.AttackerCityID)
	if err != nil {
		return err
	}

	var troops map[string]int
	_ = json.Unmarshal(r.Troops, &troops)
	atkStacks := buildStacks(troops)

	// Defesa = guarnição + Torre (ataque) + Muralha (HP).
	defTroops := map[string]int{}
	dtRows, err := q.ListCityTroops(ctx, defender.ID)
	if err != nil {
		return err
	}
	for _, t := range dtRows {
		if t.Count > 0 {
			defTroops[t.UnitType] = int(t.Count)
		}
	}
	defStacks := buildStacks(defTroops)
	buildings, err := q.ListCityBuildings(ctx, defender.ID)
	if err != nil {
		return err
	}
	towerAtk, wallHP := cityDefense(buildings)

	out := combat.ResolvePvP(atkStacks, defStacks, towerAtk, wallHP)

	// Aplica baixas do DEFENSOR (sempre — vença ou perca).
	for ut, c := range defTroops {
		killed := c - out.DefenderSurvivors[ut]
		if killed > 0 {
			if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: defender.ID, UnitType: ut, Count: int32(-killed)}); err != nil {
				return err
			}
		}
	}

	loot := resource.Amounts{}
	if out.AttackerWins {
		// Rouba o EXCEDENTE (acima da proteção), limitado pela CARGA dos sobreviventes.
		raidable := stateFromRow(defender).Raidable(now)
		loot = capByCarry(raidable, out.AttackerSurvivors)
		if loot.Matter > 0 || loot.Energy > 0 || loot.Knowledge > 0 {
			cur := stateFromRow(defender).At(now)
			cur.Matter = floorSub(cur.Matter, loot.Matter)
			cur.Energy = floorSub(cur.Energy, loot.Energy)
			cur.Knowledge = floorSub(cur.Knowledge, loot.Knowledge)
			if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
				ID: defender.ID, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
				MatterRate: defender.MatterRate, EnergyRate: defender.EnergyRate, KnowledgeRate: defender.KnowledgeRate, ResourcesUpdatedAt: now,
			}); err != nil {
				return err
			}
		}
	}

	won := out.AttackerWins
	survJSON, _ := json.Marshal(out.AttackerSurvivors)
	lootJSON, _ := json.Marshal(loot)
	returnAt := now.Add(time.Duration(config.MarchSecondsBetween(int(attacker.CoordX), int(attacker.CoordY), int(defender.CoordX), int(defender.CoordY))) * time.Second)
	if err := q.SetRaidResult(ctx, db.SetRaidResultParams{ID: rid, AttackerWon: &won, Survivors: survJSON, Loot: lootJSON, ReturnAt: pgTime(returnAt)}); err != nil {
		return err
	}
	payload, _ := json.Marshal(raidEventPayload{RaidID: raidID})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventRaidReturn, FiresAt: returnAt, Payload: payload}); err != nil {
		return err
	}

	// Relatório do DEFENSOR (fui saqueado / defendi).
	atkPlayer, _ := q.GetPlayer(ctx, attacker.PlayerID)
	defLosses := map[string]int{}
	for ut, c := range defTroops {
		if k := c - out.DefenderSurvivors[ut]; k > 0 {
			defLosses[ut] = k
		}
	}
	drJSON, _ := json.Marshal(defenseReport{AttackerName: atkPlayer.Username, AttackerWon: won, Stolen: loot, DefenderLosses: defLosses})
	if _, err := q.InsertReport(ctx, db.InsertReportParams{WorldID: defender.WorldID, PlayerID: defender.PlayerID, Type: reportTypeDefense, Payload: drJSON}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveRaidReturnEvent é o handler do scheduler para "raid.return".
func (s *Service) ResolveRaidReturnEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p raidEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.ResolveRaidReturn(ctx, p.RaidID, now)
}

// ResolveRaidReturn devolve os sobreviventes à guarnição do atacante, credita o loot e gera o
// relatório do atacante. Idempotente.
func (s *Service) ResolveRaidReturn(ctx context.Context, raidID string, now time.Time) error {
	rid, err := db.ParseUUID(raidID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	r, err := q.GetRaidForUpdate(ctx, rid)
	if err != nil {
		return fmt.Errorf("buscar saque: %w", err)
	}
	if r.Status != "returning" {
		return tx.Commit(ctx)
	}

	var survivors map[string]int
	_ = json.Unmarshal(r.Survivors, &survivors)
	for ut, c := range survivors {
		if c > 0 {
			if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: r.AttackerCityID, UnitType: ut, Count: int32(c)}); err != nil {
				return err
			}
		}
	}
	loot := resource.Amounts{}
	if len(r.Loot) > 0 {
		_ = json.Unmarshal(r.Loot, &loot)
	}
	if loot.Matter > 0 || loot.Energy > 0 || loot.Knowledge > 0 {
		attacker, err := q.GetCityForUpdate(ctx, r.AttackerCityID)
		if err != nil {
			return err
		}
		cur := stateFromRow(attacker).At(now)
		cur.Matter += loot.Matter
		cur.Energy += loot.Energy
		cur.Knowledge += loot.Knowledge
		if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
			ID: r.AttackerCityID, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
			MatterRate: attacker.MatterRate, EnergyRate: attacker.EnergyRate, KnowledgeRate: attacker.KnowledgeRate, ResourcesUpdatedAt: now,
		}); err != nil {
			return err
		}
	}
	if err := q.SetRaidDone(ctx, rid); err != nil {
		return err
	}

	// Relatório do ATACANTE.
	var sent map[string]int
	_ = json.Unmarshal(r.Troops, &sent)
	losses := map[string]int{}
	for ut, c := range sent {
		if k := c - survivors[ut]; k > 0 {
			losses[ut] = k
		}
	}
	won := r.AttackerWon != nil && *r.AttackerWon
	defName := ""
	if dc, err := q.GetCity(ctx, r.DefenderCityID); err == nil {
		if dp, err := q.GetPlayer(ctx, dc.PlayerID); err == nil {
			defName = dp.Username
		}
	}
	rrJSON, _ := json.Marshal(raidReportPvP{DefenderName: defName, Won: won, Loot: loot, Sent: sent, Losses: losses})
	attacker, err := q.GetCity(ctx, r.AttackerCityID)
	if err != nil {
		return err
	}
	if _, err := q.InsertReport(ctx, db.InsertReportParams{WorldID: r.WorldID, PlayerID: attacker.PlayerID, Type: reportTypeRaidPvP, Payload: rrJSON}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- helpers ---

// cityDefense soma o ataque das Torres e o HP das Muralhas da cidade (bônus de defesa no saque).
func cityDefense(buildings []db.CityBuilding) (towerAtk, wallHP int) {
	for _, b := range buildings {
		switch b.BuildingType {
		case config.TowerKey:
			towerAtk += config.TowerAttack(int(b.Level))
		case config.WallKey:
			wallHP += config.WallHP(int(b.Level))
		}
	}
	return towerAtk, wallHP
}

// capByCarry limita o saque ao total que os sobreviventes conseguem CARREGAR (carga = Σ Carry),
// preenchendo gulosamente matéria → energia → conhecimento.
func capByCarry(raidable resource.Amounts, survivors map[string]int) resource.Amounts {
	var carry float64
	for ut, c := range survivors {
		if u, ok := config.UnitByKey(ut); ok {
			carry += float64(u.Carry) * float64(c)
		}
	}
	out := resource.Amounts{}
	take := func(avail float64) float64 {
		t := avail
		if carry < t {
			t = carry
		}
		if t < 0 {
			t = 0
		}
		carry -= t
		return t
	}
	out.Matter = take(raidable.Matter)
	out.Energy = take(raidable.Energy)
	out.Knowledge = take(raidable.Knowledge)
	return out
}

func floorSub(a, b float64) float64 {
	if a-b < 0 {
		return 0
	}
	return a - b
}

func raidToDomain(r db.Raid) Raid {
	dr := Raid{
		ID: db.UUIDString(r.ID), AttackerCityID: db.UUIDString(r.AttackerCityID), DefenderCityID: db.UUIDString(r.DefenderCityID),
		Status: r.Status, AttackerWon: r.AttackerWon, ArriveAt: r.ArriveAt,
	}
	_ = json.Unmarshal(r.Troops, &dr.Troops)
	if len(r.Survivors) > 0 {
		_ = json.Unmarshal(r.Survivors, &dr.Survivors)
	}
	if len(r.Loot) > 0 {
		_ = json.Unmarshal(r.Loot, &dr.Loot)
	}
	if r.ReturnAt.Valid {
		t := r.ReturnAt.Time
		dr.ReturnAt = &t
	}
	return dr
}
