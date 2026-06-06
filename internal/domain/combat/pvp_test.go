package combat

import "testing"

// Stats espelhando Era1Units (lanceiro atk 10/hp 30).
func lanceiros(n int) Stack { return Stack{Key: "lanceiro", Attack: 10, HP: 30, Count: n} }

func TestResolvePvP_AttackerWins(t *testing.T) {
	// 20 lanceiros vs guarnição de 5, sem torre/muralha → atacante vence, defensor aniquilado.
	out := ResolvePvP([]Stack{lanceiros(20)}, []Stack{lanceiros(5)}, 0, 0)
	if !out.AttackerWins {
		t.Fatal("atacante deveria vencer")
	}
	if out.DefenderSurvivors["lanceiro"] != 0 {
		t.Fatalf("guarnição deveria ser aniquilada, sobraram %d", out.DefenderSurvivors["lanceiro"])
	}
	if out.AttackerSurvivors["lanceiro"] == 0 || out.AttackerSurvivors["lanceiro"] > 20 {
		t.Fatalf("atacante deveria sobreviver (1..20), veio %d", out.AttackerSurvivors["lanceiro"])
	}
}

func TestResolvePvP_WallMakesWinCostlier(t *testing.T) {
	// Mesma luta; a MURALHA (HP) não impede a vitória, mas faz o atacante perder MAIS tropas.
	noWall := ResolvePvP([]Stack{lanceiros(20)}, []Stack{lanceiros(5)}, 0, 0)
	withWall := ResolvePvP([]Stack{lanceiros(20)}, []Stack{lanceiros(5)}, 0, 600)
	if !withWall.AttackerWins {
		t.Fatal("atacante ainda deveria vencer (muralha não dá ataque)")
	}
	if withWall.AttackerLosses["lanceiro"] <= noWall.AttackerLosses["lanceiro"] {
		t.Fatalf("muralha deveria encarecer a vitória: com muralha %d vs sem %d", withWall.AttackerLosses["lanceiro"], noWall.AttackerLosses["lanceiro"])
	}
}

func TestResolvePvP_TowerCanFlipToDefender(t *testing.T) {
	// Atacante fraco vs guarnição igual + TORRE forte → defensor vence; atacante aniquilado.
	out := ResolvePvP([]Stack{lanceiros(5)}, []Stack{lanceiros(5)}, 100, 0)
	if out.AttackerWins {
		t.Fatal("com torre forte o defensor deveria vencer")
	}
	if out.AttackerSurvivors["lanceiro"] != 0 {
		t.Fatalf("atacante deveria ser aniquilado, sobraram %d", out.AttackerSurvivors["lanceiro"])
	}
	if out.DefenderSurvivors["lanceiro"] == 0 {
		t.Fatal("defensor deveria sobreviver (perdas proporcionais)")
	}
}

func TestResolvePvP_EmptyAttacker(t *testing.T) {
	out := ResolvePvP(nil, []Stack{lanceiros(5)}, 0, 0)
	if out.AttackerWins {
		t.Fatal("atacante vazio não pode vencer")
	}
	if out.DefenderSurvivors["lanceiro"] != 5 {
		t.Fatalf("defensor deveria ficar intacto, veio %d", out.DefenderSurvivors["lanceiro"])
	}
}
