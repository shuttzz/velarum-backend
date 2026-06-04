package city

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/internal/pg"
)

// A cidade começa SEM proteção (cap 0); só o Celeiro de Argila concede a parcela protegida.
func TestStorageCapFromGranary_Integration(t *testing.T) {
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
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	c := enterTestGame(t, svc, pool, "brevali", now)

	// Cidade nova: nenhuma proteção.
	if c.Capacity.Matter != 0 || c.Capacity.Energy != 0 || c.Capacity.Knowledge != 0 {
		t.Fatalf("cidade nova deveria começar sem proteção (0/0/0), veio %+v", c.Capacity)
	}

	// Constrói o Celeiro de Argila (nível 1) numa célula vazia.
	bq, err := svc.EnqueueConstruct(ctx, c.ID, "celeiro_de_argila", 0, 0, now)
	if err != nil {
		t.Fatalf("EnqueueConstruct: %v", err)
	}
	// Antes de concluir, ainda sem proteção.
	pending, _ := svc.LoadCity(ctx, c.ID, now)
	if pending.Capacity.Matter != 0 {
		t.Fatalf("proteção não deveria existir antes de concluir o Celeiro: %+v", pending.Capacity)
	}

	// Conclui: a parcela protegida passa a ser StorageCapFor(1) = 500 nos 3 recursos.
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild: %v", err)
	}
	done, _ := svc.LoadCity(ctx, c.ID, bq.FinishAt)
	if done.Capacity.Matter != 500 || done.Capacity.Energy != 500 || done.Capacity.Knowledge != 500 {
		t.Fatalf("proteção após Celeiro nv1 = %+v, quero 500/500/500", done.Capacity)
	}

	// Só pode haver UM Celeiro (MaxCopies = 1).
	if _, err := svc.EnqueueConstruct(ctx, c.ID, "celeiro_de_argila", 1, 0, bq.FinishAt); err == nil {
		t.Fatal("EnqueueConstruct de um 2º Celeiro deveria falhar (máx. 1 cópia)")
	}
}
