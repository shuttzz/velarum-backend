package city

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"backend/internal/pg"
)

func TestBuildFlow_Integration(t *testing.T) {
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

	// Construir um Viveiro de Pedra na célula (0,0). Custo nível 1: Matéria 60, Energia 20.
	bq, err := svc.EnqueueConstruct(ctx, c.ID, "viveiro_de_pedra", 0, 0, now)
	if err != nil {
		t.Fatalf("EnqueueConstruct: %v", err)
	}
	if bq.X != 0 || bq.Y != 0 {
		t.Fatalf("posição = (%d,%d), quero (0,0)", bq.X, bq.Y)
	}

	// Posição inválida: construir em cima do Lar do Clã (centro) -> ErrBadPlacement.
	if _, err := svc.EnqueueConstruct(ctx, c.ID, "celeiro_de_argila", c.GridW/2, c.GridH/2, now); !errors.Is(err, ErrBadPlacement) {
		t.Fatalf("esperava ErrBadPlacement, obtive %v", err)
	}

	// Recursos gastos: 200-60=140 Matéria, 100-20=80 Energia.
	afterSpend, _ := svc.LoadCity(ctx, c.ID, now)
	if afterSpend.Resources.Matter != 140 || afterSpend.Resources.Energy != 80 {
		t.Fatalf("recursos após gasto: %+v (quero 140/80)", afterSpend.Resources)
	}
	if len(afterSpend.Pending) != 1 || afterSpend.Pending[0].BuildingType != "viveiro_de_pedra" || afterSpend.Pending[0].IsUpgrade {
		t.Fatalf("esperava 1 construção pendente (viveiro, não-upgrade), got %+v", afterSpend.Pending)
	}

	// Conclui a construção (simula o scheduler no finish_at).
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild: %v", err)
	}
	done, _ := svc.LoadCity(ctx, c.ID, bq.FinishAt)
	if done.Rate.Matter != 8 {
		t.Fatalf("produção após conclusão = %v, quero 8", done.Rate.Matter)
	}
	if len(done.Buildings) != 2 {
		t.Fatalf("edifícios = %d, quero 2 (Lar do Clã + Viveiro)", len(done.Buildings))
	}
	if len(done.Pending) != 0 {
		t.Fatalf("pendências deveriam estar vazias após conclusão, got %d", len(done.Pending))
	}

	// Recursos sobem: 2h após a conclusão, 140 + 8*2 = 156.
	later, _ := svc.LoadCity(ctx, c.ID, bq.FinishAt.Add(2*time.Hour))
	if later.Resources.Matter != 156 {
		t.Fatalf("Matéria 2h após conclusão = %v, quero 156", later.Resources.Matter)
	}

	// Idempotência: concluir de novo não altera nada.
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild (idempotente): %v", err)
	}
	again, _ := svc.LoadCity(ctx, c.ID, bq.FinishAt.Add(2*time.Hour))
	if again.Resources.Matter != 156 {
		t.Fatalf("idempotência quebrada: Matéria = %v, quero 156", again.Resources.Matter)
	}
}
