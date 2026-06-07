package city

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/domain/resource"
)

// SW3 — ESPIONAGEM. Batedor: unidade NÃO-combatente treinada na Toca dos Batedores (contagem
// própria + fila de treino), enviada em missões de scout numa LANE SEPARADA da militar. Revela um
// SNAPSHOT do alvo (guarnição, defesa, recursos saqueáveis). O defensor é AVISADO (batedor a
// caminho). v1: sem contra-intel — o batedor sempre revela e sempre volta. (Item de confusão = futuro.)

const (
	EventScoutComplete = "scout.complete"
	EventScoutArrival  = "scout.arrival"
	EventScoutReturn   = "scout.return"
)

const (
	reportTypeScout         = "scout"          // relatório de espionagem (intel) ao atacante
	reportTypeIncomingScout = "incoming_scout" // alerta ao defensor (batedor a caminho)
)

var (
	ErrNoScoutHouse    = errors.New("construa a Toca dos Batedores")
	ErrNoScouts        = errors.New("nenhum batedor disponível")
	ErrCannotScoutAlly = errors.New("não dá para espionar um aliado")
)

type scoutQueuePayload struct {
	QueueID string `json:"queue_id"`
}
type scoutMissionPayload struct {
	MissionID string `json:"mission_id"`
}

// ScoutQueued é um lote de batedores em treino.
type ScoutQueued struct {
	ID       string    `json:"id"`
	Count    int       `json:"count"`
	FinishAt time.Time `json:"finish_at"`
}

// ScoutIntel é o snapshot revelado por um batedor.
type ScoutIntel struct {
	Garrison   map[string]int   `json:"garrison"`
	WallLevel  int              `json:"wall_level"`
	TowerLevel int              `json:"tower_level"`
	Raidable   resource.Amounts `json:"raidable"`
}

// ScoutMission é a visão de domínio de uma missão de espionagem.
type ScoutMission struct {
	ID           string      `json:"id"`
	TargetCityID string      `json:"target_city_id"`
	TargetName   string      `json:"target_name"`
	Status       string      `json:"status"` // outbound | returning | done
	Intel        *ScoutIntel `json:"intel"`
	ArriveAt     time.Time   `json:"arrive_at"`
	ReturnAt     *time.Time  `json:"return_at"`
}

// TrainScout treina `count` batedores na Toca dos Batedores (debita custo, agenda conclusão).
func (s *Service) TrainScout(ctx context.Context, cityID string, count int, now time.Time) (ScoutQueued, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return ScoutQueued{}, err
	}
	if count <= 0 {
		return ScoutQueued{}, ErrBadCount
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ScoutQueued{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return ScoutQueued{}, err
	}
	buildings, err := q.ListCityBuildings(ctx, id)
	if err != nil {
		return ScoutQueued{}, err
	}
	if !hasBuilding(buildings, config.ScoutHouseKey) {
		return ScoutQueued{}, ErrNoScoutHouse
	}
	cost := resource.Amounts{Matter: config.ScoutCost.Matter * float64(count), Energy: config.ScoutCost.Energy * float64(count), Knowledge: config.ScoutCost.Knowledge * float64(count)}
	cur := stateFromRow(cityRow).At(now)
	if cur.Matter < cost.Matter || cur.Energy < cost.Energy || cur.Knowledge < cost.Knowledge {
		return ScoutQueued{}, ErrInsufficient
	}
	cur.Matter -= cost.Matter
	cur.Energy -= cost.Energy
	cur.Knowledge -= cost.Knowledge
	if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID: id, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
		MatterRate: cityRow.MatterRate, EnergyRate: cityRow.EnergyRate, KnowledgeRate: cityRow.KnowledgeRate, ResourcesUpdatedAt: now,
	}); err != nil {
		return ScoutQueued{}, err
	}
	finishAt := now.Add(time.Duration(config.ScoutTrainSeconds*count) * time.Second)
	row, err := q.InsertScoutQueue(ctx, db.InsertScoutQueueParams{CityID: id, Count: int32(count), StartedAt: now, FinishAt: finishAt})
	if err != nil {
		return ScoutQueued{}, err
	}
	payload, _ := json.Marshal(scoutQueuePayload{QueueID: db.UUIDString(row.ID)})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventScoutComplete, FiresAt: finishAt, Payload: payload}); err != nil {
		return ScoutQueued{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScoutQueued{}, fmt.Errorf("commit: %w", err)
	}
	return ScoutQueued{ID: db.UUIDString(row.ID), Count: count, FinishAt: finishAt}, nil
}

// CompleteScoutEvent é o handler do scheduler para "scout.complete".
func (s *Service) CompleteScoutEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p scoutQueuePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	qid, err := db.ParseUUID(p.QueueID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	row, err := q.GetScoutQueueForUpdate(ctx, qid)
	if err != nil {
		return fmt.Errorf("buscar treino: %w", err)
	}
	if row.Status != "pending" {
		return tx.Commit(ctx)
	}
	if _, err := q.CompleteScoutQueue(ctx, qid); err != nil {
		return err
	}
	if err := q.AddCityScouts(ctx, db.AddCityScoutsParams{ID: row.CityID, Scouts: row.Count}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SendScout envia 1 batedor para espionar `targetCityID` (lane separada da militar).
func (s *Service) SendScout(ctx context.Context, attackerCityID, targetCityID string, now time.Time) (ScoutMission, error) {
	aid, err := db.ParseUUID(attackerCityID)
	if err != nil {
		return ScoutMission{}, err
	}
	tid, err := db.ParseUUID(targetCityID)
	if err != nil {
		return ScoutMission{}, err
	}
	if attackerCityID == targetCityID {
		return ScoutMission{}, ErrCannotRaidSelf
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ScoutMission{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	attacker, err := q.GetCityForUpdate(ctx, aid)
	if err != nil {
		return ScoutMission{}, err
	}
	buildings, err := q.ListCityBuildings(ctx, aid)
	if err != nil {
		return ScoutMission{}, err
	}
	if !hasBuilding(buildings, config.ScoutHouseKey) {
		return ScoutMission{}, ErrNoScoutHouse
	}
	if attacker.Scouts < 1 {
		return ScoutMission{}, ErrNoScouts
	}
	target, err := q.GetCity(ctx, tid)
	if err != nil {
		return ScoutMission{}, ErrTargetCityNotFound
	}
	// Aliados não se espionam (fatia B das alianças).
	if ally, err := sameAlliance(ctx, q, attacker.PlayerID, target.PlayerID); err != nil {
		return ScoutMission{}, err
	} else if ally {
		return ScoutMission{}, ErrCannotScoutAlly
	}
	if err := q.AddCityScouts(ctx, db.AddCityScoutsParams{ID: aid, Scouts: -1}); err != nil { // batedor sai
		return ScoutMission{}, err
	}
	// Velocidade de marcha do batedor melhora com o nível da Toca dos Batedores.
	scoutLvl := buildingLevel(buildings, config.ScoutHouseKey)
	dur := time.Duration(config.ScoutMarchSeconds(scoutLvl, int(attacker.CoordX), int(attacker.CoordY), int(target.CoordX), int(target.CoordY))) * time.Second
	arriveAt := now.Add(dur)
	m, err := q.InsertScoutMission(ctx, db.InsertScoutMissionParams{
		WorldID: attacker.WorldID, AttackerCityID: aid, TargetCityID: tid, DepartAt: now, ArriveAt: arriveAt,
	})
	if err != nil {
		return ScoutMission{}, err
	}
	payload, _ := json.Marshal(scoutMissionPayload{MissionID: db.UUIDString(m.ID)})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventScoutArrival, FiresAt: arriveAt, Payload: payload}); err != nil {
		return ScoutMission{}, err
	}
	// Alerta ao defensor: batedor a caminho (névoa — só sabe que está sendo observado).
	atkPlayer, err := q.GetPlayer(ctx, attacker.PlayerID)
	if err != nil {
		return ScoutMission{}, err
	}
	irJSON, _ := json.Marshal(incomingReport{AttackerName: atkPlayer.Username, ArriveAt: arriveAt})
	aaJSON, _ := json.Marshal(allyAlert{AllyName: target.Name, CoordX: int(target.CoordX), CoordY: int(target.CoordY), Kind: "scout"})
	if err := notifyAlliance(ctx, q, target.WorldID, target.PlayerID, reportTypeAllyAlert, aaJSON); err != nil {
		return ScoutMission{}, err
	}
	if _, err := q.InsertReport(ctx, db.InsertReportParams{WorldID: target.WorldID, PlayerID: target.PlayerID, Type: reportTypeIncomingScout, Payload: irJSON}); err != nil {
		return ScoutMission{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScoutMission{}, fmt.Errorf("commit: %w", err)
	}
	out := scoutMissionToDomain(m)
	if tp, err := s.q.GetPlayer(ctx, target.PlayerID); err == nil {
		out.TargetName = tp.Username
	}
	return out, nil
}

// ResolveScoutArrivalEvent é o handler do scheduler para "scout.arrival".
func (s *Service) ResolveScoutArrivalEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p scoutMissionPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	mid, err := db.ParseUUID(p.MissionID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	m, err := q.GetScoutMissionForUpdate(ctx, mid)
	if err != nil {
		return fmt.Errorf("buscar missão: %w", err)
	}
	if m.Status != "outbound" {
		return tx.Commit(ctx)
	}
	target, err := q.GetCity(ctx, m.TargetCityID)
	if err != nil {
		return err
	}
	attacker, err := q.GetCity(ctx, m.AttackerCityID)
	if err != nil {
		return err
	}
	// SNAPSHOT do alvo: guarnição + defesa (níveis muralha/torre) + saqueável.
	intel := ScoutIntel{Garrison: map[string]int{}}
	troops, err := q.ListCityTroops(ctx, target.ID)
	if err != nil {
		return err
	}
	for _, t := range troops {
		if t.Count > 0 {
			intel.Garrison[t.UnitType] = int(t.Count)
		}
	}
	tbuildings, err := q.ListCityBuildings(ctx, target.ID)
	if err != nil {
		return err
	}
	intel.WallLevel = buildingLevel(tbuildings, config.WallKey)
	intel.TowerLevel = buildingLevel(tbuildings, config.TowerKey)
	intel.Raidable = stateFromRow(target).Raidable(now)

	intelJSON, _ := json.Marshal(intel)
	// Volta também acelerada pelo nível da Toca do atacante.
	abuildings, err := q.ListCityBuildings(ctx, attacker.ID)
	if err != nil {
		return err
	}
	scoutLvl := buildingLevel(abuildings, config.ScoutHouseKey)
	returnAt := now.Add(time.Duration(config.ScoutMarchSeconds(scoutLvl, int(attacker.CoordX), int(attacker.CoordY), int(target.CoordX), int(target.CoordY))) * time.Second)
	if err := q.SetScoutMissionReturning(ctx, db.SetScoutMissionReturningParams{ID: mid, Intel: intelJSON, ReturnAt: pgTime(returnAt)}); err != nil {
		return err
	}
	payload2, _ := json.Marshal(scoutMissionPayload{MissionID: p.MissionID})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventScoutReturn, FiresAt: returnAt, Payload: payload2}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveScoutReturnEvent é o handler do scheduler para "scout.return".
func (s *Service) ResolveScoutReturnEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p scoutMissionPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	mid, err := db.ParseUUID(p.MissionID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	m, err := q.GetScoutMissionForUpdate(ctx, mid)
	if err != nil {
		return fmt.Errorf("buscar missão: %w", err)
	}
	if m.Status != "returning" {
		return tx.Commit(ctx)
	}
	if err := q.AddCityScouts(ctx, db.AddCityScoutsParams{ID: m.AttackerCityID, Scouts: 1}); err != nil { // batedor volta
		return err
	}
	if err := q.SetScoutMissionDone(ctx, mid); err != nil {
		return err
	}
	// Relatório de espionagem (intel) ao atacante — inclui o nome do alvo.
	var intel ScoutIntel
	_ = json.Unmarshal(m.Intel, &intel)
	targetName := ""
	if tc, err := q.GetCity(ctx, m.TargetCityID); err == nil {
		if tp, err := q.GetPlayer(ctx, tc.PlayerID); err == nil {
			targetName = tp.Username
		}
	}
	attacker, err := q.GetCity(ctx, m.AttackerCityID)
	if err != nil {
		return err
	}
	srJSON, _ := json.Marshal(struct {
		TargetName string `json:"target_name"`
		ScoutIntel
	}{TargetName: targetName, ScoutIntel: intel})
	if _, err := q.InsertReport(ctx, db.InsertReportParams{WorldID: m.WorldID, PlayerID: attacker.PlayerID, Type: reportTypeScout, Payload: srJSON}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- helpers ---

func hasBuilding(buildings []db.CityBuilding, key string) bool {
	for _, b := range buildings {
		if b.BuildingType == key {
			return true
		}
	}
	return false
}

func buildingLevel(buildings []db.CityBuilding, key string) int {
	for _, b := range buildings {
		if b.BuildingType == key {
			return int(b.Level)
		}
	}
	return 0
}

func scoutMissionToDomain(m db.ScoutMission) ScoutMission {
	dm := ScoutMission{
		ID: db.UUIDString(m.ID), TargetCityID: db.UUIDString(m.TargetCityID), Status: m.Status, ArriveAt: m.ArriveAt,
	}
	if len(m.Intel) > 0 {
		var intel ScoutIntel
		if json.Unmarshal(m.Intel, &intel) == nil {
			dm.Intel = &intel
		}
	}
	if m.ReturnAt.Valid {
		t := m.ReturnAt.Time
		dm.ReturnAt = &t
	}
	return dm
}
