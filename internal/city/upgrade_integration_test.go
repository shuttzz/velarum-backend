package city

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"backend/internal/pg"
)

func TestUpgradeEMoveFlow_Integration(t *testing.T) {
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

	// Constrói e conclui um Viveiro de Pedra em (0,0).
	bq, err := svc.EnqueueConstruct(ctx, c.ID, "viveiro_de_pedra", 0, 0, now)
	if err != nil {
		t.Fatalf("EnqueueConstruct: %v", err)
	}
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild: %v", err)
	}

	// Descobre o id do Viveiro.
	done, _ := svc.LoadCity(ctx, c.ID, bq.FinishAt)
	var viveiroID string
	for _, b := range done.Buildings {
		if b.Type == "viveiro_de_pedra" {
			viveiroID = b.ID
		}
	}
	if viveiroID == "" {
		t.Fatal("não achei o Viveiro")
	}

	// Upgrade do Viveiro (nível 1 -> 2).
	up, err := svc.EnqueueUpgrade(ctx, c.ID, viveiroID, bq.FinishAt)
	if err != nil {
		t.Fatalf("EnqueueUpgrade: %v", err)
	}
	if up.TargetLevel != 2 {
		t.Fatalf("target level = %d, quero 2", up.TargetLevel)
	}

	// Upgrade pendente no mesmo edifício -> ErrBuildingBusy.
	if _, err := svc.EnqueueUpgrade(ctx, c.ID, viveiroID, bq.FinishAt); !errors.Is(err, ErrBuildingBusy) {
		t.Fatalf("esperava ErrBuildingBusy, obtive %v", err)
	}
	// Edifício inexistente -> ErrBuildingNotFound.
	if _, err := svc.EnqueueUpgrade(ctx, c.ID, "01920000-0000-7000-8000-000000000000", bq.FinishAt); !errors.Is(err, ErrBuildingNotFound) {
		t.Fatalf("esperava ErrBuildingNotFound, obtive %v", err)
	}

	// Conclui o upgrade: Viveiro nível 2 produz 12/h.
	if err := svc.CompleteBuild(ctx, up.ID, up.FinishAt); err != nil {
		t.Fatalf("CompleteBuild (upgrade): %v", err)
	}
	after, _ := svc.LoadCity(ctx, c.ID, up.FinishAt)
	if after.Rate.Matter != 12 {
		t.Fatalf("produção após upgrade = %v, quero 12", after.Rate.Matter)
	}

	// Mover o Viveiro de (0,0) para (1,1) — válido.
	if err := svc.MoveBuilding(ctx, c.ID, viveiroID, 1, 1, up.FinishAt); err != nil {
		t.Fatalf("MoveBuilding: %v", err)
	}
	moved, _ := svc.LoadCity(ctx, c.ID, up.FinishAt)
	for _, b := range moved.Buildings {
		if b.ID == viveiroID && (b.X != 1 || b.Y != 1) {
			t.Fatalf("Viveiro em (%d,%d), quero (1,1)", b.X, b.Y)
		}
	}

	// Mover para cima do Lar do Clã (centro) -> ErrBadPlacement.
	if err := svc.MoveBuilding(ctx, c.ID, viveiroID, c.GridW/2, c.GridH/2, up.FinishAt); !errors.Is(err, ErrBadPlacement) {
		t.Fatalf("esperava ErrBadPlacement, obtive %v", err)
	}
}
