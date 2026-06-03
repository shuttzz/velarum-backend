// Package battle contém o motor PURO da batalha tática (grade hexagonal, turnos).
//
// Regra de arquitetura: sem I/O e DETERMINÍSTICO (sem aleatoriedade) — toda a lógica é função
// pura sobre o estado `Battle`, serializável (JSON), auditável e reproduzível. O atacante é
// controlado pelo jogador (via Act); o defensor pela IA (AITurn). Cf. GDD §9.
package battle

import "errors"

// Side identifica o lado de uma unidade/turno.
type Side string

const (
	Attacker Side = "attacker"
	Defender Side = "defender"
)

// Erros de validação de ação (o servidor mapeia para 4xx).
var (
	ErrBattleOver   = errors.New("batalha encerrada")
	ErrNoUnit       = errors.New("unidade inexistente ou morta")
	ErrNotYourTurn  = errors.New("não é o turno desta unidade")
	ErrAlreadyActed = errors.New("unidade já agiu neste turno")
	ErrOutOfBounds  = errors.New("destino fora da grade")
	ErrTooFar       = errors.New("destino além do alcance de movimento")
	ErrOccupied     = errors.New("destino ocupado")
	ErrNoTarget     = errors.New("alvo inexistente ou morto")
	ErrFriendly     = errors.New("não é possível atacar aliado")
	ErrOutOfRange   = errors.New("alvo fora do alcance de ataque")
)

// Hex é uma coordenada axial.
type Hex struct {
	Q int `json:"q"`
	R int `json:"r"`
}

// Distance é a distância hexagonal (cube distance a partir de coords axiais).
func Distance(a, b Hex) int {
	return (abs(a.Q-b.Q) + abs(a.Q+a.R-b.Q-b.R) + abs(a.R-b.R)) / 2
}

// Unit é um "stack" de tropas idênticas posicionado na grade. HP é um POOL: a quantidade de
// tropas vivas (Count) é derivada de HP/HpPer.
type Unit struct {
	ID      string `json:"id"`
	Owner   Side   `json:"owner"`
	Key     string `json:"key"` // unit_type (stats/visual)
	Hp      int    `json:"hp"`  // pool de vida atual
	HpPer   int    `json:"hp_per"`
	Attack  int    `json:"attack"`
	Defense int    `json:"defense"`
	Move    int    `json:"move"`  // alcance de movimento (hexes/turno)
	Range   int    `json:"range"` // alcance de ataque (hexes)
	Pos     Hex    `json:"pos"`
}

// Count é o número de tropas vivas no stack.
func (u *Unit) Count() int {
	if u.Hp <= 0 || u.HpPer <= 0 {
		return 0
	}
	return (u.Hp + u.HpPer - 1) / u.HpPer
}

// Alive indica se o stack ainda tem tropas.
func (u *Unit) Alive() bool { return u.Hp > 0 }

// Battle é o estado completo de uma batalha (serializável).
type Battle struct {
	W         int             `json:"w"`
	H         int             `json:"h"`
	Units     []*Unit         `json:"units"`
	Turn      Side            `json:"turn"`
	Round     int             `json:"round"`
	MaxRounds int             `json:"max_rounds"`
	Acted     map[string]bool `json:"acted"`
	Over      bool            `json:"over"`
	Winner    Side            `json:"winner"`
}

// Act executa a ação de uma unidade do lado atual: mover (opcional) e/ou atacar (opcional).
// Marca a unidade como tendo agido neste turno. Valida limites, alcance e ocupação.
func (b *Battle) Act(unitID string, moveTo *Hex, targetID string) error {
	if b.Over {
		return ErrBattleOver
	}
	u := b.unit(unitID)
	if u == nil || !u.Alive() {
		return ErrNoUnit
	}
	if u.Owner != b.Turn {
		return ErrNotYourTurn
	}
	if b.Acted[unitID] {
		return ErrAlreadyActed
	}

	pos := u.Pos
	if moveTo != nil {
		if !b.inBounds(*moveTo) {
			return ErrOutOfBounds
		}
		if Distance(pos, *moveTo) > u.Move {
			return ErrTooFar
		}
		if occ := b.unitAt(*moveTo); occ != nil && occ.ID != u.ID {
			return ErrOccupied
		}
		pos = *moveTo
	}
	if targetID != "" {
		t := b.unit(targetID)
		if t == nil || !t.Alive() {
			return ErrNoTarget
		}
		if t.Owner == u.Owner {
			return ErrFriendly
		}
		if Distance(pos, t.Pos) > u.Range {
			return ErrOutOfRange
		}
		dealDamage(u, t)
	}

	u.Pos = pos
	if b.Acted == nil {
		b.Acted = map[string]bool{}
	}
	b.Acted[unitID] = true
	b.checkOver()
	return nil
}

// EndTurn passa o turno ao outro lado (e avança o round ao voltar ao atacante). Aplica o teto
// de rounds: estourado, encerra por maioria de HP.
func (b *Battle) EndTurn() {
	if b.Over {
		return
	}
	if b.Turn == Attacker {
		b.Turn = Defender
	} else {
		b.Turn = Attacker
		b.Round++
	}
	b.Acted = map[string]bool{}
	if b.Round >= b.MaxRounds {
		b.finishByHp()
	}
}

// AITurn joga o turno do DEFENSOR de forma determinística: cada unidade aproxima-se do inimigo
// mais próximo e ataca o de menor HP ao alcance. Encerra o turno ao final.
func (b *Battle) AITurn() {
	if b.Over || b.Turn != Defender {
		return
	}
	for _, u := range b.Units {
		if u.Owner != Defender || !u.Alive() || b.Acted[u.ID] {
			continue
		}
		enemy := b.nearestEnemy(u)
		if enemy == nil {
			continue
		}
		moveTo := b.bestStep(u, enemy.Pos)
		from := u.Pos
		if moveTo != nil {
			from = *moveTo
		}
		target := ""
		if Distance(from, enemy.Pos) <= u.Range {
			target = enemy.ID
		}
		_ = b.Act(u.ID, moveTo, target)
		if b.Over {
			break
		}
	}
	b.EndTurn()
}

// --- internos ---

func dealDamage(att, def *Unit) {
	per := att.Attack - def.Defense
	if per < 1 {
		per = 1
	}
	def.Hp -= att.Count() * per
	if def.Hp < 0 {
		def.Hp = 0
	}
}

func (b *Battle) unit(id string) *Unit {
	for _, u := range b.Units {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func (b *Battle) unitAt(h Hex) *Unit {
	for _, u := range b.Units {
		if u.Alive() && u.Pos == h {
			return u
		}
	}
	return nil
}

func (b *Battle) inBounds(h Hex) bool {
	return h.Q >= 0 && h.Q < b.W && h.R >= 0 && h.R < b.H
}

func (b *Battle) totalHp(side Side) int {
	sum := 0
	for _, u := range b.Units {
		if u.Owner == side {
			sum += u.Hp
		}
	}
	return sum
}

func (b *Battle) checkOver() {
	if b.totalHp(Attacker) == 0 {
		b.Over, b.Winner = true, Defender
	} else if b.totalHp(Defender) == 0 {
		b.Over, b.Winner = true, Attacker
	}
}

func (b *Battle) finishByHp() {
	b.Over = true
	if b.totalHp(Attacker) > b.totalHp(Defender) {
		b.Winner = Attacker
	} else {
		b.Winner = Defender // empate favorece o defensor
	}
}

// nearestEnemy: inimigo vivo mais próximo (desempate determinístico pela ordem das unidades).
func (b *Battle) nearestEnemy(u *Unit) *Unit {
	var best *Unit
	bestD := 1 << 30
	for _, e := range b.Units {
		if e.Owner == u.Owner || !e.Alive() {
			continue
		}
		if d := Distance(u.Pos, e.Pos); d < bestD {
			bestD, best = d, e
		}
	}
	return best
}

// bestStep: melhor hex (até Move de distância, livre, dentro da grade) que mais aproxima de
// `goal`. Retorna nil se já está o mais perto possível (não se move). Determinístico: varre a
// grade em ordem (q, depois r) e fica com a menor distância ao alvo.
func (b *Battle) bestStep(u *Unit, goal Hex) *Hex {
	bestPos := u.Pos
	bestD := Distance(u.Pos, goal)
	for q := 0; q < b.W; q++ {
		for r := 0; r < b.H; r++ {
			h := Hex{Q: q, R: r}
			if Distance(u.Pos, h) > u.Move {
				continue
			}
			if occ := b.unitAt(h); occ != nil && occ.ID != u.ID {
				continue
			}
			if d := Distance(h, goal); d < bestD {
				bestD, bestPos = d, h
			}
		}
	}
	if bestPos == u.Pos {
		return nil
	}
	return &Hex{Q: bestPos.Q, R: bestPos.R}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
