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

// Erros de negócio do recrutamento (mapeáveis para HTTP 4xx).
var (
	ErrUnitUnknown     = errors.New("unidade desconhecida")
	ErrNoBarracks      = errors.New("requer o Canteiro de Almas para recrutar")
	ErrArmyCapExceeded = errors.New("teto de exército excedido")
	ErrBadCount        = errors.New("quantidade inválida")
	ErrUnitLocked      = errors.New("unidade ainda bloqueada (suba o Canteiro de Almas)")
)

// EventRecruitComplete é o tipo de evento agendado para concluir um recrutamento.
const EventRecruitComplete = "recruit.complete"

type recruitCompletePayload struct {
	RecruitQueueID string `json:"recruit_queue_id"`
}

// RecruitQueued descreve um recrutamento recém-enfileirado.
type RecruitQueued struct {
	ID       string    `json:"id"`
	UnitType string    `json:"unit_type"`
	Count    int       `json:"count"`
	FinishAt time.Time `json:"finish_at"`
}

// EnqueueRecruit enfileira o recrutamento de `count` unidades de `unitType`, respeitando o
// teto de exército (Canteiro de Almas) e debitando o custo (custo unitário × count).
func (s *Service) EnqueueRecruit(ctx context.Context, cityID, unitType string, count int, now time.Time) (RecruitQueued, error) {
	def, ok := config.UnitByKey(unitType)
	if !ok {
		return RecruitQueued{}, ErrUnitUnknown
	}
	if count <= 0 {
		return RecruitQueued{}, ErrBadCount
	}
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return RecruitQueued{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecruitQueued{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return RecruitQueued{}, fmt.Errorf("lock cidade: %w", err)
	}
	buildings, err := q.ListCityBuildings(ctx, id)
	if err != nil {
		return RecruitQueued{}, err
	}

	bLevel := barracksLevel(buildings)
	capacity := config.ArmyCap(bLevel)
	if capacity == 0 {
		return RecruitQueued{}, ErrNoBarracks
	}
	if bLevel < def.MinBarracksLevel {
		return RecruitQueued{}, ErrUnitLocked
	}

	// Uso atual = guarnição + recrutamentos pendentes (de qualquer tipo).
	troops, err := q.ListCityTroops(ctx, id)
	if err != nil {
		return RecruitQueued{}, err
	}
	pending, err := q.ListPendingRecruits(ctx, id)
	if err != nil {
		return RecruitQueued{}, err
	}
	used := 0
	for _, t := range troops {
		used += int(t.Count)
	}
	for _, p := range pending {
		used += int(p.Count)
	}
	if used+count > capacity {
		return RecruitQueued{}, fmt.Errorf("%w: %d/%d", ErrArmyCapExceeded, used, capacity)
	}

	cost := resource.Amounts{
		Matter:    def.Cost.Matter * float64(count),
		Energy:    def.Cost.Energy * float64(count),
		Knowledge: def.Cost.Knowledge * float64(count),
	}
	newState, ok := stateFromRow(cityRow).Spend(cost, now)
	if !ok {
		return RecruitQueued{}, ErrInsufficient
	}
	if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID: cityRow.ID, MatterStored: newState.Stored.Matter, EnergyStored: newState.Stored.Energy, KnowledgeStored: newState.Stored.Knowledge,
		MatterRate: newState.RatePerHour.Matter, EnergyRate: newState.RatePerHour.Energy, KnowledgeRate: newState.RatePerHour.Knowledge, ResourcesUpdatedAt: now,
	}); err != nil {
		return RecruitQueued{}, err
	}

	finishAt := now.Add(time.Duration(def.RecruitTime * float64(count) * float64(time.Second)))
	rq, err := q.InsertRecruitQueue(ctx, db.InsertRecruitQueueParams{
		CityID: cityRow.ID, UnitType: def.Key, Count: int32(count), StartedAt: now, FinishAt: finishAt,
	})
	if err != nil {
		return RecruitQueued{}, err
	}
	payload, _ := json.Marshal(recruitCompletePayload{RecruitQueueID: db.UUIDString(rq.ID)})
	if _, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: EventRecruitComplete, FiresAt: finishAt, Payload: payload}); err != nil {
		return RecruitQueued{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecruitQueued{}, fmt.Errorf("commit: %w", err)
	}
	return RecruitQueued{ID: db.UUIDString(rq.ID), UnitType: rq.UnitType, Count: int(rq.Count), FinishAt: rq.FinishAt}, nil
}

// CompleteRecruitEvent é o handler do scheduler para o evento "recruit.complete".
func (s *Service) CompleteRecruitEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p recruitCompletePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.CompleteRecruit(ctx, p.RecruitQueueID, now)
}

// CompleteRecruit adiciona as unidades recrutadas à guarnição. Idempotente.
func (s *Service) CompleteRecruit(ctx context.Context, recruitQueueID string, now time.Time) error {
	rqID, err := db.ParseUUID(recruitQueueID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	item, err := q.GetRecruitForUpdate(ctx, rqID)
	if err != nil {
		return fmt.Errorf("buscar fila de recrutamento: %w", err)
	}
	n, err := q.CompleteRecruitQueue(ctx, rqID)
	if err != nil {
		return err
	}
	if n == 0 {
		return tx.Commit(ctx) // já processado
	}
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: item.CityID, UnitType: item.UnitType, Count: item.Count}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CancelRecruit cancela um recrutamento PENDENTE e devolve 100% do custo. Idempotente-seguro.
func (s *Service) CancelRecruit(ctx context.Context, cityID, recruitQueueID string, now time.Time) error {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return err
	}
	rqID, err := db.ParseUUID(recruitQueueID)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	item, err := q.GetRecruitForUpdate(ctx, rqID)
	if err != nil {
		return ErrBuildingNotFound
	}
	if !sameUUID(item.CityID, id) {
		return ErrBuildingNotFound
	}
	n, err := q.CancelRecruitQueue(ctx, rqID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotCancelable
	}

	def, ok := config.UnitByKey(item.UnitType)
	if !ok {
		return ErrUnitUnknown
	}
	c := float64(item.Count)
	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return err
	}
	cur := stateFromRow(cityRow).At(now)
	cur.Matter += def.Cost.Matter * c
	cur.Energy += def.Cost.Energy * c
	cur.Knowledge += def.Cost.Knowledge * c
	if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID: id, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
		MatterRate: cityRow.MatterRate, EnergyRate: cityRow.EnergyRate, KnowledgeRate: cityRow.KnowledgeRate, ResourcesUpdatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// barracksLevel retorna o maior nível do Canteiro de Almas entre os edifícios (0 se não há).
func barracksLevel(buildings []db.CityBuilding) int {
	lvl := 0
	for _, b := range buildings {
		if b.BuildingType == config.BarracksKey && int(b.Level) > lvl {
			lvl = int(b.Level)
		}
	}
	return lvl
}
