// Package city implementa os casos de uso de cidade (criar jogo, carregar, construir,
// concluir, mover edifícios numa grade 2D). A lógica temporal de recursos vem de
// internal/domain/resource e a de posicionamento de internal/domain/grid (ambas puras).
package city

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/domain/resource"
)

// Service expõe os casos de uso de cidade.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewService cria o serviço de cidade sobre um pool de conexões.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

// City é a visão de domínio de uma cidade, com recursos calculados (lazy eval) em "now".
type City struct {
	ID        string           `json:"id"`
	PlayerID  string           `json:"player_id"`
	Name      string           `json:"name"`
	Region    string           `json:"region"`
	Era       int              `json:"era"`
	CoordX    int              `json:"coord_x"`
	CoordY    int              `json:"coord_y"`
	Resources resource.Amounts `json:"resources"`
	Rate      resource.Amounts `json:"rate"`
	Capacity  resource.Amounts `json:"capacity"`
	GridW     int              `json:"grid_w"`
	GridH     int              `json:"grid_h"`
	Buildings []Building        `json:"buildings"`
	Pending   []PendingBuild   `json:"pending"`
	Troops    []Troop          `json:"troops"`
	Recruits  []RecruitQueued  `json:"recruits"`
	ArmyCap      int          `json:"army_cap"`
	Marches      []March      `json:"marches"`
	WorldMarches []WorldMarch `json:"world_marches"` // marchas a nós do mundo compartilhado (SW2)
	// ActiveBattleID é o id da batalha tática em andamento (vazio se nenhuma) — permite
	// ao frontend retomar/abrir a tela de batalha ao carregar a cidade.
	ActiveBattleID string    `json:"active_battle_id"`
	ServerNow      time.Time `json:"server_now"`
}

// Troop é a quantidade de uma unidade na guarnição da cidade.
type Troop struct {
	UnitType string `json:"unit_type"`
	Count    int    `json:"count"`
}

// Building é um edifício posicionado na grade da cidade.
type Building struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Level int    `json:"level"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	W     int    `json:"w"`
	H     int    `json:"h"`
}

// PendingBuild é uma construção (nova) ou upgrade em andamento na fila.
type PendingBuild struct {
	ID           string    `json:"id"`
	BuildingType string    `json:"building_type"`
	TargetLevel  int       `json:"target_level"`
	X            int       `json:"x"`
	Y            int       `json:"y"`
	IsUpgrade    bool      `json:"is_upgrade"`
	FinishAt     time.Time `json:"finish_at"`
}

// EnterWorld coloca a conta no mundo padrão (compartilhado): cria seu player e a cidade
// inicial (Era 1, recursos iniciais + Lar do Clã no centro da grade) caso ainda não existam.
// É IDEMPOTENTE — se a conta já tem cidade no mundo, retorna a existente. A alocação de
// coordenada no mapa é serializada por um lock no mundo (FOR UPDATE).
func (s *Service) EnterWorld(ctx context.Context, accountID, faction, cityName string, now time.Time) (City, error) {
	accUUID, err := db.ParseUUID(accountID)
	if err != nil {
		return City{}, err
	}
	worldUUID, err := db.ParseUUID(config.DefaultWorldID)
	if err != nil {
		return City{}, err
	}

	// Caminho rápido: já tem player no mundo → carrega a cidade existente.
	if c, ok, err := s.loadExistingCity(ctx, s.q, worldUUID, accUUID, now); err != nil {
		return City{}, err
	} else if ok {
		return c, nil
	}

	if faction == "" {
		faction = "aurenthos"
	}
	if cityName == "" {
		cityName = "Capital"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return City{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	// Lock no mundo: serializa a alocação de coordenadas entre entradas concorrentes.
	if _, err := q.GetWorldForUpdate(ctx, worldUUID); err != nil {
		return City{}, fmt.Errorf("mundo padrão indisponível: %w", err)
	}

	// Reconfere player (corrida): outra entrada pode ter criado nesse meio-tempo.
	if c, ok, err := s.loadExistingCity(ctx, q, worldUUID, accUUID, now); err != nil {
		return City{}, err
	} else if ok {
		return c, nil
	}

	acc, err := q.GetAccountByID(ctx, accUUID)
	if err != nil {
		return City{}, fmt.Errorf("conta inexistente: %w", err)
	}
	player, err := q.CreatePlayer(ctx, db.CreatePlayerParams{
		WorldID: worldUUID, AccountID: accUUID, Username: acc.Username, Faction: faction,
	})
	if err != nil {
		return City{}, fmt.Errorf("criar jogador: %w", err)
	}

	// Posicionar a cidade no MUNDO COMPARTILHADO: preenchimento por região (quadrante) até o teto,
	// espalhada dentro da região, com espaçamento mínimo de outras cidades.
	cities, err := q.ListWorldCities(ctx, worldUUID)
	if err != nil {
		return City{}, fmt.Errorf("listar cidades: %w", err)
	}
	taken := make(map[[2]int]bool, len(cities))
	counts := map[string]int{}
	for _, c := range cities {
		taken[[2]int{int(c.CoordX), int(c.CoordY)}] = true
		counts[c.Region]++
	}
	rng := rand.New(rand.NewSource(now.UnixNano())) //nolint:gosec // placement, não-cripto
	regionKey, cx, cy := config.PlaceNewCity(rng, counts, taken)

	start := config.StartingResources
	// Cidade nova começa SEM proteção contra saque (cap 0/0/0). A parcela protegida só passa a
	// existir após construir o Celeiro de Argila (recomputeProduction recalcula os caps).
	row, err := q.CreateCity(ctx, db.CreateCityParams{
		WorldID: worldUUID, PlayerID: player.ID, Name: cityName, Region: regionKey,
		CoordX: int32(cx), CoordY: int32(cy), Era: 1,
		MatterStored: start.Matter, EnergyStored: start.Energy, KnowledgeStored: start.Knowledge,
		MatterRate: 0, EnergyRate: 0, KnowledgeRate: 0,
		MatterCap: 0, EnergyCap: 0, KnowledgeCap: 0,
		ResourcesUpdatedAt: now,
	})
	if err != nil {
		return City{}, fmt.Errorf("criar cidade: %w", err)
	}

	// Lar do Clã nível 1 no centro da grade.
	gw, gh := config.GridForEra(1)
	larBuilding, err := q.InsertCityBuilding(ctx, db.InsertCityBuildingParams{
		CityID: row.ID, BuildingType: "lar_do_cla", Level: 1,
		PosX: int32(gw / 2), PosY: int32(gh / 2),
	})
	if err != nil {
		return City{}, fmt.Errorf("criar edifício inicial: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return City{}, fmt.Errorf("commit: %w", err)
	}

	c := toDomainCity(row, now)
	c.Buildings = []Building{buildingToDomain(larBuilding)}
	return c, nil
}

// loadExistingCity retorna (cidade, true, nil) se a conta já tem player+cidade no mundo;
// (zero, false, nil) se ainda não tem; ou (zero, false, err) em falha real.
func (s *Service) loadExistingCity(ctx context.Context, q *db.Queries, worldUUID, accUUID pgtype.UUID, now time.Time) (City, bool, error) {
	player, err := q.GetPlayerByAccountAndWorld(ctx, db.GetPlayerByAccountAndWorldParams{
		WorldID: worldUUID, AccountID: accUUID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return City{}, false, nil
	}
	if err != nil {
		return City{}, false, fmt.Errorf("buscar player: %w", err)
	}
	cityRow, err := q.GetCityByPlayer(ctx, player.ID)
	if err != nil {
		return City{}, false, fmt.Errorf("buscar cidade: %w", err)
	}
	c, err := s.LoadCity(ctx, db.UUIDString(cityRow.ID), now)
	if err != nil {
		return City{}, false, err
	}
	return c, true, nil
}

// OwnerAccountID retorna o ID da conta dona da cidade (via player). pgx.ErrNoRows se a
// cidade não existe.
func (s *Service) OwnerAccountID(ctx context.Context, cityID string) (string, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return "", err
	}
	acc, err := s.q.GetCityAccountID(ctx, id)
	if err != nil {
		return "", err
	}
	return db.UUIDString(acc), nil
}

// WorldCity é uma cidade visível no mapa-mundo COMPARTILHADO (posição + dono).
type WorldCity struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Region   string `json:"region"`
	CoordX   int    `json:"coord_x"`
	CoordY   int    `json:"coord_y"`
	Username string `json:"username"`
}

// WorldCities lista todas as cidades do mundo padrão (compartilhado) — o mapa mostra os vizinhos.
func (s *Service) WorldCities(ctx context.Context) ([]WorldCity, error) {
	worldUUID, err := db.ParseUUID(config.DefaultWorldID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListWorldCities(ctx, worldUUID)
	if err != nil {
		return nil, err
	}
	out := make([]WorldCity, 0, len(rows))
	for _, r := range rows {
		out = append(out, WorldCity{
			ID: db.UUIDString(r.ID), Name: r.Name, Region: r.Region,
			CoordX: int(r.CoordX), CoordY: int(r.CoordY), Username: r.Username,
		})
	}
	return out, nil
}

// LoadCity carrega a cidade (com edifícios) e calcula os recursos atuais (lazy eval) em "now".
func (s *Service) LoadCity(ctx context.Context, cityID string, now time.Time) (City, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return City{}, err
	}
	row, err := s.q.GetCity(ctx, id)
	if err != nil {
		return City{}, err
	}
	buildings, err := s.q.ListCityBuildings(ctx, id)
	if err != nil {
		return City{}, err
	}
	pending, err := s.q.ListPendingBuilds(ctx, id)
	if err != nil {
		return City{}, err
	}
	c := toDomainCity(row, now)
	for _, b := range buildings {
		c.Buildings = append(c.Buildings, buildingToDomain(b))
	}
	for _, p := range pending {
		c.Pending = append(c.Pending, PendingBuild{
			ID:           db.UUIDString(p.ID),
			BuildingType: p.BuildingType,
			TargetLevel:  int(p.TargetLevel),
			X:            int(p.PosX),
			Y:            int(p.PosY),
			IsUpgrade:    p.BuildingID.Valid,
			FinishAt:     p.FinishAt,
		})
	}

	// Exército: guarnição + recrutamentos pendentes + teto (nível do Canteiro de Almas).
	c.ArmyCap = config.ArmyCap(barracksLevel(buildings))
	troops, err := s.q.ListCityTroops(ctx, id)
	if err != nil {
		return City{}, err
	}
	for _, tr := range troops {
		if tr.Count > 0 {
			c.Troops = append(c.Troops, Troop{UnitType: tr.UnitType, Count: int(tr.Count)})
		}
	}
	recruits, err := s.q.ListPendingRecruits(ctx, id)
	if err != nil {
		return City{}, err
	}
	for _, r := range recruits {
		c.Recruits = append(c.Recruits, RecruitQueued{
			ID: db.UUIDString(r.ID), UnitType: r.UnitType, Count: int(r.Count), FinishAt: r.FinishAt,
		})
	}
	marches, err := s.q.ListActiveMarches(ctx, id)
	if err != nil {
		return City{}, err
	}
	for _, m := range marches {
		c.Marches = append(c.Marches, marchToDomain(m))
	}
	worldMarches, err := s.q.ListActiveWorldMarches(ctx, id)
	if err != nil {
		return City{}, err
	}
	for _, m := range worldMarches {
		c.WorldMarches = append(c.WorldMarches, worldMarchToDomain(m))
	}
	if bat, err := s.q.GetActiveBattle(ctx, id); err == nil {
		c.ActiveBattleID = db.UUIDString(bat.ID)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return City{}, err
	}
	return c, nil
}

// SetProduction materializa os recursos acumulados até "now" e passa a produzir "rate".
func (s *Service) SetProduction(ctx context.Context, cityID string, rate resource.Amounts, now time.Time) error {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return err
	}
	row, err := s.q.GetCity(ctx, id)
	if err != nil {
		return err
	}
	cur := stateFromRow(row).At(now)
	return s.q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID: id, MatterStored: cur.Matter, EnergyStored: cur.Energy, KnowledgeStored: cur.Knowledge,
		MatterRate: rate.Matter, EnergyRate: rate.Energy, KnowledgeRate: rate.Knowledge, ResourcesUpdatedAt: now,
	})
}

func stateFromRow(c db.City) resource.State {
	return resource.State{
		Stored:      resource.Amounts{Matter: c.MatterStored, Energy: c.EnergyStored, Knowledge: c.KnowledgeStored},
		RatePerHour: resource.Amounts{Matter: c.MatterRate, Energy: c.EnergyRate, Knowledge: c.KnowledgeRate},
		Capacity:    resource.Amounts{Matter: c.MatterCap, Energy: c.EnergyCap, Knowledge: c.KnowledgeCap},
		UpdatedAt:   c.ResourcesUpdatedAt,
	}
}

func toDomainCity(c db.City, now time.Time) City {
	st := stateFromRow(c)
	gw, gh := config.GridForEra(int(c.Era))
	return City{
		ID: db.UUIDString(c.ID), PlayerID: db.UUIDString(c.PlayerID), Name: c.Name, Region: c.Region,
		Era: int(c.Era), CoordX: int(c.CoordX), CoordY: int(c.CoordY),
		Resources: st.At(now), Rate: st.RatePerHour, Capacity: st.Capacity,
		GridW: gw, GridH: gh, Buildings: []Building{}, Pending: []PendingBuild{},
		Troops: []Troop{}, Recruits: []RecruitQueued{}, Marches: []March{}, WorldMarches: []WorldMarch{}, ServerNow: now,
	}
}

func buildingToDomain(b db.CityBuilding) Building {
	w, h := footprintOf(b.BuildingType)
	return Building{
		ID: db.UUIDString(b.ID), Type: b.BuildingType, Level: int(b.Level),
		X: int(b.PosX), Y: int(b.PosY), W: w, H: h,
	}
}

func footprintOf(buildingType string) (w, h int) {
	if def, ok := config.BuildingByKey(buildingType); ok {
		return def.Footprint()
	}
	return 1, 1
}
