package city

import (
	"errors"
	"testing"

	"backend/internal/db"
)

// Loop completo de SAQUE PvP: A (20 lanceiros) saqueia B (guarnição fraca, recursos desprotegidos)
// → vence, rouba o excedente limitado pela carga, B perde tropas + recursos, A volta com loot.
func TestRaidFlow_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	a := enterTestGame(t, svc, pool, "brevali", now)
	b := enterTestGame(t, svc, pool, "brevali", now)
	aUUID, _ := db.ParseUUID(a.ID)
	bUUID, _ := db.ParseUUID(b.ID)
	bPlayer, _ := db.ParseUUID(b.PlayerID)

	// Tira o escudo de novato do defensor (senão não pode ser saqueado) e dá uma guarnição fraca.
	if err := q.DropPlayerShield(ctx, bPlayer); err != nil {
		t.Fatalf("DropPlayerShield: %v", err)
	}
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: bUUID, UnitType: "lanceiro", Count: 2}); err != nil {
		t.Fatalf("AddCityTroops B: %v", err)
	}
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: aUUID, UnitType: "lanceiro", Count: 20}); err != nil {
		t.Fatalf("AddCityTroops A: %v", err)
	}

	rd, err := svc.StartRaid(ctx, a.ID, b.ID, map[string]int{"lanceiro": 20}, now)
	if err != nil {
		t.Fatalf("StartRaid: %v", err)
	}
	if rd.Status != "outbound" {
		t.Fatalf("saque deveria estar 'outbound', veio %q", rd.Status)
	}
	// Defensor vê o incoming (alerta de ataque a caminho).
	bLoaded, _ := svc.LoadCity(ctx, b.ID, now)
	if len(bLoaded.Incoming) != 1 {
		t.Fatalf("defensor deveria ver 1 ataque a caminho, veio %d", len(bLoaded.Incoming))
	}

	// Chegada → combate + saque.
	if err := svc.ResolveRaidArrival(ctx, rd.ID, rd.ArriveAt); err != nil {
		t.Fatalf("ResolveRaidArrival: %v", err)
	}
	aLoaded, _ := svc.LoadCity(ctx, a.ID, rd.ArriveAt)
	if len(aLoaded.Raids) != 1 || aLoaded.Raids[0].Status != "returning" || aLoaded.Raids[0].AttackerWon == nil || !*aLoaded.Raids[0].AttackerWon {
		t.Fatalf("saque deveria estar 'returning' e vitorioso: %+v", aLoaded.Raids)
	}
	if aLoaded.Raids[0].Loot.Matter <= 0 {
		t.Fatalf("deveria ter saqueado matéria, loot=%+v", aLoaded.Raids[0].Loot)
	}
	loot := aLoaded.Raids[0].Loot
	// Defensor perdeu os recursos roubados e a guarnição (2 lanceiros morrem contra 20).
	bLoaded, _ = svc.LoadCity(ctx, b.ID, rd.ArriveAt)
	if bLoaded.Resources.Matter != 500-loot.Matter {
		t.Fatalf("matéria do defensor: %v, quero %v", bLoaded.Resources.Matter, 500-loot.Matter)
	}
	if len(bLoaded.Troops) != 0 {
		t.Fatalf("guarnição do defensor deveria ser aniquilada: %+v", bLoaded.Troops)
	}

	// Volta → sobreviventes + loot creditados ao atacante.
	returnAt := *aLoaded.Raids[0].ReturnAt
	if err := svc.ResolveRaidReturn(ctx, rd.ID, returnAt); err != nil {
		t.Fatalf("ResolveRaidReturn: %v", err)
	}
	aLoaded, _ = svc.LoadCity(ctx, a.ID, returnAt)
	if len(aLoaded.Raids) != 0 {
		t.Fatalf("saque deveria estar encerrado: %+v", aLoaded.Raids)
	}
	if len(aLoaded.Troops) != 1 || aLoaded.Troops[0].Count <= 0 {
		t.Fatalf("sobreviventes deveriam voltar: %+v", aLoaded.Troops)
	}
	if aLoaded.Resources.Matter != 500+loot.Matter {
		t.Fatalf("matéria do atacante: %v, quero %v", aLoaded.Resources.Matter, 500+loot.Matter)
	}
	reps, _ := svc.ListReports(ctx, a.ID)
	if len(reps) != 1 || reps[0].Type != "raid_pvp" {
		t.Fatalf("atacante deveria ter 1 relatório raid_pvp, veio %+v", reps)
	}
}

// Escudo de novato BLOQUEIA o saque (defensor recém-criado).
func TestRaidShieldBlocks_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	a := enterTestGame(t, svc, pool, "brevali", now)
	b := enterTestGame(t, svc, pool, "brevali", now) // escudo de novato ativo (4 dias)
	aUUID, _ := db.ParseUUID(a.ID)
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: aUUID, UnitType: "lanceiro", Count: 10}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}
	_, err := svc.StartRaid(ctx, a.ID, b.ID, map[string]int{"lanceiro": 10}, now)
	if !errors.Is(err, ErrDefenderShielded) {
		t.Fatalf("esperava ErrDefenderShielded, veio %v", err)
	}
}
