package config

import "backend/internal/domain/resource"

// UnitDef é a definição estática de um tipo de unidade militar.
// Era 1 tem duas categorias (infantaria e projétil); mais categorias/facções vêm depois.
type UnitDef struct {
	Key             string
	Name            string
	Category        string           // infantry | projectile
	Attack          int              // poder de ataque por unidade
	Defense         int              // poder de defesa por unidade
	HP              int              // vida por unidade (usado na batalha tática)
	Move            int              // alcance de movimento na batalha tática (hexes/turno)
	Range           int              // alcance de ataque na batalha tática (hexes)
	Cost            resource.Amounts // custo por unidade
	RecruitTime     float64          // segundos por unidade
	MinBarracksLevel int             // nível mínimo do Canteiro de Almas para desbloquear
	Carry           int              // capacidade de CARGA (recurso que a unidade carrega de um nó)
	Era             int
}

// Era1Units: catálogo de unidades da Era 1. O Lanceiro (mais fraco/barato) já vem liberado
// com o Canteiro nível 1; o Arqueiro desbloqueia no Canteiro nível 2 (progressão).
// Lanceiro: infantaria corpo-a-corpo (alcance 1). Arqueiro: projétil (alcance 2).
// Carga: quanto cada unidade carrega de um nó. O TEMPO de coleta é governado por
// config.CollectFillSeconds (encher a carga cheia leva esse tempo; drenar nó pequeno é proporcional).
var Era1Units = []UnitDef{
	{Key: "lanceiro", Name: "Lanceiro", Category: "infantry", Attack: 10, Defense: 8, HP: 30, Move: 1, Range: 1, Cost: resource.Amounts{Matter: 20, Energy: 10}, RecruitTime: 10, MinBarracksLevel: 1, Carry: 25, Era: 1},
	{Key: "arqueiro", Name: "Arqueiro", Category: "projectile", Attack: 14, Defense: 4, HP: 20, Move: 1, Range: 2, Cost: resource.Amounts{Matter: 15, Energy: 15}, RecruitTime: 15, MinBarracksLevel: 2, Carry: 20, Era: 1},
}

// UnitByKey busca a definição de uma unidade pela key (por ora, só Era 1).
func UnitByKey(key string) (UnitDef, bool) {
	for _, u := range Era1Units {
		if u.Key == key {
			return u, true
		}
	}
	return UnitDef{}, false
}

// ArmyCap retorna o teto de exército (nº de unidades) dado o nível do Canteiro de Almas.
// Sem o Canteiro (nível 0) não há capacidade — é preciso construí-lo para recrutar.
func ArmyCap(barracksLevel int) int {
	if barracksLevel <= 0 {
		return 0
	}
	return ArmyCapBase + barracksLevel*ArmyCapPerBarracksLv
}

// BarracksKey é a key do edifício que habilita/eleva o recrutamento (Canteiro de Almas).
const BarracksKey = "canteiro_de_almas"
