// Package combat contém a lógica PURA de combate (auto-resolução determinística).
//
// Regra de arquitetura: sem I/O (sem banco, rede ou relógio). É função pura sobre dados,
// determinística e auditável (sem aleatoriedade) — testável isoladamente e reproduzível.
// A batalha tática (grade hex, turnos, IA) virá depois sobre os mesmos dados de unidade.
package combat

// Stack é um grupo de unidades idênticas do atacante.
type Stack struct {
	Key    string // identificador da unidade (unit_type)
	Attack int    // dano por unidade por rodada
	HP     int    // vida por unidade
	Count  int    // quantidade
}

// Defender é a defesa (auto-resolve) de uma província: dano por rodada e vida total.
type Defender struct {
	Attack int
	HP     int
}

// Outcome é o resultado determinístico do auto-resolve.
type Outcome struct {
	AttackerWins bool
	Survivors    map[string]int // unit_type -> sobreviventes
	Losses       map[string]int // unit_type -> perdas
}

// AutoResolve simula um combate SEM aleatoriedade: ambos os lados causam dano por rodada;
// vence quem zera o HP do oponente primeiro (menos rodadas). As baixas do atacante são
// distribuídas proporcionalmente ao HP total de cada stack.
func AutoResolve(stacks []Stack, def Defender) Outcome {
	out := Outcome{Survivors: map[string]int{}, Losses: map[string]int{}}

	totalAtk, totalHP := 0, 0
	for _, s := range stacks {
		totalAtk += s.Attack * s.Count
		totalHP += s.HP * s.Count
	}

	// Sem poder de ataque ou sem exército → derrota total.
	if totalAtk <= 0 || totalHP <= 0 {
		for _, s := range stacks {
			out.Survivors[s.Key] = 0
			out.Losses[s.Key] = s.Count
		}
		return out
	}

	roundsToKillDef := ceilDiv(def.HP, totalAtk)
	roundsToKillAtk := ceilDiv(totalHP, maxInt(def.Attack, 1))
	out.AttackerWins = roundsToKillDef <= roundsToKillAtk

	dmgToAtk := roundsToKillDef * def.Attack // dano sofrido até derrubar o defensor
	if !out.AttackerWins {
		dmgToAtk = totalHP // exército aniquilado na derrota
	}
	if dmgToAtk > totalHP {
		dmgToAtk = totalHP
	}

	for _, s := range stacks {
		stackHP := s.HP * s.Count
		stackDmg := dmgToAtk * stackHP / totalHP // proporcional ao HP do stack
		killed := stackDmg / s.HP                // unidades mortas (floor)
		if killed > s.Count {
			killed = s.Count
		}
		out.Survivors[s.Key] = s.Count - killed
		out.Losses[s.Key] = killed
	}
	return out
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
