package city

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"backend/internal/db"
	"backend/internal/pg"
)

// Era 1 = 1 fila de obra: a 2ª construção concorrente é recusada; ao concluir a 1ª, libera.
func TestBuildQueueLimit_Integration(t *testing.T) {
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
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	c := enterTestGame(t, svc, pool, "brevali", now)

	bq, err := svc.EnqueueConstruct(ctx, c.ID, "viveiro_de_pedra", 0, 0, now)
	if err != nil {
		t.Fatalf("1ª construção: %v", err)
	}
	// 2ª obra concorrente na Era 1 → fila cheia.
	if _, err := svc.EnqueueConstruct(ctx, c.ID, "fogueira_comunal", 1, 0, now); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("2ª obra deveria falhar com ErrQueueFull, veio %v", err)
	}
	// Conclui a 1ª → fila livre de novo.
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild: %v", err)
	}
	if _, err := svc.EnqueueConstruct(ctx, c.ID, "fogueira_comunal", 1, 0, bq.FinishAt); err != nil {
		t.Fatalf("após concluir, nova obra deveria passar: %v", err)
	}
}

// Era 1 = 1 fila de marcha: a 2ª marcha concorrente é recusada.
func TestMarchQueueLimit_Integration(t *testing.T) {
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
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	c := enterTestGame(t, svc, pool, "brevali", now)

	cityUUID, _ := db.ParseUUID(c.ID)
	if err := db.New(pool).AddCityTroops(ctx, db.AddCityTroopsParams{CityID: cityUUID, UnitType: "lanceiro", Count: 40}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}
	provs, err := svc.ListProvinces(ctx, c.ID, now)
	if err != nil {
		t.Fatalf("ListProvinces: %v", err)
	}

	if _, err := svc.StartMarch(ctx, c.ID, provs[0].ID, map[string]int{"lanceiro": 10}, now); err != nil {
		t.Fatalf("1ª marcha: %v", err)
	}
	// 2ª marcha concorrente na Era 1 → fila cheia.
	if _, err := svc.StartMarch(ctx, c.ID, provs[1].ID, map[string]int{"lanceiro": 10}, now); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("2ª marcha deveria falhar com ErrQueueFull, veio %v", err)
	}
}
