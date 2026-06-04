package config

import "testing"

func TestPlaceEra1Provinces(t *testing.T) {
	a := PlaceEra1Provinces(12345)
	b := PlaceEra1Provinces(12345)
	if len(a) != len(Era1Provinces) {
		t.Fatalf("esperava %d posições, veio %d", len(Era1Provinces), len(a))
	}
	// Determinístico: mesma seed → mesmo layout.
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("layout não determinístico p/ a mesma seed em %d: %+v vs %+v", i, a[i], b[i])
		}
	}
	// Distintas, fora da capital, e a pelo menos 2 hexes.
	seen := map[ProvCoord]bool{}
	for i, c := range a {
		if c.Q == 0 && c.R == 0 {
			t.Fatalf("província %d caiu na capital (0,0)", i)
		}
		if seen[c] {
			t.Fatalf("posição duplicada: %+v", c)
		}
		seen[c] = true
		if d := HexDistance(0, 0, c.Q, c.R); d < 2 {
			t.Fatalf("província %d perto demais (dist %d)", i, d)
		}
	}
	// Dificuldade cresce → mais longe: a última fica mais distante que a primeira.
	if HexDistance(0, 0, a[len(a)-1].Q, a[len(a)-1].R) <= HexDistance(0, 0, a[0].Q, a[0].R) {
		t.Fatal("a última província deveria ficar mais longe que a primeira")
	}
}

func TestPlaceEra1Provinces_PerSeed(t *testing.T) {
	// Seeds diferentes → layouts diferentes (mundo único por jogador).
	a := PlaceEra1Provinces(1)
	b := PlaceEra1Provinces(2)
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("seeds diferentes deveriam gerar layouts diferentes")
	}
}
