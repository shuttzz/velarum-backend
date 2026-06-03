package city

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"backend/internal/pg"
)

func TestCancelBuild_Integration(t *testing.T) {
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
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	c := enterTestGame(t, svc, pool, "aurenthos", now) // 500/500/200

	// Constrói um Viveiro (custo 50 Matéria, 20 Energia) → estoque cai para 450/480.
	bq, err := svc.EnqueueConstruct(ctx, c.ID, "viveiro_de_pedra", 0, 0, now)
	if err != nil {
		t.Fatalf("EnqueueConstruct: %v", err)
	}
	after, _ := svc.LoadCity(ctx, c.ID, now)
	if after.Resources.Matter != 450 || after.Resources.Energy != 480 {
		t.Fatalf("após gasto: %+v (quero 450/480)", after.Resources)
	}

	// Cancela → devolução TOTAL (volta a 500/500) e fila vazia.
	if err := svc.CancelBuild(ctx, c.ID, bq.ID, now); err != nil {
		t.Fatalf("CancelBuild: %v", err)
	}
	refunded, _ := svc.LoadCity(ctx, c.ID, now)
	if refunded.Resources.Matter != 500 || refunded.Resources.Energy != 500 {
		t.Fatalf("devolução total falhou: %+v (quero 500/500)", refunded.Resources)
	}
	if len(refunded.Pending) != 0 {
		t.Fatalf("fila deveria estar vazia após cancelar, got %d", len(refunded.Pending))
	}

	// Cancelar de novo → ErrNotCancelable.
	if err := svc.CancelBuild(ctx, c.ID, bq.ID, now); !errors.Is(err, ErrNotCancelable) {
		t.Fatalf("esperava ErrNotCancelable, veio %v", err)
	}

	// O evento de conclusão (que ainda dispararia) vira no-op: nenhum edifício novo é criado.
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild (cancelado): %v", err)
	}
	done, _ := svc.LoadCity(ctx, c.ID, bq.FinishAt)
	if len(done.Buildings) != 1 { // só o Lar do Clã
		t.Fatalf("obra cancelada não deveria virar edifício: %d edifícios", len(done.Buildings))
	}
}
