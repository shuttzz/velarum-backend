package city

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"backend/internal/pg"
)

func TestUpgradeFlow_Integration(t *testing.T) {
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

	c, err := svc.CreateNewGame(ctx, NewGameInput{
		WorldName: "Mundo Up", Username: "up", Email: "up@example.com",
		Faction: "brevali", CityName: "Forja", CoordX: 9, CoordY: 9,
	}, now)
	if err != nil {
		t.Fatalf("CreateNewGame: %v", err)
	}

	// Constrói e conclui um Viveiro de Pedra (slot 1, nível 1, produz 8/h).
	bq, err := svc.EnqueueConstruct(ctx, c.ID, "viveiro_de_pedra", now)
	if err != nil {
		t.Fatalf("EnqueueConstruct: %v", err)
	}
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild: %v", err)
	}

	// Upgrade do slot 1 (nível 1 -> 2).
	up, err := svc.EnqueueUpgrade(ctx, c.ID, 1, bq.FinishAt)
	if err != nil {
		t.Fatalf("EnqueueUpgrade: %v", err)
	}
	if up.TargetLevel != 2 {
		t.Fatalf("target level = %d, quero 2", up.TargetLevel)
	}

	// Slot com upgrade pendente -> ErrSlotBusy.
	if _, err := svc.EnqueueUpgrade(ctx, c.ID, 1, bq.FinishAt); !errors.Is(err, ErrSlotBusy) {
		t.Fatalf("esperava ErrSlotBusy, obtive %v", err)
	}
	// Slot vazio -> ErrBuildingNotInSlot.
	if _, err := svc.EnqueueUpgrade(ctx, c.ID, 5, bq.FinishAt); !errors.Is(err, ErrBuildingNotInSlot) {
		t.Fatalf("esperava ErrBuildingNotInSlot, obtive %v", err)
	}

	// Conclui o upgrade: Viveiro nível 2 produz 12/h.
	if err := svc.CompleteBuild(ctx, up.ID, up.FinishAt); err != nil {
		t.Fatalf("CompleteBuild (upgrade): %v", err)
	}
	done, _ := svc.LoadCity(ctx, c.ID, up.FinishAt)
	if done.Rate.Matter != 12 {
		t.Fatalf("produção após upgrade = %v, quero 12", done.Rate.Matter)
	}
	level := 0
	for _, b := range done.Buildings {
		if b.Slot == 1 {
			level = b.Level
		}
	}
	if level != 2 {
		t.Fatalf("Viveiro slot 1 nível = %d, quero 2", level)
	}
}
