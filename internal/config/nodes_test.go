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
		// 10 lanceiros: carga 250, taxa 5/s. Nó grande → leva a carga cheia (50s).
		{"carga cheia (nó grande)", map[string]int{"lanceiro": 10}, 1000, 250, 50},
		// 20 lanceiros: carga 500, taxa 10/s. Nó pequeno (300) → drena 300 mais rápido (30s).
		{"drena nó pequeno", map[string]int{"lanceiro": 20}, 300, 300, 30},
		// Misto: carga 250+200=450, taxa 5+4=9/s. Encher carga cheia ≈ 50s.
		{"exército misto", map[string]int{"lanceiro": 10, "arqueiro": 10}, 1000, 450, 50},
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
