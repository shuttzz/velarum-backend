package city

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/pg"
)

// Recrutamento = 1 lane por TIPO: 2º recruta do mesmo tipo é recusado; outro tipo é permitido.
func TestRecruitOnePerType_Integration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL não definido — pulando teste de integração")
	}
	if err := pg.Migrate(url); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	pool, err := pg.Connect(ctx, url)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()

	svc := NewService(pool)
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	c := enterTestGame(t, svc, pool, "brevali", now)

	// Canteiro nível 2 → habilita lanceiro E arqueiro; teto = 20+10 = 30.
	cityUUID, _ := db.ParseUUID(c.ID)
	if _, err := db.New(pool).InsertCityBuilding(ctx, db.InsertCityBuildingParams{
		CityID: cityUUID, BuildingType: config.BarracksKey, Level: 2, PosX: 0, PosY: 0,
	}); err != nil {
		t.Fatalf("InsertCityBuilding: %v", err)
	}

	if _, err := svc.EnqueueRecruit(ctx, c.ID, "lanceiro", 2, now); err != nil {
		t.Fatalf("1º lanceiro: %v", err)
	}
	// 2º recruta de lanceiro (mesmo tipo) → recusado.
	if _, err := svc.EnqueueRecruit(ctx, c.ID, "lanceiro", 1, now); !errors.Is(err, ErrRecruitBusy) {
		t.Fatalf("2º lanceiro deveria falhar com ErrRecruitBusy, veio %v", err)
	}
	// Outro tipo (arqueiro) em paralelo → permitido.
	if _, err := svc.EnqueueRecruit(ctx, c.ID, "arqueiro", 1, now); err != nil {
		t.Fatalf("arqueiro (outro tipo) deveria passar: %v", err)
	}
}
