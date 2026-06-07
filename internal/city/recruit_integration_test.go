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

func TestRecruitFlow_Integration(t *testing.T) {
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

	// Coloca um Canteiro de Almas nível 1 direto (habilita recrutamento — sem teto de posse).
	cityUUID, _ := db.ParseUUID(c.ID)
	if _, err := db.New(pool).InsertCityBuilding(ctx, db.InsertCityBuildingParams{
		CityID: cityUUID, BuildingType: config.BarracksKey, Level: 1, PosX: 0, PosY: 0,
	}); err != nil {
		t.Fatalf("InsertCityBuilding: %v", err)
	}

	// Recruta 5 lanceiros (custo 5×{Matéria 20, Energia 10} = 100/50; recursos iniciais 200/100).
	rq, err := svc.EnqueueRecruit(ctx, c.ID, "lanceiro", 5, now)
	if err != nil {
		t.Fatalf("EnqueueRecruit: %v", err)
	}
	if rq.Count != 5 || rq.UnitType != "lanceiro" {
		t.Fatalf("recrutamento inesperado: %+v", rq)
	}

	// Antes de concluir: aparece como recrutamento pendente, guarnição ainda vazia.
	loaded, err := svc.LoadCity(ctx, c.ID, now)
	if err != nil {
		t.Fatalf("LoadCity: %v", err)
	}
	if len(loaded.Recruits) != 1 || len(loaded.Troops) != 0 {
		t.Fatalf("estado pendente inesperado: recruits=%d troops=%d", len(loaded.Recruits), len(loaded.Troops))
	}
	if loaded.MarchCapacity != config.MarchCapForEra(1) {
		t.Fatalf("march_capacity = %d, quero %d", loaded.MarchCapacity, config.MarchCapForEra(1))
	}

	// Conclui o recrutamento → 5 lanceiros na guarnição.
	if err := svc.CompleteRecruit(ctx, rq.ID, rq.FinishAt); err != nil {
		t.Fatalf("CompleteRecruit: %v", err)
	}
	loaded, err = svc.LoadCity(ctx, c.ID, rq.FinishAt)
	if err != nil {
		t.Fatalf("LoadCity pós-conclusão: %v", err)
	}
	if len(loaded.Troops) != 1 || loaded.Troops[0].UnitType != "lanceiro" || loaded.Troops[0].Count != 5 {
		t.Fatalf("guarnição inesperada: %+v", loaded.Troops)
	}

	// Idempotência: concluir de novo não duplica.
	if err := svc.CompleteRecruit(ctx, rq.ID, rq.FinishAt); err != nil {
		t.Fatalf("CompleteRecruit (2x): %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, c.ID, rq.FinishAt)
	if loaded.Troops[0].Count != 5 {
		t.Fatalf("idempotência falhou: count = %d, quero 5", loaded.Troops[0].Count)
	}

	// SEM teto de posse: com recursos suficientes, recrutar muito além do antigo teto (25) deve PASSAR.
	if err := db.New(pool).UpdateCityResources(ctx, db.UpdateCityResourcesParams{
		ID: cityUUID, MatterStored: 100000, EnergyStored: 100000, KnowledgeStored: 100000,
		MatterRate: 0, EnergyRate: 0, KnowledgeRate: 0, ResourcesUpdatedAt: rq.FinishAt,
	}); err != nil {
		t.Fatalf("UpdateCityResources: %v", err)
	}
	if _, err := svc.EnqueueRecruit(ctx, c.ID, "lanceiro", 50, rq.FinishAt); err != nil {
		t.Fatalf("recrutar 50 (> antigo teto 25) deveria passar sem teto de posse, veio: %v", err)
	}
}

func TestCancelRecruit_Integration(t *testing.T) {
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
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	c := enterTestGame(t, svc, pool, "aurenthos", now) // 500/500/200

	cityUUID, _ := db.ParseUUID(c.ID)
	if _, err := db.New(pool).InsertCityBuilding(ctx, db.InsertCityBuildingParams{
		CityID: cityUUID, BuildingType: config.BarracksKey, Level: 1, PosX: 0, PosY: 0,
	}); err != nil {
		t.Fatalf("InsertCityBuilding: %v", err)
	}

	// Recruta 5 lanceiros (custo 100/50) → 400/450.
	rq, err := svc.EnqueueRecruit(ctx, c.ID, "lanceiro", 5, now)
	if err != nil {
		t.Fatalf("EnqueueRecruit: %v", err)
	}
	after, _ := svc.LoadCity(ctx, c.ID, now)
	if after.Resources.Matter != 400 || after.Resources.Energy != 450 {
		t.Fatalf("após recrutar: %+v (quero 400/450)", after.Resources)
	}

	// Cancela → devolução total (500/500) e fila de recrutamento vazia.
	if err := svc.CancelRecruit(ctx, c.ID, rq.ID, now); err != nil {
		t.Fatalf("CancelRecruit: %v", err)
	}
	refunded, _ := svc.LoadCity(ctx, c.ID, now)
	if refunded.Resources.Matter != 500 || refunded.Resources.Energy != 500 {
		t.Fatalf("devolução falhou: %+v (quero 500/500)", refunded.Resources)
	}
	if len(refunded.Recruits) != 0 {
		t.Fatalf("fila de recrutamento deveria estar vazia, got %d", len(refunded.Recruits))
	}
	if err := svc.CancelRecruit(ctx, c.ID, rq.ID, now); !errors.Is(err, ErrNotCancelable) {
		t.Fatalf("cancelar de novo deve dar ErrNotCancelable, veio %v", err)
	}
}
