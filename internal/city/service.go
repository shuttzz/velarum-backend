// Package city implementa os casos de uso de cidade (criar jogo, carregar, construir,
// concluir, mover edifícios numa grade 2D). A lógica temporal de recursos vem de
// internal/domain/resource e a de posicionamento de internal/domain/grid (ambas puras).
package city

import (
	"context"
	"fmt"
	"time"

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
	ServerNow time.Time         `json:"server_now"`
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

// NewGameInput descreve os dados para criar um novo jogo (mundo + jogador + cidade inicial).
type NewGameInput struct {
	WorldName string
	Username  string
	Email     string
	Faction   string
	CityName  string
	CoordX    int
	CoordY    int
}

// CreateNewGame cria, numa transação, um mundo, um jogador e a cidade inicial (Era 1)
// com os recursos iniciais e o Lar do Clã nível 1 no centro da grade.
func (s *Service) CreateNewGame(ctx context.Context, in NewGameInput, now time.Time) (City, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return City{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	world, err := q.CreateWorld(ctx, db.CreateWorldParams{Name: in.WorldName, Speed: 1})
	if err != nil {
		return City{}, fmt.Errorf("criar mundo: %w", err)
	}
	player, err := q.CreatePlayer(ctx, db.CreatePlayerParams{
		WorldID: world.ID, Username: in.Username, Email: in.Email, PasswordHash: "", Faction: in.Faction,
	})
	if err != nil {
		return City{}, fmt.Errorf("criar jogador: %w", err)
	}

	start := config.StartingResources
	capStore := config.StartingStorage
	row, err := q.CreateCity(ctx, db.CreateCityParams{
		WorldID: world.ID, PlayerID: player.ID, Name: in.CityName,
		CoordX: int32(in.CoordX), CoordY: int32(in.CoordY), Era: 1,
		MatterStored: start.Matter, EnergyStored: start.Energy, KnowledgeStored: start.Knowledge,
		MatterRate: 0, EnergyRate: 0, KnowledgeRate: 0,
		MatterCap: capStore.Matter, EnergyCap: capStore.Energy, KnowledgeCap: capStore.Knowledge,
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
		ID: db.UUIDString(c.ID), PlayerID: db.UUIDString(c.PlayerID), Name: c.Name,
		Era: int(c.Era), CoordX: int(c.CoordX), CoordY: int(c.CoordY),
		Resources: st.At(now), Rate: st.RatePerHour, Capacity: st.Capacity,
		GridW: gw, GridH: gh, Buildings: []Building{}, Pending: []PendingBuild{}, ServerNow: now,
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
