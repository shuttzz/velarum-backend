package config

import "backend/internal/domain/resource"

// ProvinceMarchSecondsPerRing é o tempo de marcha (em segundos) por anel de distância,
// para CADA trecho (ida e volta são separados). Anel 1 = 60s de ida + 60s de volta.
const ProvinceMarchSecondsPerRing = 60

// ProvinceTemplate é a definição estática de uma província PvE (mapa instanciado por jogador).
// A cidade fica no centro (0,0); o anel 1 são os 6 vizinhos hex. NameKey é traduzido no front.
// DefStack é um grupo de tropas defensoras (tipo + quantidade) de uma província.
type DefStack struct {
	Unit  string
	Count int
}

type ProvinceTemplate struct {
	NameKey string
	Q, R    int // coordenada axial hex (cidade = 0,0)
	Ring    int
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

// Era1Provinces: 6 províncias do anel 1 (os 6 vizinhos hex), defesa por TROPAS reais (lanceiros +
// arqueiros), dificuldade/recompensa crescentes. Vitória no auto-resolve ≈ aggHP×aggAtk ≤ 300×N²
// (N = lanceiros): a 1ª cai com pelotão pequeno (onboarding), a última exige quase o exército
// cheio (cap 25 no Canteiro nv1) ou arqueiros. Conquistar dá recompensa única + Deposit/hora.
var Era1Provinces = []ProvinceTemplate{
	{NameKey: "clareira_dos_ecos", Q: 1, R: 0, Ring: 1, Defense: []DefStack{{"lanceiro", 6}}, Reward: resource.Amounts{Matter: 120, Energy: 60}, Deposit: resource.Amounts{Matter: 6}},
	{NameKey: "pedreira_selvagem", Q: 1, R: -1, Ring: 1, Defense: []DefStack{{"lanceiro", 9}, {"arqueiro", 2}}, Reward: resource.Amounts{Matter: 180, Energy: 70, Knowledge: 20}, Deposit: resource.Amounts{Matter: 8, Energy: 3}},
	{NameKey: "bosque_cinza", Q: 0, R: -1, Ring: 1, Defense: []DefStack{{"lanceiro", 10}, {"arqueiro", 3}}, Reward: resource.Amounts{Matter: 150, Energy: 120, Knowledge: 30}, Deposit: resource.Amounts{Energy: 5, Knowledge: 4}},
	{NameKey: "ribeira_morta", Q: -1, R: 0, Ring: 1, Defense: []DefStack{{"lanceiro", 13}, {"arqueiro", 4}}, Reward: resource.Amounts{Matter: 200, Energy: 100, Knowledge: 40}, Deposit: resource.Amounts{Matter: 8, Energy: 6, Knowledge: 3}},
	{NameKey: "colina_dos_vigias", Q: -1, R: 1, Ring: 1, Defense: []DefStack{{"lanceiro", 15}, {"arqueiro", 5}}, Reward: resource.Amounts{Matter: 220, Energy: 140, Knowledge: 50}, Deposit: resource.Amounts{Energy: 10, Knowledge: 6}},
	{NameKey: "ruina_primeva", Q: 0, R: 1, Ring: 1, Defense: []DefStack{{"lanceiro", 17}, {"arqueiro", 6}}, Reward: resource.Amounts{Matter: 260, Energy: 160, Knowledge: 70}, Deposit: resource.Amounts{Matter: 12, Energy: 10, Knowledge: 8}},
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
