package config

import (
	"math"
	"testing"
)

func TestCollectPlan(t *testing.T) {
	const eps = 1e-9
	cases := []struct {
		name          string
		troops        map[string]int
		available     float64
		wantCollected float64
		wantSeconds   float64
	}{
		// 10 lanceiros: carga 250. Nó grande → enche a carga cheia (CollectFillSeconds=300s).
		{"carga cheia (nó grande)", map[string]int{"lanceiro": 10}, 1000, 250, 300},
		// 20 lanceiros: carga 500. Nó pequeno (300) → drena 300 PROPORCIONAL (300/500×300=180s).
		{"drena nó pequeno", map[string]int{"lanceiro": 20}, 300, 300, 180},
		// Misto: carga 250+200=450. Nó grande → carga cheia (300s).
		{"exército misto", map[string]int{"lanceiro": 10, "arqueiro": 10}, 1000, 450, 300},
		// Sem tropas ou nó vazio → nada.
		{"sem tropas", map[string]int{}, 1000, 0, 0},
		{"nó vazio", map[string]int{"lanceiro": 10}, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			collected, seconds := CollectPlan(tc.troops, tc.available)
			if math.Abs(collected-tc.wantCollected) > eps {
				t.Errorf("coletado = %v, quero %v", collected, tc.wantCollected)
			}
			if math.Abs(seconds-tc.wantSeconds) > eps {
				t.Errorf("segundos = %v, quero %v", seconds, tc.wantSeconds)
			}
		})
	}
}

// Alvos de combate: criatura é mais "tanque" (HP) que aldeia no mesmo nível; defesa e loot crescem
// com o nível. (Sanity da tabela CombatTargetFor.)
func TestCombatTargetFor(t *testing.T) {
	vA1, vH1, vR1 := CombatTargetFor("village", 1)
	_, cH1, cR1 := CombatTargetFor("creature", 1)
	if cH1 <= vH1 {
		t.Errorf("criatura deveria ter mais HP que aldeia no nível 1: criatura %d vs aldeia %d", cH1, vH1)
	}
	if vA1 <= 0 || vH1 <= 0 || vR1.Matter <= 0 {
		t.Errorf("aldeia nível 1 deveria ter defesa e loot positivos: atk=%d hp=%d loot=%+v", vA1, vH1, vR1)
	}
	if cR1.Matter <= 0 {
		t.Errorf("criatura nível 1 deveria render matéria: %+v", cR1)
	}
	// Escala por nível: nível 3 mais forte e mais recompensador que nível 1.
	vA3, vH3, vR3 := CombatTargetFor("village", 3)
	if !(vA3 > vA1 && vH3 > vH1 && vR3.Matter > vR1.Matter) {
		t.Errorf("aldeia nível 3 deveria superar nível 1: atk %d>%d hp %d>%d loot %v>%v", vA3, vA1, vH3, vH1, vR3.Matter, vR1.Matter)
	}
}

// Conhecimento é mais escasso: seus nós rendem MENOS que matéria/energia no mesmo nível (espelha
// a taxa de produção menor da cidade — estilo RoK). Matéria ≥ energia ≥ conhecimento.
func TestNodeAmountFor_KnowledgeScarcer(t *testing.T) {
	for _, lvl := range []int{1, 2, 3} {
		matter := NodeAmountFor("matter", lvl)
		energy := NodeAmountFor("energy", lvl)
		knowledge := NodeAmountFor("knowledge", lvl)
		if !(matter >= energy && energy > knowledge && knowledge > 0) {
			t.Errorf("nível %d: esperava matéria(%v) ≥ energia(%v) > conhecimento(%v) > 0", lvl, matter, energy, knowledge)
		}
	}
}
