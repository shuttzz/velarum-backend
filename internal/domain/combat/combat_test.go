package combat

import "testing"

func TestAutoResolveVitoriaComBaixas(t *testing.T) {
	// 10 lanceiros (atk 10, hp 30) vs defesa fraca → vitória.
	stacks := []Stack{{Key: "lanceiro", Attack: 10, HP: 30, Count: 10}}
	out := AutoResolve(stacks, Defender{Attack: 20, HP: 100})
	if !out.AttackerWins {
		t.Fatal("esperava vitória do atacante")
	}
	// totalAtk=100, def.HP=100 → 1 rodada; dano ao atacante = 1*20 = 20 → 0 baixas (20<30).
	if out.Losses["lanceiro"] != 0 || out.Survivors["lanceiro"] != 10 {
		t.Fatalf("baixas inesperadas: %+v / %+v", out.Losses, out.Survivors)
	}
}

func TestAutoResolveDerrotaQuandoFraco(t *testing.T) {
	// 1 lanceiro vs defesa forte → derrota e aniquilação.
	stacks := []Stack{{Key: "lanceiro", Attack: 10, HP: 30, Count: 1}}
	out := AutoResolve(stacks, Defender{Attack: 100, HP: 1000})
	if out.AttackerWins {
		t.Fatal("esperava derrota do atacante")
	}
	if out.Survivors["lanceiro"] != 0 || out.Losses["lanceiro"] != 1 {
		t.Fatalf("na derrota o exército deve ser aniquilado: %+v", out)
	}
}

func TestAutoResolveExercitoVazio(t *testing.T) {
	out := AutoResolve(nil, Defender{Attack: 10, HP: 10})
	if out.AttackerWins {
		t.Fatal("exército vazio não vence")
	}
}

func TestAutoResolveDeterministico(t *testing.T) {
	stacks := []Stack{
		{Key: "lanceiro", Attack: 10, HP: 30, Count: 8},
		{Key: "arqueiro", Attack: 14, HP: 20, Count: 6},
	}
	def := Defender{Attack: 40, HP: 300}
	a := AutoResolve(stacks, def)
	b := AutoResolve(stacks, def)
	if a.AttackerWins != b.AttackerWins ||
		a.Survivors["lanceiro"] != b.Survivors["lanceiro"] ||
		a.Survivors["arqueiro"] != b.Survivors["arqueiro"] {
		t.Fatalf("auto-resolve não determinístico: %+v vs %+v", a, b)
	}
}
