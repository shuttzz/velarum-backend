package grid

import "testing"

func TestOverlaps(t *testing.T) {
	a := Rect{X: 0, Y: 0, W: 2, H: 2}
	if !a.Overlaps(Rect{X: 1, Y: 1, W: 2, H: 2}) {
		t.Error("retângulos sobrepostos deveriam colidir")
	}
	if a.Overlaps(Rect{X: 2, Y: 0, W: 2, H: 2}) {
		t.Error("retângulos adjacentes (à direita) não sobrepõem")
	}
	if a.Overlaps(Rect{X: 0, Y: 2, W: 1, H: 1}) {
		t.Error("retângulos adjacentes (abaixo) não sobrepõem")
	}
}

func TestWithin(t *testing.T) {
	if !(Rect{X: 0, Y: 0, W: 2, H: 2}).Within(8, 6) {
		t.Error("deveria caber no grid")
	}
	if (Rect{X: 7, Y: 0, W: 2, H: 2}).Within(8, 6) {
		t.Error("x+w > largura: não cabe")
	}
	if (Rect{X: 0, Y: 5, W: 2, H: 2}).Within(8, 6) {
		t.Error("y+h > altura: não cabe")
	}
	if (Rect{X: -1, Y: 0, W: 1, H: 1}).Within(8, 6) {
		t.Error("x negativo: não cabe")
	}
}

func TestFits(t *testing.T) {
	occupied := []Rect{{X: 2, Y: 2, W: 2, H: 2}}
	if !Fits(Rect{X: 0, Y: 0, W: 1, H: 1}, 8, 6, occupied) {
		t.Error("célula livre deveria caber")
	}
	if Fits(Rect{X: 2, Y: 2, W: 1, H: 1}, 8, 6, occupied) {
		t.Error("colide com edifício ocupado")
	}
	if Fits(Rect{X: 7, Y: 5, W: 2, H: 2}, 8, 6, occupied) {
		t.Error("fora dos limites do grid")
	}
}
