// Package city implementa os casos de uso de cidade (criar jogo, carregar cidade,
// ligar produção). A lógica temporal de recursos vem de internal/domain/resource (pura).
package city

import (
	"context"
	"fmt"
	"time"

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

// City é a visão de domínio de uma cidade, com os recursos já calculados (lazy eval) em "now".
type City struct {
	ID        string           `json:"id"`
	PlayerID  string           `json:"player_id"`
	Name      string           `json:"name"`
	Era       int              `json:"era"`
	CoordX    int              `json:"coord_x"`
	CoordY    int              `json:"coord_y"`
	Resources resource.Amounts `json:"resources"` // recursos ATUAIS em now
	Rate      resource.Amounts `json:"rate"`       // produção por hora
	Capacity  resource.Amounts `json:"capacity"`   // teto de armazém
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
// com os recursos iniciais da config e o Lar do Clã nível 1.
func (s *Service) CreateNewGame(ctx context.Context, in NewGameInput, now time.Time) (City, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return City{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) // no-op após commit bem-sucedido

	q := s.q.WithTx(tx)

	world, err := q.CreateWorld(ctx, db.CreateWorldParams{Name: in.WorldName, Speed: 1})
	if err != nil {
		return City{}, fmt.Errorf("criar mundo: %w", err)
	}
	player, err := q.CreatePlayer(ctx, db.CreatePlayerParams{
		WorldID:      world.ID,
		Username:     in.Username,
		Email:        in.Email,
		PasswordHash: "",
		Faction:      in.Faction,
	})
	if err != nil {
		return City{}, fmt.Errorf("criar jogador: %w", err)
	}

	start := config.StartingResources
	capStore := config.StartingStorage
	row, err := q.CreateCity(ctx, db.CreateCityParams{
		WorldID:            world.ID,
		PlayerID:           player.ID,
		Name:               in.CityName,
		CoordX:             int32(in.CoordX),
		CoordY:             int32(in.CoordY),
		Era:                1,
		MatterStored:       start.Matter,
		EnergyStored:       start.Energy,
		KnowledgeStored:    start.Knowledge,
		MatterRate:         0, // ainda sem edifícios de produção
		EnergyRate:         0,
		KnowledgeRate:      0,
		MatterCap:          capStore.Matter,
		EnergyCap:          capStore.Energy,
		KnowledgeCap:       capStore.Knowledge,
		ResourcesUpdatedAt: now,
	})
	if err != nil {
		return City{}, fmt.Errorf("criar cidade: %w", err)
	}

	// Edifício inicial: Lar do Clã nível 1 no slot 0.
	if _, err := q.AddCityBuilding(ctx, db.AddCityBuildingParams{
		CityID:       row.ID,
		SlotIndex:    0,
		BuildingType: "lar_do_cla",
		Level:        1,
	}); err != nil {
		return City{}, fmt.Errorf("criar edifício inicial: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return City{}, fmt.Errorf("commit: %w", err)
	}
	return toDomainCity(row, now), nil
}

// LoadCity carrega uma cidade e calcula os recursos atuais (lazy eval) em "now".
func (s *Service) LoadCity(ctx context.Context, cityID string, now time.Time) (City, error) {
	id, err := parseUUID(cityID)
	if err != nil {
		return City{}, err
	}
	row, err := s.q.GetCity(ctx, id)
	if err != nil {
		return City{}, err
	}
	return toDomainCity(row, now), nil
}

// SetProduction materializa os recursos acumulados até "now" e passa a produzir "rate".
// É o padrão que a conclusão de um edifício de produção vai usar (1B-2).
func (s *Service) SetProduction(ctx context.Context, cityID string, rate resource.Amounts, now time.Time) error {
	id, err := parseUUID(cityID)
	if err != nil {
		return err
	}
	row, err := s.q.GetCity(ctx, id)
	if err != nil {
		return err
	}
	cur := stateFromRow(row).At(now)
	return s.q.UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID:                 id,
		MatterStored:       cur.Matter,
		EnergyStored:       cur.Energy,
		KnowledgeStored:    cur.Knowledge,
		MatterRate:         rate.Matter,
		EnergyRate:         rate.Energy,
		KnowledgeRate:      rate.Knowledge,
		ResourcesUpdatedAt: now,
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
	return City{
		ID:        uuidToString(c.ID),
		PlayerID:  uuidToString(c.PlayerID),
		Name:      c.Name,
		Era:       int(c.Era),
		CoordX:    int(c.CoordX),
		CoordY:    int(c.CoordY),
		Resources: st.At(now),
		Rate:      st.RatePerHour,
		Capacity:  st.Capacity,
	}
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("uuid inválido %q: %w", s, err)
	}
	return u, nil
}

func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
