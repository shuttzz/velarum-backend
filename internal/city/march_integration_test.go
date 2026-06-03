package city

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/internal/db"
	"backend/internal/pg"
)

func TestMarchFlow_Integration(t *testing.T) {
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
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	c := enterTestGame(t, svc, pool, "brevali", now)

	// Semeia 20 lanceiros na guarnição (poder de sobra para o anel 1).
	cityUUID, _ := db.ParseUUID(c.ID)
	if err := db.New(pool).AddCityTroops(ctx, db.AddCityTroopsParams{CityID: cityUUID, UnitType: "lanceiro", Count: 20}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}

	// Gera/lista as províncias (anel 1) e escolhe a primeira.
	provs, err := svc.ListProvinces(ctx, c.ID, now)
	if err != nil {
		t.Fatalf("ListProvinces: %v", err)
	}
	if len(provs) != 6 {
		t.Fatalf("esperava 6 províncias no anel 1, veio %d", len(provs))
	}
	target := provs[0]
	if target.Status != "unconquered" {
		t.Fatalf("província deveria começar não conquistada: %+v", target)
	}

	// Marcha com 20 lanceiros.
	m, err := svc.StartMarch(ctx, c.ID, target.ID, map[string]int{"lanceiro": 20}, now)
	if err != nil {
		t.Fatalf("StartMarch: %v", err)
	}
	if m.Status != "outbound" {
		t.Fatalf("marcha deveria estar 'outbound', veio %q", m.Status)
	}
	// Durante a marcha a guarnição fica vazia (tropas a caminho).
	loaded, _ := svc.LoadCity(ctx, c.ID, now)
	if len(loaded.Troops) != 0 {
		t.Fatalf("guarnição deveria estar vazia durante a marcha: %+v", loaded.Troops)
	}
	if len(loaded.Marches) != 1 {
		t.Fatalf("esperava 1 marcha ativa, veio %d", len(loaded.Marches))
	}

	// Chegada: combate auto-resolve (vitória), conquista + recompensa.
	if err := svc.ResolveArrival(ctx, m.ID, m.ArriveAt); err != nil {
		t.Fatalf("ResolveArrival: %v", err)
	}
	provs, _ = svc.ListProvinces(ctx, c.ID, m.ArriveAt)
	var conquered *Province
	for i := range provs {
		if provs[i].ID == target.ID {
			conquered = &provs[i]
		}
	}
	if conquered == nil || conquered.Status != "conquered" {
		t.Fatalf("província deveria estar conquistada: %+v", conquered)
	}
	loaded, _ = svc.LoadCity(ctx, c.ID, m.ArriveAt)
	wantMatter := 500 + target.Reward.Matter // estoque inicial 500 + recompensa
	if loaded.Resources.Matter != wantMatter {
		t.Fatalf("recompensa não aplicada: matéria = %v, quero %v", loaded.Resources.Matter, wantMatter)
	}

	// Volta: sobreviventes retornam à guarnição (20 lanceiros sobrevivem ao anel 1).
	loaded, _ = svc.LoadCity(ctx, c.ID, m.ArriveAt)
	if loaded.Marches[0].Status != "returning" || loaded.Marches[0].ReturnAt == nil {
		t.Fatalf("marcha deveria estar 'returning' com return_at: %+v", loaded.Marches[0])
	}
	returnAt := *loaded.Marches[0].ReturnAt
	if err := svc.ResolveReturn(ctx, m.ID, returnAt); err != nil {
		t.Fatalf("ResolveReturn: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, c.ID, returnAt)
	if len(loaded.Troops) != 1 || loaded.Troops[0].Count != 20 {
		t.Fatalf("sobreviventes deveriam voltar à guarnição (20 lanceiros): %+v", loaded.Troops)
	}
	if len(loaded.Marches) != 0 {
		t.Fatalf("marcha deveria estar encerrada (done): %+v", loaded.Marches)
	}
}
