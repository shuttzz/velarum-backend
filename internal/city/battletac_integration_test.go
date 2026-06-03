package city

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/internal/db"
	"backend/internal/domain/battle"
	"backend/internal/pg"
)

// TestBattleFlow_Integration cobre o fluxo da batalha tática ponta a ponta:
// start → mover/atacar turnos → vitória → conquista + recompensa + sobreviventes + relatório.
func TestBattleFlow_Integration(t *testing.T) {
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

	// Semeia 20 lanceiros na guarnição (poder de sobra contra a província mais fraca do anel 1).
	cityUUID, _ := db.ParseUUID(c.ID)
	if err := db.New(pool).AddCityTroops(ctx, db.AddCityTroopsParams{CityID: cityUUID, UnitType: "lanceiro", Count: 20}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}

	// Escolhe a província mais fraca (menor DefHP) para garantir a vitória do atacante.
	provs, err := svc.ListProvinces(ctx, c.ID, now)
	if err != nil {
		t.Fatalf("ListProvinces: %v", err)
	}
	target := provs[0]
	for _, p := range provs {
		if p.DefHP < target.DefHP {
			target = p
		}
	}

	// Inicia a batalha tática enviando os 20 lanceiros.
	view, err := svc.StartBattle(ctx, c.ID, target.ID, map[string]int{"lanceiro": 20}, now)
	if err != nil {
		t.Fatalf("StartBattle: %v", err)
	}
	if view.Status != "active" || view.State == nil {
		t.Fatalf("batalha deveria iniciar ativa com estado: %+v", view)
	}
	// Durante a batalha a guarnição fica vazia (tropas empenhadas) e a cidade aponta a batalha ativa.
	loaded, _ := svc.LoadCity(ctx, c.ID, now)
	if len(loaded.Troops) != 0 {
		t.Fatalf("guarnição deveria estar vazia durante a batalha: %+v", loaded.Troops)
	}
	if loaded.ActiveBattleID != view.ID {
		t.Fatalf("active_battle_id deveria apontar a batalha: %q vs %q", loaded.ActiveBattleID, view.ID)
	}

	// Não pode haver duas batalhas ativas ao mesmo tempo.
	if _, err := svc.StartBattle(ctx, c.ID, target.ID, map[string]int{"lanceiro": 1}, now); err == nil {
		t.Fatal("StartBattle deveria falhar com batalha já ativa")
	}

	// Joga os turnos: a cada turno, ataca se em alcance, senão dá um passo na direção do defensor;
	// depois encerra o turno (roda a IA). Limita a rondas para não travar caso a vitória não venha.
	const attackerID = "a:lanceiro"
	for round := 0; round < 30 && !view.State.Over; round++ {
		atk := findUnit(view.State, attackerID)
		def := defenderUnit(view.State)
		if atk == nil || def == nil {
			break
		}
		if battle.Distance(atk.Pos, def.Pos) <= atk.Range {
			view, err = svc.BattleAct(ctx, c.ID, view.ID, attackerID, nil, def.ID, now)
		} else {
			step := stepToward(view.State, atk.Pos, def.Pos)
			view, err = svc.BattleAct(ctx, c.ID, view.ID, attackerID, &step, "", now)
		}
		if err != nil {
			t.Fatalf("BattleAct (round %d): %v", round, err)
		}
		if view.State.Over {
			break
		}
		view, err = svc.BattleEndTurn(ctx, c.ID, view.ID, now)
		if err != nil {
			t.Fatalf("BattleEndTurn (round %d): %v", round, err)
		}
	}

	if !view.State.Over || view.State.Winner != battle.Attacker {
		t.Fatalf("atacante deveria vencer: over=%v winner=%q", view.State.Over, view.State.Winner)
	}
	if view.Status != "resolved" {
		t.Fatalf("batalha deveria estar resolvida: %q", view.Status)
	}

	// Pós-batalha: província conquistada + recompensa + sobreviventes na guarnição + relatório.
	provs, _ = svc.ListProvinces(ctx, c.ID, now)
	var conquered *Province
	for i := range provs {
		if provs[i].ID == target.ID {
			conquered = &provs[i]
		}
	}
	if conquered == nil || conquered.Status != "conquered" {
		t.Fatalf("província deveria estar conquistada: %+v", conquered)
	}

	loaded, _ = svc.LoadCity(ctx, c.ID, now)
	if loaded.ActiveBattleID != "" {
		t.Fatalf("não deveria haver batalha ativa após resolver: %q", loaded.ActiveBattleID)
	}
	if wantMatter := 500.0 + target.Reward.Matter; loaded.Resources.Matter != wantMatter {
		t.Fatalf("recompensa não aplicada: matéria = %v, quero %v", loaded.Resources.Matter, wantMatter)
	}
	survivors := 0
	for _, tr := range loaded.Troops {
		if tr.UnitType == "lanceiro" {
			survivors = tr.Count
		}
	}
	if survivors <= 0 || survivors > 20 {
		t.Fatalf("sobreviventes deveriam voltar à guarnição (1..20 lanceiros): %d", survivors)
	}

	reps, err := svc.ListReports(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(reps) != 1 || reps[0].Type != "battle" {
		t.Fatalf("esperava 1 relatório de batalha, veio %+v", reps)
	}
}

func findUnit(b *battle.Battle, id string) *battle.Unit {
	for _, u := range b.Units {
		if u.ID == id && u.Hp > 0 {
			return u
		}
	}
	return nil
}

func defenderUnit(b *battle.Battle) *battle.Unit {
	for _, u := range b.Units {
		if u.Owner == battle.Defender && u.Hp > 0 {
			return u
		}
	}
	return nil
}

// stepToward escolhe o vizinho hex (dentro da grade, não ocupado pelo alvo) que mais reduz a
// distância até o destino — passo a passo greedy para conduzir o atacante até o defensor.
func stepToward(b *battle.Battle, from, to battle.Hex) battle.Hex {
	dirs := []battle.Hex{{Q: 1, R: 0}, {Q: 1, R: -1}, {Q: 0, R: -1}, {Q: -1, R: 0}, {Q: -1, R: 1}, {Q: 0, R: 1}}
	best := from
	bestDist := battle.Distance(from, to)
	for _, d := range dirs {
		cand := battle.Hex{Q: from.Q + d.Q, R: from.R + d.R}
		if cand.Q < 0 || cand.Q >= b.W || cand.R < 0 || cand.R >= b.H {
			continue
		}
		if cand == to {
			continue // ocupado pelo defensor
		}
		if dist := battle.Distance(cand, to); dist < bestDist {
			best, bestDist = cand, dist
		}
	}
	return best
}
