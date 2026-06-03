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
// as taxas de produção por hora (RatePerHour), a PARCELA PROTEGIDA contra saque (Capacity)
// e o instante em que Stored foi calculado pela última vez (UpdatedAt).
//
// IMPORTANTE: Capacity NÃO é um teto de produção — os recursos sobem sem limite. Ela é só o
// quanto fica ABRIGADO de saque; o excedente (acima de Capacity) é saqueável no PvP. O
// armazém (Celeiro) aumenta essa parcela protegida. Cf. GDD §5/§11.
type State struct {
	Stored      Amounts
	RatePerHour Amounts
	Capacity    Amounts // parcela protegida contra saque (não é teto de acumulação)
	UpdatedAt   time.Time
}

// At calcula a quantidade ATUAL de recursos em `now`, sem tocar em I/O.
//
// Este é o coração do gênero: recurso = armazenado + taxa * tempo_decorrido. Sem teto de
// acumulação (o estoque sobe livremente). Uma cidade inativa há dias é calculada
// instantaneamente com uma multiplicação — sem nenhum "tick" rodando em background.
func (s State) At(now time.Time) Amounts {
	elapsed := now.Sub(s.UpdatedAt).Hours()
	if elapsed < 0 {
		elapsed = 0
	}
	return Amounts{
		Matter:    floor0(s.Stored.Matter + s.RatePerHour.Matter*elapsed),
		Energy:    floor0(s.Stored.Energy + s.RatePerHour.Energy*elapsed),
		Knowledge: floor0(s.Stored.Knowledge + s.RatePerHour.Knowledge*elapsed),
	}
}

// Raidable devolve, em `now`, o excedente saqueável por recurso (estoque acima da parcela
// protegida pela Capacity). Base para a mecânica de saque do PvP.
func (s State) Raidable(now time.Time) Amounts {
	cur := s.At(now)
	return Amounts{
		Matter:    floor0(cur.Matter - s.Capacity.Matter),
		Energy:    floor0(cur.Energy - s.Capacity.Energy),
		Knowledge: floor0(cur.Knowledge - s.Capacity.Knowledge),
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

// floor0 limita v a um piso de 0 (recursos nunca ficam negativos).
func floor0(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}
