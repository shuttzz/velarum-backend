// Package grid contém a lógica PURA de posicionamento de edifícios na cidade
// (limites do grid e colisão). Sem I/O — testável isoladamente.
package grid

// Rect é a área ocupada por um edifício: canto (X,Y) com largura W e altura H.
type Rect struct {
	X, Y, W, H int
}

// Overlaps diz se dois retângulos se sobrepõem (em células).
func (r Rect) Overlaps(o Rect) bool {
	return r.X < o.X+o.W && o.X < r.X+r.W &&
		r.Y < o.Y+o.H && o.Y < r.Y+r.H
}

// Within diz se r cabe dentro de um grid width×height ancorado em (0,0).
func (r Rect) Within(width, height int) bool {
	if r.W <= 0 || r.H <= 0 {
		return false
	}
	return r.X >= 0 && r.Y >= 0 && r.X+r.W <= width && r.Y+r.H <= height
}

// Fits diz se r cabe no grid e não colide com nenhum retângulo ocupado.
// Ao mover um edifício, o chamador deve excluir o próprio edifício de `occupied`.
func Fits(r Rect, width, height int, occupied []Rect) bool {
	if !r.Within(width, height) {
		return false
	}
	for _, o := range occupied {
		if r.Overlaps(o) {
			return false
		}
	}
	return true
}
