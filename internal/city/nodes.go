package city

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/domain/resource"
)

// SW2 — PvE em tiles COMPARTILHADOS do mundo. Esta fatia: NÓS DE RECURSO (coleta estilo RoK).
// Marcha → ocupa o nó → coleta por um tempo (∝ carga÷taxa, taxa POR TROPA) → volta com loot.
// Depleção PARCIAL; respawn ao zerar; 1 ocupante por vez (2º que chega em nó ocupado/esgotado
// volta pra casa — a batalha auto-resolvida na disputa entra numa fatia futura, junto com PvP).

// Eventos agendados dos nós (espelhados em scheduled_events).
const (
	EventWorldArrival = "world.arrival"
	EventWorldCollect = "world.collect"
	EventWorldReturn  = "world.return"
)

const reportTypeCollection = "collection"

// Erros de negócio dos nós (mapeáveis para HTTP 4xx).
var (
	ErrTargetNotFound = errors.New("alvo não encontrado")
	ErrTargetDepleted = errors.New("nó esgotado")
)

type worldMarchEventPayload struct {
	MarchID string `json:"march_id"`
}

// collectReport é o payload de um relatório de coleta (resultado de uma marcha a um nó).
type collectReport struct {
	TargetID  string         `json:"target_id"`
	Resource  string         `json:"resource"` // "" quando bounce (não coletou)
	Collected float64        `json:"collected"`
	Sent      map[string]int `json:"sent"`
	Bounced   bool           `json:"bounced"` // true = nó ocupado/esgotado ao chegar; voltou sem coletar
}

// WorldTarget é a visão de domínio de um alvo PvE compartilhado (SW2: nó de recurso).
type WorldTarget struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	Resource        string  `json:"resource"`
	Level           int     `json:"level"`
	CoordX          int     `json:"coord_x"`
	CoordY          int     `json:"coord_y"`
	AmountTotal     float64 `json:"amount_total"`
	AmountRemaining float64 `json:"amount_remaining"`
	Status          string  `json:"status"`
}

// WorldMarch é a visão de domínio de uma marcha a um nó (ida → coleta → volta).
type WorldMarch struct {
	ID           string           `json:"id"`
	TargetID     string           `json:"target_id"`
	Status       string           `json:"status"` // outbound | collecting | returning | done
	Troops       map[string]int   `json:"troops"`
	Loot         resource.Amounts `json:"loot"`
	ArriveAt     time.Time        `json:"arrive_at"`
	CollectUntil *time.Time       `json:"collect_until"`
	ReturnAt     *time.Time       `json:"return_at"`
}

// WorldTargets lista os alvos PvE vivos do mundo padrão (gera o conjunto inicial na 1ª vez).
func (s *Service) WorldTargets(ctx context.Context, now time.Time) ([]WorldTarget, error) {
	worldUUID, err := db.ParseUUID(config.DefaultWorldID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureWorldTargets(ctx, worldUUID, now); err != nil {
		return nil, err
	}
	rows, err := s.q.ListWorldTargets(ctx, worldUUID)
	if err != nil {
		return nil, err
	}
	out := make([]WorldTarget, 0, len(rows))
	for _, r := range rows {
		out = append(out, worldTargetToDomain(r))
	}
	return out, nil
}

// ensureWorldTargets semeia o conjunto inicial de nós (uma vez por mundo). Serializa pelo lock do
// mundo (FOR UPDATE) para evitar seed concorrente. Respawn mantém a contagem estável depois disso.
func (s *Service) ensureWorldTargets(ctx context.Context, worldUUID pgtype.UUID, now time.Time) error {
	n, err := s.q.CountWorldTargets(ctx, worldUUID)
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

	if _, err := q.GetWorldForUpdate(ctx, worldUUID); err != nil {
		return err
	}
	again, err := q.CountWorldTargets(ctx, worldUUID)
	if err != nil {
		return err
	}
	if again > 0 {
		return tx.Commit(ctx) // outra requisição semeou
	}

	taken, err := worldTakenCoords(ctx, q, worldUUID)
	if err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(now.UnixNano())) //nolint:gosec // placement, não-cripto
	for _, sp := range config.PlaceWorldNodes(rng, taken) {
		if _, err := q.InsertWorldTarget(ctx, db.InsertWorldTargetParams{
			WorldID: worldUUID, Kind: "node", Resource: sp.Resource, Level: int32(sp.Level),
			CoordX: int32(sp.X), CoordY: int32(sp.Y), AmountTotal: config.NodeAmountFor(sp.Resource, sp.Level),
		}); err != nil {
			return fmt.Errorf("semear nó: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// StartCollect envia `troops` da guarnição para coletar num nó do mundo (marcha de ida = timer).
func (s *Service) StartCollect(ctx context.Context, cityID, targetID string, troops map[string]int, now time.Time) (WorldMarch, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return WorldMarch{}, err
	}
	tid, err := db.ParseUUID(targetID)
	if err != nil {
		return WorldMarch{}, err
	}
	total := 0
	for ut, c := range troops {
		if c <= 0 {
			return WorldMarch{}, ErrBadCount
		}
		if _, ok := config.UnitByKey(ut); !ok {
			return WorldMarch{}, ErrUnitUnknown
		}
		total += c
	}
	if total == 0 {
		return WorldMarch{}, ErrBadCount
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return WorldMarch{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	cityRow, err := q.GetCityForUpdate(ctx, id)
	if err != nil {
		return WorldMarch{}, fmt.Errorf("lock cidade: %w", err)
	}
	target, err := q.GetWorldTargetForUpdate(ctx, tid)
	if err != nil {
		return WorldMarch{}, ErrTargetNotFound
	}
	if target.Status == "depleted" || target.AmountRemaining <= 0 {
		return WorldMarch{}, ErrTargetDepleted
	}

	// Fila de expedições (províncias + nós) limitada por era — mesma lane.
	n, err := activeExpeditions(ctx, q, id)
	if err != nil {
		return WorldMarch{}, err
	}
	if n >= config.QueuesForEra(int(cityRow.Era)) {
		return WorldMarch{}, ErrQueueFull
	}

	garrison := map[string]int{}
	rows, err := q.ListCityTroops(ctx, id)
	if err != nil {
		return WorldMarch{}, err
	}
	for _, t := range rows {
		garrison[t.UnitType] = int(t.Count)
	}
	for ut, c := range troops {
		if garrison[ut] < c {
			return WorldMarch{}, ErrNoTroops
		}
	}
	for ut, c := range troops {
		if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: id, UnitType: ut, Count: int32(-c)}); err != nil {
			return WorldMarch{}, err
		}
	}

	dur := time.Duration(config.MarchSecondsBetween(int(cityRow.CoordX), int(cityRow.CoordY), int(target.CoordX), int(target.CoordY))) * time.Second
	arriveAt := now.Add(dur)
	troopsJSON, _ := json.Marshal(troops)
	m, err := q.InsertWorldMarch(ctx, db.InsertWorldMarchParams{
		WorldID: cityRow.WorldID, CityID: id, TargetID: tid, Troops: troopsJSON, DepartAt: now, ArriveAt: arriveAt,
	})
	if err != nil {
		return WorldMarch{}, err
	}
	if err := scheduleWorldEvent(ctx, q, EventWorldArrival, db.UUIDString(m.ID), arriveAt); err != nil {
		return WorldMarch{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WorldMarch{}, fmt.Errorf("commit: %w", err)
	}
	return worldMarchToDomain(m), nil
}

// ResolveWorldArrivalEvent é o handler do scheduler para "world.arrival".
func (s *Service) ResolveWorldArrivalEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p worldMarchEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.ResolveWorldArrival(ctx, p.MarchID, now)
}

// ResolveWorldArrival: chegou ao nó. Se livre, OCUPA e inicia a coleta (timer ∝ carga÷taxa);
// se ocupado/esgotado, volta pra casa sem coletar (bounce). Idempotente.
func (s *Service) ResolveWorldArrival(ctx context.Context, marchID string, now time.Time) error {
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

	m, err := q.GetWorldMarchForUpdate(ctx, mid)
	if err != nil {
		return fmt.Errorf("buscar marcha: %w", err)
	}
	if m.Status != "outbound" {
		return tx.Commit(ctx) // já processado
	}
	target, err := q.GetWorldTargetForUpdate(ctx, m.TargetID)
	if err != nil {
		return err
	}
	cityRow, err := q.GetCity(ctx, m.CityID)
	if err != nil {
		return err
	}
	backDur := time.Duration(config.MarchSecondsBetween(int(cityRow.CoordX), int(cityRow.CoordY), int(target.CoordX), int(target.CoordY))) * time.Second

	busy := target.OccupiedBy.Valid
	empty := target.Status == "depleted" || target.AmountRemaining <= 0

	if busy || empty {
		// Bounce: volta pra casa sem coletar (loot fica vazio).
		return s.bounceWorldMarch(ctx, q, tx, m, now.Add(backDur))
	}

	var troops map[string]int
	_ = json.Unmarshal(m.Troops, &troops)
	collected, seconds := config.CollectPlan(troops, target.AmountRemaining)
	if collected <= 0 {
		return s.bounceWorldMarch(ctx, q, tx, m, now.Add(backDur))
	}

	// Reserva (decrementa) o coletado e trava a ocupação.
	if err := q.ReserveWorldTarget(ctx, db.ReserveWorldTargetParams{
		ID: target.ID, AmountRemaining: target.AmountRemaining - collected, OccupiedBy: mid,
	}); err != nil {
		return err
	}
	lootJSON, _ := json.Marshal(amountsForResource(target.Resource, collected))
	collectUntil := now.Add(time.Duration(seconds * float64(time.Second)))
	if err := q.SetWorldMarchCollecting(ctx, db.SetWorldMarchCollectingParams{
		ID: mid, Loot: lootJSON, CollectUntil: pgTime(collectUntil),
	}); err != nil {
		return err
	}
	if err := scheduleWorldEvent(ctx, q, EventWorldCollect, marchID, collectUntil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// bounceWorldMarch encerra a coleta antes de começar (nó ocupado/esgotado): agenda a volta com as
// tropas intactas e sem loot. Commita a transação.
func (s *Service) bounceWorldMarch(ctx context.Context, q *db.Queries, tx pgx.Tx, m db.WorldMarch, returnAt time.Time) error {
	if err := q.SetWorldMarchReturning(ctx, db.SetWorldMarchReturningParams{
		ID: m.ID, Survivors: m.Troops, ReturnAt: pgTime(returnAt),
	}); err != nil {
		return err
	}
	if err := scheduleWorldEvent(ctx, q, EventWorldReturn, db.UUIDString(m.ID), returnAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveWorldCollectEvent é o handler do scheduler para "world.collect".
func (s *Service) ResolveWorldCollectEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p worldMarchEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.ResolveWorldCollect(ctx, p.MarchID, now)
}

// ResolveWorldCollect: coleta terminou. Libera o nó (ou respawna se zerou) e manda as tropas de
// volta com o loot. Idempotente.
func (s *Service) ResolveWorldCollect(ctx context.Context, marchID string, now time.Time) error {
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

	m, err := q.GetWorldMarchForUpdate(ctx, mid)
	if err != nil {
		return fmt.Errorf("buscar marcha: %w", err)
	}
	if m.Status != "collecting" {
		return tx.Commit(ctx) // já processado
	}
	target, err := q.GetWorldTargetForUpdate(ctx, m.TargetID)
	if err != nil {
		return err
	}
	// Só libera/respawna se ESTA marcha é quem ocupa (idempotência/segurança).
	if sameUUID(target.OccupiedBy, mid) {
		if target.AmountRemaining <= 0 {
			taken, err := worldTakenCoords(ctx, q, target.WorldID)
			if err != nil {
				return err
			}
			delete(taken, [2]int{int(target.CoordX), int(target.CoordY)}) // a própria casa libera
			rng := rand.New(rand.NewSource(now.UnixNano())) //nolint:gosec // placement
			if sp, ok := config.PlaceRespawnNode(rng, taken); ok {
				if err := q.RespawnWorldTarget(ctx, db.RespawnWorldTargetParams{
					ID: target.ID, Kind: "node", Resource: sp.Resource, Level: int32(sp.Level),
					CoordX: int32(sp.X), CoordY: int32(sp.Y), AmountTotal: config.NodeAmountFor(sp.Resource, sp.Level),
				}); err != nil {
					return err
				}
			} else if err := q.ReleaseWorldTarget(ctx, target.ID); err != nil {
				return err
			}
		} else if err := q.ReleaseWorldTarget(ctx, target.ID); err != nil {
			return err
		}
	}

	cityRow, err := q.GetCity(ctx, m.CityID)
	if err != nil {
		return err
	}
	backDur := time.Duration(config.MarchSecondsBetween(int(cityRow.CoordX), int(cityRow.CoordY), int(target.CoordX), int(target.CoordY))) * time.Second
	if err := q.SetWorldMarchReturning(ctx, db.SetWorldMarchReturningParams{
		ID: mid, Survivors: m.Troops, ReturnAt: pgTime(now.Add(backDur)),
	}); err != nil {
		return err
	}
	if err := scheduleWorldEvent(ctx, q, EventWorldReturn, marchID, now.Add(backDur)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ResolveWorldReturnEvent é o handler do scheduler para "world.return".
func (s *Service) ResolveWorldReturnEvent(ctx context.Context, payload []byte, now time.Time) error {
	var p worldMarchEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	return s.ResolveWorldReturn(ctx, p.MarchID, now)
}

// ResolveWorldReturn: tropas voltaram à cidade. Devolve a guarnição, credita o loot e gera o
// relatório de coleta. Idempotente.
func (s *Service) ResolveWorldReturn(ctx context.Context, marchID string, now time.Time) error {
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

	m, err := q.GetWorldMarchForUpdate(ctx, mid)
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

	loot := resource.Amounts{}
	if len(m.Loot) > 0 {
		_ = json.Unmarshal(m.Loot, &loot)
	}
	collected := loot.Matter + loot.Energy + loot.Knowledge
	if collected > 0 {
		cityRow, err := q.GetCityForUpdate(ctx, m.CityID)
		if err != nil {
			return err
		}
		cur := stateFromRow(cityRow).At(now)
		cur.Matter += loot.Matter
		cur.Energy += loot.Energy
		cur.Knowledge += loot.Knowledge
		if err := q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
			ID: m.CityID, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
			MatterRate: cityRow.MatterRate, EnergyRate: cityRow.EnergyRate, KnowledgeRate: cityRow.KnowledgeRate, ResourcesUpdatedAt: now,
		}); err != nil {
			return err
		}
	}

	if err := q.SetWorldMarchDone(ctx, mid); err != nil {
		return err
	}

	var sent map[string]int
	_ = json.Unmarshal(m.Troops, &sent)
	crJSON, _ := json.Marshal(collectReport{
		TargetID: db.UUIDString(m.TargetID), Resource: resourceNameOf(loot), Collected: collected, Sent: sent, Bounced: collected <= 0,
	})
	if _, err := q.InsertReport(ctx, db.InsertReportParams{WorldID: m.WorldID, PlayerID: cityPlayerID(ctx, q, m.CityID), Type: reportTypeCollection, Payload: crJSON}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- helpers ---

// activeExpeditions conta as marchas ativas (províncias + nós do mundo) de uma cidade — todas
// dividem a mesma fila/lane limitada por era.
func activeExpeditions(ctx context.Context, q *db.Queries, cityID pgtype.UUID) (int, error) {
	pm, err := q.ListActiveMarches(ctx, cityID)
	if err != nil {
		return 0, err
	}
	wm, err := q.ListActiveWorldMarches(ctx, cityID)
	if err != nil {
		return 0, err
	}
	return len(pm) + len(wm), nil
}

func scheduleWorldEvent(ctx context.Context, q *db.Queries, kind, marchID string, at time.Time) error {
	payload, _ := json.Marshal(worldMarchEventPayload{MarchID: marchID})
	_, err := q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{Type: kind, FiresAt: at, Payload: payload})
	return err
}

func worldTakenCoords(ctx context.Context, q *db.Queries, worldUUID pgtype.UUID) (map[[2]int]bool, error) {
	taken := map[[2]int]bool{}
	cities, err := q.ListWorldCityCoords(ctx, worldUUID)
	if err != nil {
		return nil, err
	}
	for _, c := range cities {
		taken[[2]int{int(c.CoordX), int(c.CoordY)}] = true
	}
	targets, err := q.ListWorldTargets(ctx, worldUUID)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		taken[[2]int{int(t.CoordX), int(t.CoordY)}] = true
	}
	return taken, nil
}

func cityPlayerID(ctx context.Context, q *db.Queries, cityID pgtype.UUID) pgtype.UUID {
	row, err := q.GetCity(ctx, cityID)
	if err != nil {
		return pgtype.UUID{}
	}
	return row.PlayerID
}

func amountsForResource(res string, v float64) resource.Amounts {
	switch res {
	case "matter":
		return resource.Amounts{Matter: v}
	case "energy":
		return resource.Amounts{Energy: v}
	case "knowledge":
		return resource.Amounts{Knowledge: v}
	}
	return resource.Amounts{}
}

func resourceNameOf(a resource.Amounts) string {
	switch {
	case a.Matter > 0:
		return "matter"
	case a.Energy > 0:
		return "energy"
	case a.Knowledge > 0:
		return "knowledge"
	}
	return ""
}

func worldTargetToDomain(t db.WorldTarget) WorldTarget {
	return WorldTarget{
		ID: db.UUIDString(t.ID), Kind: t.Kind, Resource: t.Resource, Level: int(t.Level),
		CoordX: int(t.CoordX), CoordY: int(t.CoordY),
		AmountTotal: t.AmountTotal, AmountRemaining: t.AmountRemaining, Status: t.Status,
	}
}

func worldMarchToDomain(m db.WorldMarch) WorldMarch {
	dm := WorldMarch{
		ID: db.UUIDString(m.ID), TargetID: db.UUIDString(m.TargetID), Status: m.Status, ArriveAt: m.ArriveAt,
	}
	_ = json.Unmarshal(m.Troops, &dm.Troops)
	if len(m.Loot) > 0 {
		_ = json.Unmarshal(m.Loot, &dm.Loot)
	}
	if m.CollectUntil.Valid {
		t := m.CollectUntil.Time
		dm.CollectUntil = &t
	}
	if m.ReturnAt.Valid {
		t := m.ReturnAt.Time
		dm.ReturnAt = &t
	}
	return dm
}
