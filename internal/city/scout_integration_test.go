package city

import (
	"encoding/json"
	"testing"

	"backend/internal/db"
)

// Loop completo de ESPIONAGEM: treina batedores → envia 1 a um alvo → volta com snapshot de intel
// (guarnição + saqueável) + relatório; o batedor retorna à contagem.
func TestScoutFlow_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	a := enterTestGame(t, svc, pool, "brevali", now)
	b := enterTestGame(t, svc, pool, "brevali", now)
	aUUID, _ := db.ParseUUID(a.ID)
	bUUID, _ := db.ParseUUID(b.ID)

	// A precisa da Toca dos Batedores construída.
	if _, err := q.InsertCityBuilding(ctx, db.InsertCityBuildingParams{CityID: aUUID, BuildingType: "toca_dos_batedores", Level: 1, PosX: 0, PosY: 0}); err != nil {
		t.Fatalf("InsertCityBuilding: %v", err)
	}
	// B tem guarnição (intel deve revelar) e recursos desprotegidos (raidable).
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: bUUID, UnitType: "lanceiro", Count: 3}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}

	// Treina 2 batedores.
	sq, err := svc.TrainScout(ctx, a.ID, 2, now)
	if err != nil {
		t.Fatalf("TrainScout: %v", err)
	}
	payload, _ := json.Marshal(scoutQueuePayload{QueueID: sq.ID})
	if err := svc.CompleteScoutEvent(ctx, payload, sq.FinishAt); err != nil {
		t.Fatalf("CompleteScoutEvent: %v", err)
	}
	loaded, _ := svc.LoadCity(ctx, a.ID, sq.FinishAt)
	if loaded.Scouts != 2 {
		t.Fatalf("esperava 2 batedores, veio %d", loaded.Scouts)
	}

	// Envia 1 batedor a B.
	m, err := svc.SendScout(ctx, a.ID, b.ID, now)
	if err != nil {
		t.Fatalf("SendScout: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, a.ID, now)
	if loaded.Scouts != 1 || len(loaded.ScoutMissions) != 1 {
		t.Fatalf("esperava 1 batedor restante e 1 missão, veio scouts=%d missões=%d", loaded.Scouts, len(loaded.ScoutMissions))
	}
	// Defensor foi alertado (batedor a caminho).
	bReps, _ := svc.ListReports(ctx, b.ID)
	if len(bReps) != 1 || bReps[0].Type != "incoming_scout" {
		t.Fatalf("defensor deveria ter 1 alerta incoming_scout, veio %+v", bReps)
	}

	// Chegada → snapshot.
	mp, _ := json.Marshal(scoutMissionPayload{MissionID: m.ID})
	if err := svc.ResolveScoutArrivalEvent(ctx, mp, m.ArriveAt); err != nil {
		t.Fatalf("ResolveScoutArrivalEvent: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, a.ID, m.ArriveAt)
	mi := loaded.ScoutMissions[0]
	if mi.Status != "returning" || mi.Intel == nil {
		t.Fatalf("missão deveria estar 'returning' com intel: %+v", mi)
	}
	if mi.Intel.Garrison["lanceiro"] != 3 {
		t.Fatalf("intel deveria revelar 3 lanceiros, veio %d", mi.Intel.Garrison["lanceiro"])
	}
	if mi.Intel.Raidable.Matter != 500 {
		t.Fatalf("intel deveria revelar 500 de matéria saqueável, veio %v", mi.Intel.Raidable.Matter)
	}

	// Volta → batedor retorna + relatório de espionagem.
	returnAt := *mi.ReturnAt
	if err := svc.ResolveScoutReturnEvent(ctx, mp, returnAt); err != nil {
		t.Fatalf("ResolveScoutReturnEvent: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, a.ID, returnAt)
	if loaded.Scouts != 2 || len(loaded.ScoutMissions) != 0 {
		t.Fatalf("batedor deveria voltar (2) e missão encerrar, veio scouts=%d missões=%d", loaded.Scouts, len(loaded.ScoutMissions))
	}
	reps, _ := svc.ListReports(ctx, a.ID)
	if len(reps) != 1 || reps[0].Type != "scout" {
		t.Fatalf("atacante deveria ter 1 relatório scout, veio %+v", reps)
	}
}

// Sem Toca dos Batedores não dá pra enviar batedor.
func TestScoutRequiresHouse_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	a := enterTestGame(t, svc, pool, "brevali", now)
	b := enterTestGame(t, svc, pool, "brevali", now)
	_ = pool
	_, err := svc.SendScout(ctx, a.ID, b.ID, now)
	if err != ErrNoScoutHouse {
		t.Fatalf("esperava ErrNoScoutHouse, veio %v", err)
	}
}
