package city

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/domain/resource"
)

// Erros de negócio da construção (mapeáveis para HTTP 4xx).
var (
	ErrBuildingUnknown   = errors.New("edifício desconhecido")
	ErrPrereqNotMet      = errors.New("pré-requisito não atendido")
	ErrNoFreeSlot        = errors.New("sem slot livre na cidade")
	ErrMaxCopies         = errors.New("limite de cópias do edifício atingido")
	ErrInsufficient      = errors.New("recursos insuficientes")
	ErrBuildingNotInSlot = errors.New("nenhum edifício neste slot")
	ErrSlotBusy          = errors.New("já há uma construção em andamento neste slot")
)

// BuildQueued descreve uma construção/upgrade recém-enfileirado.
type BuildQueued struct {
	ID           string    `json:"id"`
	BuildingType string    `json:"building_type"`
	SlotIndex    int       `json:"slot_index"`
	TargetLevel  int       `json:"target_level"`
	FinishAt     time.Time `json:"finish_at"`
}

// EventBuildComplete é o tipo de evento agendado para concluir uma construção.
const EventBuildComplete = "build.complete"

type buildCompletePayload struct {
	BuildQueueID string `json:"build_queue_id"`
}

// EnqueueConstruct enfileira a construção de um NOVO edifício (nível 1) num slot livre.
func (s *Service) EnqueueConstruct(ctx context.Context, cityID, buildingType string, now time.Time) (BuildQueued, error) {
	def, ok := config.BuildingByKey(buildingType)
	if !ok {
		return BuildQueued{}, ErrBuildingUnknown
	}
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return BuildQueued{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BuildQueued{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return BuildQueued{}, fmt.Errorf("lock cidade: %w", err)
	}
	buildings, err := q.ListCityBuildings(ctx, id)
	if err != nil {
		return BuildQueued{}, err
	}
	pending, err := q.ListPendingBuilds(ctx, id)
	if err != nil {
		return BuildQueued{}, err
	}

	if err := checkPrereqs(def, buildings); err != nil {
		return BuildQueued{}, err
	}

	// Slots ocupados = edifícios existentes + construções pendentes (reservados).
	used := map[int16]bool{}
	copies := 0
	for _, b := range buildings {
		used[b.SlotIndex] = true
		if b.BuildingType == def.Key {
			copies++
		}
	}
	for _, p := range pending {
		used[p.SlotIndex] = true
		if p.BuildingType == def.Key {
			copies++
		}
	}
	if copies >= def.MaxCopies {
		return BuildQueued{}, ErrMaxCopies
	}

	slot := -1
	for i := 0; i < config.SlotsForEra(int(cityRow.Era)); i++ {
		if !used[int16(i)] {
			slot = i
			break
		}
	}
	if slot < 0 {
		return BuildQueued{}, ErrNoFreeSlot
	}

	bq, err := enqueueBuild(ctx, q, cityRow, slot, def, 1, now)
	if err != nil {
		return BuildQueued{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BuildQueued{}, fmt.Errorf("commit: %w", err)
	}
	return toBuildQueued(bq), nil
}

// EnqueueUpgrade enfileira o upgrade do edifício existente no slot (nível atual + 1).
func (s *Service) EnqueueUpgrade(ctx context.Context, cityID string, slot int, now time.Time) (BuildQueued, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return BuildQueued{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BuildQueued{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return BuildQueued{}, fmt.Errorf("lock cidade: %w", err)
	}
	buildings, err := q.ListCityBuildings(ctx, id)
	if err != nil {
		return BuildQueued{}, err
	}
	pending, err := q.ListPendingBuilds(ctx, id)
	if err != nil {
		return BuildQueued{}, err
	}

	for _, p := range pending {
		if int(p.SlotIndex) == slot {
			return BuildQueued{}, ErrSlotBusy
		}
	}

	var current *db.CityBuilding
	for i := range buildings {
		if int(buildings[i].SlotIndex) == slot {
			current = &buildings[i]
			break
		}
	}
	if current == nil {
		return BuildQueued{}, ErrBuildingNotInSlot
	}
	def, ok := config.BuildingByKey(current.BuildingType)
	if !ok {
		return BuildQueued{}, ErrBuildingUnknown
	}

	target := int(current.Level) + 1
	bq, err := enqueueBuild(ctx, q, cityRow, slot, def, target, now)
	if err != nil {
		return BuildQueued{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BuildQueued{}, fmt.Errorf("commit: %w", err)
	}
	return toBuildQueued(bq), nil
}

// enqueueBuild faz o gasto de recursos (lazy spend), insere na fila e agenda a conclusão,
// tudo na transação `q`. Não faz commit (responsabilidade do chamador).
func enqueueBuild(ctx context.Context, q *db.Queries, cityRow db.City, slot int, def config.BuildingDef, targetLevel int, now time.Time) (db.BuildQueue, error) {
	cost := config.CostFor(def.BaseCost, targetLevel)
	newState, ok := stateFromRow(cityRow).Spend(cost, now)
	if !ok {
		return db.BuildQueue{}, ErrInsufficient
	}
	if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID:                 cityRow.ID,
		MatterStored:       newState.Stored.Matter,
		EnergyStored:       newState.Stored.Energy,
		KnowledgeStored:    newState.Stored.Knowledge,
		MatterRate:         newState.RatePerHour.Matter,
		EnergyRate:         newState.RatePerHour.Energy,
		KnowledgeRate:      newState.RatePerHour.Knowledge,
		ResourcesUpdatedAt: now,
	}); err != nil {
		return db.BuildQueue{}, err
	}

	finishAt := now.Add(config.BuildTimeFor(def.BaseTime, targetLevel))
	bq, err := q.InsertBuildQueue(ctx, db.InsertBuildQueueParams{
		CityID:       cityRow.ID,
		SlotIndex:    int16(slot),
		BuildingType: def.Key,
		TargetLevel:  int16(targetLevel),
		StartedAt:    now,
		FinishAt:     finishAt,
	})
	if err != nil {
		return db.BuildQueue{}, err
	}

	// Outbox transacional: o evento de conclusão só existe se a fila/gasto forem commitados.
	payload, _ := json.Marshal(buildCompletePayload{BuildQueueID: db.UUIDString(bq.ID)})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{
		Type:    EventBuildComplete,
		FiresAt: finishAt,
		Payload: payload,
	}); err != nil {
		return db.BuildQueue{}, err
	}
	return bq, nil
}

func toBuildQueued(bq db.BuildQueue) BuildQueued {
	return BuildQueued{
		ID:           db.UUIDString(bq.ID),
		BuildingType: bq.BuildingType,
		SlotIndex:    int(bq.SlotIndex),
		TargetLevel:  int(bq.TargetLevel),
		FinishAt:     bq.FinishAt,
	}
}

// CompleteBuildEvent é o handler do scheduler para o evento "build.complete".
func (s *Service) CompleteBuildEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p buildCompletePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.CompleteBuild(ctx, p.BuildQueueID, now)
}

// CompleteBuild aplica uma construção/upgrade concluído: grava o nível no slot e recalcula
// a produção. Idempotente: se a fila já não estiver pendente, não faz nada.
func (s *Service) CompleteBuild(ctx context.Context, buildQueueID string, now time.Time) error {
	bqID, err := db.ParseUUID(buildQueueID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	item, err := q.GetBuildQueueForUpdate(ctx, bqID)
	if err != nil {
		return fmt.Errorf("buscar fila: %w", err)
	}
	n, err := q.CompleteBuildQueue(ctx, bqID)
	if err != nil {
		return err
	}
	if n == 0 {
		return tx.Commit(ctx) // já processado por outra execução
	}

	if _, err := q.UpsertCityBuilding(ctx, db.UpsertCityBuildingParams{
		CityID:       item.CityID,
		SlotIndex:    item.SlotIndex,
		BuildingType: item.BuildingType,
		Level:        item.TargetLevel,
	}); err != nil {
		return err
	}

	if err := recomputeProduction(ctx, q, item.CityID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func checkPrereqs(def config.BuildingDef, buildings []db.CityBuilding) error {
	level := map[string]int{}
	for _, b := range buildings {
		if int(b.Level) > level[b.BuildingType] {
			level[b.BuildingType] = int(b.Level)
		}
	}
	for _, r := range def.Requires {
		if level[r.BuildingKey] < r.Level {
			return fmt.Errorf("%w: requer %s nível %d", ErrPrereqNotMet, r.BuildingKey, r.Level)
		}
	}
	return nil
}

// recomputeProduction soma a produção de todos os edifícios, materializa os recursos
// acumulados até "now" e grava a nova taxa. Usa lock na cidade.
func recomputeProduction(ctx context.Context, q *db.Queries, cityID pgtype.UUID, now time.Time) error {
	cityRow, err := q.GetCityForUpdate(ctx, cityID)
	if err != nil {
		return err
	}
	buildings, err := q.ListCityBuildings(ctx, cityID)
	if err != nil {
		return err
	}
	var rate resource.Amounts
	for _, b := range buildings {
		if def, ok := config.BuildingByKey(b.BuildingType); ok {
			p := def.ProductionAt(int(b.Level))
			rate.Matter += p.Matter
			rate.Energy += p.Energy
			rate.Knowledge += p.Knowledge
		}
	}
	cur := stateFromRow(cityRow).At(now)
	return q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID:                 cityID,
		MatterStored:       cur.Matter,
		EnergyStored:       cur.Energy,
		KnowledgeStored:    cur.Knowledge,
		MatterRate:         rate.Matter,
		EnergyRate:         rate.Energy,
		KnowledgeRate:      rate.Knowledge,
		ResourcesUpdatedAt: now,
	})
}
