package battle

import "testing"

func TestDistance(t *testing.T) {
	if d := Distance(Hex{0, 0}, Hex{0, 0}); d != 0 {
		t.Fatalf("dist mesma célula = %d, quero 0", d)
	}
	if d := Distance(Hex{0, 0}, Hex{2, 0}); d != 2 {
		t.Fatalf("dist (0,0)->(2,0) = %d, quero 2", d)
	}
}

func newBattle() *Battle {
	return &Battle{
		W: 6, H: 6, Turn: Attacker, MaxRounds: 12, Acted: map[string]bool{},
		Units: []*Unit{
			{ID: "a1", Owner: Attacker, Key: "lanceiro", Hp: 300, HpPer: 30, Attack: 10, Defense: 8, Move: 1, Range: 1, Pos: Hex{0, 0}},
			{ID: "d1", Owner: Defender, Key: "def", Hp: 60, HpPer: 30, Attack: 8, Defense: 2, Move: 1, Range: 1, Pos: Hex{1, 0}},
		},
	}
}

func TestActAttackReducesHp(t *testing.T) {
	b := newBattle()
	// a1 (10 stacks, atk 10) ataca d1 (def 2) → 10*(10-2)=80 de dano → d1 (hp 60) morre.
	if err := b.Act("a1", nil, "d1"); err != nil {
		t.Fatalf("Act: %v", err)
	}
	if b.unit("d1").Alive() {
		t.Fatalf("d1 deveria ter morrido, hp=%d", b.unit("d1").Hp)
	}
	if !b.Over || b.Winner != Attacker {
		t.Fatalf("batalha deveria acabar com vitória do atacante: over=%v winner=%s", b.Over, b.Winner)
	}
}

func TestActValidations(t *testing.T) {
	b := newBattle()
	if err := b.Act("a1", &Hex{5, 5}, ""); err != ErrTooFar {
		t.Fatalf("mover longe demais: %v", err)
	}
	if err := b.Act("d1", nil, "a1"); err != ErrNotYourTurn {
		t.Fatalf("agir fora do turno: %v", err)
	}
	// Ataque fora de alcance: move d1? não — testa atacante atacando alvo distante.
	b2 := newBattle()
	b2.unit("d1").Pos = Hex{4, 0} // longe
	if err := b2.Act("a1", nil, "d1"); err != ErrOutOfRange {
		t.Fatalf("atacar fora de alcance: %v", err)
	}
	// Já agiu.
	b3 := newBattle()
	b3.unit("d1").Pos = Hex{3, 0}
	_ = b3.Act("a1", nil, "")
	if err := b3.Act("a1", nil, ""); err != ErrAlreadyActed {
		t.Fatalf("agir duas vezes: %v", err)
	}
}

func TestAITurnAttacks(t *testing.T) {
	b := newBattle()
	// Enfraquece a1 e o defensor para a1 NÃO morrer mas levar dano.
	b.unit("d1").Hp = 300 // defensor robusto
	b.unit("a1").Defense = 0
	// Turno do atacante: não faz nada de letal; passa.
	b.EndTurn() // → defender
	hpBefore := b.unit("a1").Hp
	b.AITurn()
	if b.unit("a1").Hp >= hpBefore {
		t.Fatalf("IA do defensor deveria ter causado dano: antes=%d depois=%d", hpBefore, b.unit("a1").Hp)
	}
	if b.Turn != Attacker {
		t.Fatalf("após AITurn o turno deve voltar ao atacante, veio %s", b.Turn)
	}
}

func TestAIMovesTowardEnemy(t *testing.T) {
	b := newBattle()
	b.unit("d1").Pos = Hex{5, 0} // longe, fora de alcance
	b.unit("d1").Hp = 300
	b.EndTurn() // → defender
	b.AITurn()
	// O defensor deve ter se aproximado (q menor que 5).
	if b.unit("d1").Pos.Q >= 5 {
		t.Fatalf("IA deveria aproximar-se; pos=%+v", b.unit("d1").Pos)
	}
}

func TestDeterminismo(t *testing.T) {
	run := func() (Side, int) {
		b := newBattle()
		b.unit("d1").Hp = 300
		// Sequência roteirizada: atacante ataca, encerra; IA joga; repete alguns rounds.
		for i := 0; i < 20 && !b.Over; i++ {
			if b.Turn == Attacker {
				_ = b.Act("a1", nil, "d1")
				b.EndTurn()
			} else {
				b.AITurn()
			}
		}
		return b.Winner, b.totalHp(Attacker)
	}
	w1, hp1 := run()
	w2, hp2 := run()
	if w1 != w2 || hp1 != hp2 {
		t.Fatalf("não determinístico: (%s,%d) vs (%s,%d)", w1, hp1, w2, hp2)
	}
}
