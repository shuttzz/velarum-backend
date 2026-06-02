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
	"backend/internal/domain/grid"
	"backend/internal/domain/resource"
)

// Erros de negócio da construção (mapeáveis para HTTP 4xx).
var (
	ErrBuildingUnknown  = errors.New("edifício desconhecido")
	ErrPrereqNotMet     = errors.New("pré-requisito não atendido")
	ErrMaxCopies        = errors.New("limite de cópias do edifício atingido")
	ErrInsufficient     = errors.New("recursos insuficientes")
	ErrBadPlacement     = errors.New("posição inválida (fora da grade ou ocupada)")
	ErrBuildingNotFound = errors.New("edifício não encontrado")
	ErrBuildingBusy     = errors.New("já há uma construção em andamento neste edifício")
)

// BuildQueued descreve uma construção/upgrade recém-enfileirado.
type BuildQueued struct {
	ID           string    `json:"id"`
	BuildingType string    `json:"building_type"`
	TargetLevel  int       `json:"target_level"`
	X            int       `json:"x"`
	Y            int       `json:"y"`
	FinishAt     time.Time `json:"finish_at"`
}

// EventBuildComplete é o tipo de evento agendado para concluir uma construção.
const EventBuildComplete = "build.complete"

type buildCompletePayload struct {
	BuildQueueID string `json:"build_queue_id"`
}

// EnqueueConstruct enfileira a construção de um NOVO edifício (nível 1) na posição (x,y).
func (s *Service) EnqueueConstruct(ctx context.Context, cityID, buildingType string, x, y int, now time.Time) (BuildQueued, error) {
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

	// Limite de cópias (existentes + construções novas pendentes).
	copies := 0
	for _, b := range buildings {
		if b.BuildingType == def.Key {
			copies++
		}
	}
	for _, p := range pending {
		if !p.BuildingID.Valid && p.BuildingType == def.Key {
			copies++
		}
	}
	if copies >= def.MaxCopies {
		return BuildQueued{}, ErrMaxCopies
	}

	// Posicionamento na grade.
	w, h := def.Footprint()
	gw, gh := config.GridForEra(int(cityRow.Era))
	rect := grid.Rect{X: x, Y: y, W: w, H: h}
	if !grid.Fits(rect, gw, gh, occupiedRects(buildings, pending, pgtype.UUID{})) {
		return BuildQueued{}, ErrBadPlacement
	}

	bq, err := enqueueBuild(ctx, q, cityRow, pgtype.UUID{}, def, 1, x, y, now)
	if err != nil {
		return BuildQueued{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BuildQueued{}, fmt.Errorf("commit: %w", err)
	}
	return toBuildQueued(bq), nil
}

// EnqueueUpgrade enfileira o upgrade de um edifício (por id) para o nível atual + 1.
func (s *Service) EnqueueUpgrade(ctx context.Context, cityID, buildingID string, now time.Time) (BuildQueued, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return BuildQueued{}, err
	}
	bid, err := db.ParseUUID(buildingID)
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

	current := findBuilding(buildings, bid)
	if current == nil {
		return BuildQueued{}, ErrBuildingNotFound
	}
	for _, p := range pending {
		if sameUUID(p.BuildingID, bid) {
			return BuildQueued{}, ErrBuildingBusy
		}
	}
	def, ok := config.BuildingByKey(current.BuildingType)
	if !ok {
		return BuildQueued{}, ErrBuildingUnknown
	}

	target := int(current.Level) + 1
	bq, err := enqueueBuild(ctx, q, cityRow, bid, def, target, int(current.PosX), int(current.PosY), now)
	if err != nil {
		return BuildQueued{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BuildQueued{}, fmt.Errorf("commit: %w", err)
	}
	return toBuildQueued(bq), nil
}

// MoveBuilding reposiciona um edifício existente (instantâneo) para (x,y), validando a grade.
func (s *Service) MoveBuilding(ctx context.Context, cityID, buildingID string, x, y int, now time.Time) error {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return err
	}
	bid, err := db.ParseUUID(buildingID)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return err
	}
	buildings, err := q.ListCityBuildings(ctx, id)
	if err != nil {
		return err
	}
	pending, err := q.ListPendingBuilds(ctx, id)
	if err != nil {
		return err
	}

	current := findBuilding(buildings, bid)
	if current == nil {
		return ErrBuildingNotFound
	}

	w, h := footprintOf(current.BuildingType)
	gw, gh := config.GridForEra(int(cityRow.Era))
	rect := grid.Rect{X: x, Y: y, W: w, H: h}
	if !grid.Fits(rect, gw, gh, occupiedRects(buildings, pending, bid)) {
		return ErrBadPlacement
	}
	if err := q.MoveCityBuilding(ctx, db.MoveCityBuildingParams{ID: bid, PosX: int32(x), PosY: int32(y)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func enqueueBuild(ctx context.Context, q *db.Queries, cityRow db.City, buildingID pgtype.UUID, def config.BuildingDef, targetLevel, x, y int, now time.Time) (db.BuildQueue, error) {
	cost := config.CostFor(def.BaseCost, targetLevel)
	newState, ok := stateFromRow(cityRow).Spend(cost, now)
	if !ok {
		return db.BuildQueue{}, ErrInsufficient
	}
	if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID: cityRow.ID, MatterStored: newState.Stored.Matter, EnergyStored: newState.Stored.Energy, KnowledgeStored: newState.Stored.Knowledge,
		MatterRate: newState.RatePerHour.Matter, EnergyRate: newState.RatePerHour.Energy, KnowledgeRate: newState.RatePerHour.Knowledge, ResourcesUpdatedAt: now,
	}); err != nil {
		return db.BuildQueue{}, err
	}

	finishAt := now.Add(config.BuildTimeFor(def.BaseTime, targetLevel))
	bq, err := q.InsertBuildQueue(ctx, db.InsertBuildQueueParams{
		CityID: cityRow.ID, BuildingID: buildingID, BuildingType: def.Key, TargetLevel: int16(targetLevel),
		PosX: int32(x), PosY: int32(y), StartedAt: now, FinishAt: finishAt,
	})
	if err != nil {
		return db.BuildQueue{}, err
	}

	payload, _ := json.Marshal(buildCompletePayload{BuildQueueID: db.UUIDString(bq.ID)})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventBuildComplete, FiresAt: finishAt, Payload: payload}); err != nil {
		return db.BuildQueue{}, err
	}
	return bq, nil
}

func toBuildQueued(bq db.BuildQueue) BuildQueued {
	return BuildQueued{
		ID: db.UUIDString(bq.ID), BuildingType: bq.BuildingType, TargetLevel: int(bq.TargetLevel),
		X: int(bq.PosX), Y: int(bq.PosY), FinishAt: bq.FinishAt,
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

// CompleteBuild aplica a construção/upgrade concluído e recalcula a produção. Idempotente.
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
		return tx.Commit(ctx) // já processado
	}

	if item.BuildingID.Valid {
		// Upgrade: sobe o nível do edifício existente.
		if err := q.SetCityBuildingLevel(ctx, db.SetCityBuildingLevelParams{ID: item.BuildingID, Level: item.TargetLevel}); err != nil {
			return err
		}
	} else {
		// Construção: cria o novo edifício na posição reservada.
		if _, err := q.InsertCityBuilding(ctx, db.InsertCityBuildingParams{
			CityID: item.CityID, BuildingType: item.BuildingType, Level: item.TargetLevel, PosX: item.PosX, PosY: item.PosY,
		}); err != nil {
			return err
		}
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
		ID: cityID, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
		MatterRate: rate.Matter, EnergyRate: rate.Energy, KnowledgeRate: rate.Knowledge, ResourcesUpdatedAt: now,
	})
}

func findBuilding(buildings []db.CityBuilding, id pgtype.UUID) *db.CityBuilding {
	for i := range buildings {
		if sameUUID(buildings[i].ID, id) {
			return &buildings[i]
		}
	}
	return nil
}

func sameUUID(a, b pgtype.UUID) bool {
	return a.Valid && b.Valid && a.Bytes == b.Bytes
}

// occupiedRects monta os retângulos ocupados: edifícios existentes + construções NOVAS
// pendentes (upgrades não mudam de lugar). `exclude` (se válido) é ignorado — útil ao mover.
func occupiedRects(buildings []db.CityBuilding, pending []db.ListPendingBuildsRow, exclude pgtype.UUID) []grid.Rect {
	var rects []grid.Rect
	for _, b := range buildings {
		if exclude.Valid && sameUUID(b.ID, exclude) {
			continue
		}
		w, h := footprintOf(b.BuildingType)
		rects = append(rects, grid.Rect{X: int(b.PosX), Y: int(b.PosY), W: w, H: h})
	}
	for _, p := range pending {
		if p.BuildingID.Valid {
			continue
		}
		w, h := footprintOf(p.BuildingType)
		rects = append(rects, grid.Rect{X: int(p.PosX), Y: int(p.PosY), W: w, H: h})
	}
	return rects
}
