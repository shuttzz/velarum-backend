package city

import (
	"context"
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

	city, err := svc.CreateNewGame(ctx, NewGameInput{
		WorldName: "Mundo Build", Username: "builder", Email: "builder@example.com",
		Faction: "brevali", CityName: "Forja", CoordX: 5, CoordY: 5,
	}, now)
	if err != nil {
		t.Fatalf("CreateNewGame: %v", err)
	}

	// Enfileira um Viveiro de Pedra (custo nível 1: Matéria 60, Energia 20; tempo 30s).
	bq, err := svc.EnqueueConstruct(ctx, city.ID, "viveiro_de_pedra", now)
	if err != nil {
		t.Fatalf("EnqueueConstruct: %v", err)
	}
	if bq.SlotIndex != 1 { // slot 0 é o Lar do Clã
		t.Fatalf("slot = %d, quero 1", bq.SlotIndex)
	}
	if want := now.Add(30 * time.Second); !bq.FinishAt.Equal(want) {
		t.Fatalf("finish_at = %v, quero %v", bq.FinishAt, want)
	}

	// Recursos foram gastos: 200-60=140 Matéria, 100-20=80 Energia.
	afterSpend, _ := svc.LoadCity(ctx, city.ID, now)
	if afterSpend.Resources.Matter != 140 || afterSpend.Resources.Energy != 80 {
		t.Fatalf("recursos após gasto: %+v (quero 140/80)", afterSpend.Resources)
	}
	if afterSpend.Rate.Matter != 0 {
		t.Fatalf("produção antes da conclusão = %v, quero 0", afterSpend.Rate.Matter)
	}

	// Conclui a construção (simula o scheduler disparando no finish_at).
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild: %v", err)
	}

	done, _ := svc.LoadCity(ctx, city.ID, bq.FinishAt)
	if done.Rate.Matter != 8 { // Viveiro nível 1 produz 8 Matéria/h
		t.Fatalf("produção após conclusão = %v, quero 8", done.Rate.Matter)
	}
	if len(done.Buildings) != 2 {
		t.Fatalf("edifícios = %d, quero 2 (Lar do Clã + Viveiro)", len(done.Buildings))
	}

	// Recursos sobem com o tempo: 2h após a conclusão, 140 + 8*2 = 156.
	later, _ := svc.LoadCity(ctx, city.ID, bq.FinishAt.Add(2*time.Hour))
	if later.Resources.Matter != 156 {
		t.Fatalf("Matéria 2h após conclusão = %v, quero 156", later.Resources.Matter)
	}

	// Idempotência: concluir de novo não altera nada.
	if err := svc.CompleteBuild(ctx, bq.ID, bq.FinishAt); err != nil {
		t.Fatalf("CompleteBuild (idempotente): %v", err)
	}
	again, _ := svc.LoadCity(ctx, city.ID, bq.FinishAt.Add(2*time.Hour))
	if again.Resources.Matter != 156 {
		t.Fatalf("idempotência quebrada: Matéria = %v, quero 156", again.Resources.Matter)
	}
}
