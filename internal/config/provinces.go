package config

import (
	"math"
	"math/rand"

	"backend/internal/domain/resource"
)

// MarchSecondsPerHex é o tempo de marcha (segundos) por HEX de distância, em CADA trecho (ida e
// volta separados). Regiões estão espalhadas pelo mapa; a marcha é proporcional à distância hex
// real da capital (0,0) → mais longe = expedição mais demorada.
const MarchSecondsPerHex = 30

// HexDistance é a distância hexagonal (axial) entre (q1,r1) e (q2,r2).
func HexDistance(q1, r1, q2, r2 int) int {
	dq := q1 - q2
	dr := r1 - r2
	return (abs(q1-q2) + abs(dq+dr) + abs(r1-r2)) / 2
}

// MarchSecondsTo devolve o tempo de UM trecho de marcha até a região em (q,r), a partir da
// capital no centro (0,0).
func MarchSecondsTo(q, r int) int {
	d := HexDistance(0, 0, q, r)
	if d < 1 {
		d = 1
	}
	return d * MarchSecondsPerHex
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ProvinceTemplate é a definição estática de uma província PvE (mapa instanciado por jogador).
// A cidade fica no centro (0,0); o anel 1 são os 6 vizinhos hex. NameKey é traduzido no front.
// DefStack é um grupo de tropas defensoras (tipo + quantidade) de uma província.
type DefStack struct {
	Unit  string
	Count int
}

type ProvinceTemplate struct {
	NameKey string
	Ring    int              // faixa/era (1 = Era 1). A posição (q,r) NÃO é fixa — é gerada por seed.
	Defense []DefStack       // composição de tropas defensoras (≥1 tipo). Auto-resolve usa o agregado.
	Reward  resource.Amounts // recompensa ÚNICA ao conquistar
	Deposit resource.Amounts // renda PASSIVA por hora enquanto a província é mantida (GDD §8)
}

// DefenseAggregate soma a composição em (ataque, HP) totais — usado no auto-resolve e para o
// def_attack/def_hp persistido (exibição). A batalha tática instancia cada DefStack separadamente.
func (t ProvinceTemplate) DefenseAggregate() (attack, hp int) {
	for _, s := range t.Defense {
		if u, ok := UnitByKey(s.Unit); ok {
			attack += u.Attack * s.Count
			hp += u.HP * s.Count
		}
	}
	return attack, hp
}

// Era1Provinces: 6 províncias da Era 1 (ordem = dificuldade crescente), defesa por TROPAS reais
// (lanceiros + arqueiros). Vitória no auto-resolve ≈ aggHP×aggAtk ≤ 300×N² (N = lanceiros): a 1ª
// cai com pelotão pequeno (onboarding), a última exige quase o exército cheio (cap 25) ou
// arqueiros. Conquistar dá recompensa única + Deposit/hora. A POSIÇÃO no mapa é gerada por SEED
// do jogador (ver PlaceEra1Provinces) — espalhada e única por jogador, sem geometria fixa.
var Era1Provinces = []ProvinceTemplate{
	{NameKey: "clareira_dos_ecos", Ring: 1, Defense: []DefStack{{"lanceiro", 6}}, Reward: resource.Amounts{Matter: 120, Energy: 60}, Deposit: resource.Amounts{Matter: 6}},
	{NameKey: "pedreira_selvagem", Ring: 1, Defense: []DefStack{{"lanceiro", 9}, {"arqueiro", 2}}, Reward: resource.Amounts{Matter: 180, Energy: 70, Knowledge: 20}, Deposit: resource.Amounts{Matter: 8, Energy: 3}},
	{NameKey: "bosque_cinza", Ring: 1, Defense: []DefStack{{"lanceiro", 10}, {"arqueiro", 3}}, Reward: resource.Amounts{Matter: 150, Energy: 120, Knowledge: 30}, Deposit: resource.Amounts{Energy: 5, Knowledge: 4}},
	{NameKey: "ribeira_morta", Ring: 1, Defense: []DefStack{{"lanceiro", 13}, {"arqueiro", 4}}, Reward: resource.Amounts{Matter: 200, Energy: 100, Knowledge: 40}, Deposit: resource.Amounts{Matter: 8, Energy: 6, Knowledge: 3}},
	{NameKey: "colina_dos_vigias", Ring: 1, Defense: []DefStack{{"lanceiro", 15}, {"arqueiro", 5}}, Reward: resource.Amounts{Matter: 220, Energy: 140, Knowledge: 50}, Deposit: resource.Amounts{Energy: 10, Knowledge: 6}},
	{NameKey: "ruina_primeva", Ring: 1, Defense: []DefStack{{"lanceiro", 17}, {"arqueiro", 6}}, Reward: resource.Amounts{Matter: 260, Energy: 160, Knowledge: 70}, Deposit: resource.Amounts{Matter: 12, Energy: 10, Knowledge: 8}},
}

// ProvinceByKey busca o template de uma província pela NameKey (Era 1).
func ProvinceByKey(nameKey string) (ProvinceTemplate, bool) {
	for _, p := range Era1Provinces {
		if p.NameKey == nameKey {
			return p, true
		}
	}
	return ProvinceTemplate{}, false
}

// ProvCoord é uma posição axial hex (q,r) gerada para uma província.
type ProvCoord struct{ Q, R int }

// PlaceEra1Provinces gera, de forma DETERMINÍSTICA a partir de `seed` (hash do jogador), uma
// posição ESPALHADA para cada província da Era 1 — ângulo aleatório + distância crescente por
// dificuldade (a 1ª perto da capital, a última longe). Sem geometria fixa: cada jogador tem um
// layout único e orgânico. Evita repetir hex e a casa da capital (0,0).
func PlaceEra1Provinces(seed uint64) []ProvCoord {
	rng := rand.New(rand.NewSource(int64(seed))) //nolint:gosec // seed determinística, não-cripto
	taken := map[ProvCoord]bool{{Q: 0, R: 0}: true}
	out := make([]ProvCoord, len(Era1Provinces))
	for i := range Era1Provinces {
		minDist := 2.0 + float64(i) // 2,3,4,5,6,7 — dificuldade cresce → mais longe
		var c ProvCoord
		for attempt := 0; attempt < 60; attempt++ {
			dist := minDist + rng.Float64()*1.6
			ang := rng.Float64() * 2 * math.Pi
			c = pixelToAxial(dist*math.Cos(ang), dist*math.Sin(ang))
			if !taken[c] {
				break
			}
		}
		taken[c] = true
		out[i] = c
	}
	return out
}

// pixelToAxial converte uma posição em "unidades de hex" (HEX=1, pointy-top) para o hex axial
// mais próximo (arredondamento cúbico).
func pixelToAxial(px, py float64) ProvCoord {
	rf := py / 1.5
	qf := px/math.Sqrt(3) - rf/2
	return axialRound(qf, rf)
}

func axialRound(qf, rf float64) ProvCoord {
	xs, zs := qf, rf
	ys := -xs - zs
	rx, ry, rz := math.Round(xs), math.Round(ys), math.Round(zs)
	xd, yd, zd := math.Abs(rx-xs), math.Abs(ry-ys), math.Abs(rz-zs)
	if xd > yd && xd > zd {
		rx = -ry - rz
	} else if yd > zd {
		ry = -rx - rz
	} else {
		rz = -rx - ry
	}
	_ = ry
	return ProvCoord{Q: int(rx), R: int(rz)}
}
