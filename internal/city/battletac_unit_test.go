package city

import (
	"testing"

	"backend/internal/domain/battle"
)

func TestTilesForProvince_Deterministic(t *testing.T) {
	a := tilesForProvince("019e8edf-d709-706c-98a0-8f998095c462", nil)
	b := tilesForProvince("019e8edf-d709-706c-98a0-8f998095c462", nil)
	if len(a) != len(battleTileTypes) {
		t.Fatalf("esperava %d tiles, veio %d", len(battleTileTypes), len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("layout não determinístico para a mesma província: %+v vs %+v", a[i], b[i])
		}
	}
}

func TestTilesForProvince_PerProvinceLayout(t *testing.T) {
	a := tilesForProvince("019e8edf-d709-706c-98a0-8f998095c462", nil)
	b := tilesForProvince("019e8edf-d708-7e81-88b1-1b1150242834", nil)
	same := true
	for i := range a {
		if a[i].Pos != b[i].Pos {
			same = false
			break
		}
	}
	if same {
		t.Fatal("províncias diferentes deveriam render layouts diferentes (posições)")
	}
}

func TestTilesForProvince_NoSpawnColumnsNoDup(t *testing.T) {
	tiles := tilesForProvince("019e8edf-d709-706c-98a0-8f998095c462", nil)
	seen := map[[2]int]bool{}
	for _, tl := range tiles {
		if tl.Pos.Q <= 0 || tl.Pos.Q >= battleW-1 {
			t.Fatalf("tile na coluna de spawn (q=%d): %+v", tl.Pos.Q, tl)
		}
		if tl.Pos.R < 0 || tl.Pos.R >= battleH {
			t.Fatalf("tile fora da grade: %+v", tl)
		}
		key := [2]int{tl.Pos.Q, tl.Pos.R}
		if seen[key] {
			t.Fatalf("posição duplicada: %+v", tl.Pos)
		}
		seen[key] = true
	}
}

// Nenhum tile pode nascer numa casa ocupada por unidade (garante que a Fenda, que causa dano,
// nunca surja sob uma tropa no início). Ocupa TODAS as casas candidatas de uma coluna interna
// e confirma que nenhum tile caiu nelas.
func TestTilesForProvince_ExcludesOccupied(t *testing.T) {
	occupied := make([]battle.Hex, 0, battleH)
	for r := 0; r < battleH; r++ {
		occupied = append(occupied, battle.Hex{Q: 2, R: r})
	}
	tiles := tilesForProvince("019e8edf-d709-706c-98a0-8f998095c462", occupied)
	for _, tl := range tiles {
		if tl.Pos.Q == 2 {
			t.Fatalf("tile nasceu em casa ocupada por unidade: %+v", tl.Pos)
		}
	}
}
