package config

import "backend/internal/domain/resource"

// ProvinceMarchSecondsPerRing é o tempo de marcha (em segundos) por anel de distância,
// para CADA trecho (ida e volta são separados). Anel 1 = 60s de ida + 60s de volta.
const ProvinceMarchSecondsPerRing = 60

// ProvinceTemplate é a definição estática de uma província PvE (mapa instanciado por jogador).
// A cidade fica no centro (0,0); o anel 1 são os 6 vizinhos hex. NameKey é traduzido no front.
type ProvinceTemplate struct {
	NameKey   string
	Q, R      int // coordenada axial hex (cidade = 0,0)
	Ring      int
	DefAttack int              // dano de defesa por rodada (auto-resolve)
	DefHP     int              // vida total da defesa
	Reward    resource.Amounts // recompensa ao conquistar
}

// Era1Provinces: 6 províncias do anel 1 (os 6 vizinhos hex da cidade), dificuldade e
// recompensa crescentes. Calibráveis. Um exército modesto da Era 1 vence as primeiras.
var Era1Provinces = []ProvinceTemplate{
	{NameKey: "clareira_dos_ecos", Q: 1, R: 0, Ring: 1, DefAttack: 10, DefHP: 80, Reward: resource.Amounts{Matter: 120, Energy: 60}},
	{NameKey: "pedreira_selvagem", Q: 1, R: -1, Ring: 1, DefAttack: 12, DefHP: 110, Reward: resource.Amounts{Matter: 180, Energy: 70, Knowledge: 20}},
	{NameKey: "bosque_cinza", Q: 0, R: -1, Ring: 1, DefAttack: 14, DefHP: 140, Reward: resource.Amounts{Matter: 150, Energy: 120, Knowledge: 30}},
	{NameKey: "ribeira_morta", Q: -1, R: 0, Ring: 1, DefAttack: 16, DefHP: 170, Reward: resource.Amounts{Matter: 200, Energy: 100, Knowledge: 40}},
	{NameKey: "colina_dos_vigias", Q: -1, R: 1, Ring: 1, DefAttack: 18, DefHP: 200, Reward: resource.Amounts{Matter: 220, Energy: 140, Knowledge: 50}},
	{NameKey: "ruina_primeva", Q: 0, R: 1, Ring: 1, DefAttack: 20, DefHP: 240, Reward: resource.Amounts{Matter: 260, Energy: 160, Knowledge: 70}},
}
