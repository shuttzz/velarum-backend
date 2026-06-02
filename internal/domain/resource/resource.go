// Package resource contém a lógica PURA de recursos do jogo (lazy evaluation).
//
// Regra de arquitetura: este pacote NÃO faz I/O (sem banco, sem rede, sem relógio global).
// Tudo é função pura sobre dados + um `time.Time` passado pelo chamador. Isso o torna
// testável isoladamente e seguro para rodar tanto no servidor quanto (futuramente) ser
// reaproveitado no cliente para extrapolar os contadores em tempo real.
package resource

import "time"

// Amounts representa uma quantidade dos 3 recursos âncora do Velarum.
// Os nomes/visuais mudam por era (Matéria: pedra→aço→liga; Energia: lenha→plasma; etc.),
// mas a função mecânica é constante.
type Amounts struct {
	Matter    float64 `json:"matter"`
	Energy    float64 `json:"energy"`
	Knowledge float64 `json:"knowledge"`
}

// State é o estado persistido de recursos de uma cidade: o último "snapshot" (Stored),
// as taxas de produção por hora (RatePerHour), o teto de armazém (Capacity) e o instante
// em que Stored foi calculado pela última vez (UpdatedAt).
type State struct {
	Stored      Amounts
	RatePerHour Amounts
	Capacity    Amounts
	UpdatedAt   time.Time
}

// At calcula a quantidade ATUAL de recursos em `now`, sem tocar em I/O.
//
// Este é o coração do gênero: recurso = armazenado + taxa * tempo_decorrido,
// limitado pela capacidade do armazém. Uma cidade inativa há dias é calculada
// instantaneamente com uma multiplicação — sem nenhum "tick" rodando em background.
func (s State) At(now time.Time) Amounts {
	elapsed := now.Sub(s.UpdatedAt).Hours()
	if elapsed < 0 {
		elapsed = 0
	}
	return Amounts{
		Matter:    clamp(s.Stored.Matter+s.RatePerHour.Matter*elapsed, s.Capacity.Matter),
		Energy:    clamp(s.Stored.Energy+s.RatePerHour.Energy*elapsed, s.Capacity.Energy),
		Knowledge: clamp(s.Stored.Knowledge+s.RatePerHour.Knowledge*elapsed, s.Capacity.Knowledge),
	}
}

// CanAfford diz se, em `now`, a cidade tem recursos suficientes para pagar `cost`.
func (s State) CanAfford(cost Amounts, now time.Time) bool {
	cur := s.At(now)
	return cur.Matter >= cost.Matter &&
		cur.Energy >= cost.Energy &&
		cur.Knowledge >= cost.Knowledge
}

// Spend materializa o estado em `now` e debita `cost`, retornando o novo State.
// Retorna (novoState, true) em sucesso; (estado inalterado, false) se faltarem recursos.
//
// O chamador deve persistir o novoState dentro de uma transação com lock na linha da
// cidade (SELECT ... FOR UPDATE) para evitar gasto duplicado (double-spend).
func (s State) Spend(cost Amounts, now time.Time) (State, bool) {
	cur := s.At(now)
	if cur.Matter < cost.Matter || cur.Energy < cost.Energy || cur.Knowledge < cost.Knowledge {
		return s, false
	}
	s.Stored = Amounts{
		Matter:    cur.Matter - cost.Matter,
		Energy:    cur.Energy - cost.Energy,
		Knowledge: cur.Knowledge - cost.Knowledge,
	}
	s.UpdatedAt = now
	return s, true
}

// clamp limita v ao intervalo [0, max]. Se max <= 0, é tratado como "sem teto".
func clamp(v, max float64) float64 {
	if v < 0 {
		return 0
	}
	if max > 0 && v > max {
		return max
	}
	return v
}
