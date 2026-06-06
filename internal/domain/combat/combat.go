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

// PvPOutcome é o resultado determinístico de um combate de SAQUE (atacante vs defensor real).
// Diferente de AutoResolve (defensor NPC sem baixas rastreadas), aqui AMBOS os lados perdem tropas.
type PvPOutcome struct {
	AttackerWins      bool
	AttackerSurvivors map[string]int
	AttackerLosses    map[string]int
	DefenderSurvivors map[string]int
	DefenderLosses    map[string]int
}

// ResolvePvP resolve um saque: tropas do atacante vs guarnição do defensor + estruturas. A Torre
// soma ATAQUE defensivo (towerAttack); a Muralha soma HP (wallHP) que ABSORVE dano (reduz as baixas
// da guarnição, sem ser destruída). Determinístico: vence quem zera o oponente em menos rodadas; o
// perdedor é aniquilado, o vencedor perde proporcional. Sem aleatoriedade.
func ResolvePvP(attacker, defender []Stack, towerAttack, wallHP int) PvPOutcome {
	out := PvPOutcome{
		AttackerSurvivors: map[string]int{}, AttackerLosses: map[string]int{},
		DefenderSurvivors: map[string]int{}, DefenderLosses: map[string]int{},
	}
	atkAtk, atkHP := stackTotals(attacker)
	defTroopAtk, defTroopHP := stackTotals(defender)
	defAtk := defTroopAtk + towerAttack
	defHP := defTroopHP + wallHP

	// Atacante sem poder/exército → derrota total; defensor intacto.
	if atkAtk <= 0 || atkHP <= 0 {
		for _, s := range attacker {
			out.AttackerLosses[s.Key] = s.Count
		}
		for _, s := range defender {
			out.DefenderSurvivors[s.Key] = s.Count
		}
		return out
	}
	// Defensor sem NENHUMA defesa (sem guarnição, torre nem muralha) → atacante entra sem baixas.
	if defAtk <= 0 && defHP <= 0 {
		out.AttackerWins = true
		for _, s := range attacker {
			out.AttackerSurvivors[s.Key] = s.Count
		}
		return out
	}

	rA := ceilDiv(defHP, atkAtk)            // rodadas p/ o atacante quebrar o defensor
	rD := ceilDiv(atkHP, maxInt(defAtk, 1)) // rodadas p/ o defensor quebrar o atacante
	out.AttackerWins = rA <= rD
	rounds := rA
	if rD < rA {
		rounds = rD
	}
	distributeLosses(attacker, minInt(rounds*defAtk, atkHP), atkHP, out.AttackerSurvivors, out.AttackerLosses)
	// Defensor: dano distribuído sobre o pool TOTAL (guarnição + muralha) → a muralha absorve sua
	// fração e poupa tropas (totalHP = defHP inclui wallHP, mas a muralha não está nos stacks).
	distributeLosses(defender, minInt(rounds*atkAtk, defHP), defHP, out.DefenderSurvivors, out.DefenderLosses)
	return out
}

func stackTotals(stacks []Stack) (atk, hp int) {
	for _, s := range stacks {
		atk += s.Attack * s.Count
		hp += s.HP * s.Count
	}
	return atk, hp
}

func distributeLosses(stacks []Stack, dmg, totalHP int, surv, loss map[string]int) {
	for _, s := range stacks {
		killed := 0
		if totalHP > 0 && s.HP > 0 {
			killed = (dmg * (s.HP * s.Count) / totalHP) / s.HP
		}
		if killed > s.Count {
			killed = s.Count
		}
		surv[s.Key] = s.Count - killed
		loss[s.Key] = killed
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
