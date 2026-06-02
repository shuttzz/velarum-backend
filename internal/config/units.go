package config

import "backend/internal/domain/resource"

// UnitDef é a definição estática de um tipo de unidade militar.
// Era 1 tem duas categorias (infantaria e projétil); mais categorias/facções vêm depois.
type UnitDef struct {
	Key         string
	Name        string
	Category    string           // infantry | projectile
	Attack      int              // poder de ataque por unidade
	Defense     int              // poder de defesa por unidade
	HP          int              // vida por unidade (usado na batalha tática)
	Cost        resource.Amounts // custo por unidade
	RecruitTime float64          // segundos por unidade
	Era         int
}

// Era1Units: catálogo de unidades da Era 1.
var Era1Units = []UnitDef{
	{Key: "lanceiro", Name: "Lanceiro", Category: "infantry", Attack: 10, Defense: 8, HP: 30, Cost: resource.Amounts{Matter: 20, Energy: 10}, RecruitTime: 20, Era: 1},
	{Key: "arqueiro", Name: "Arqueiro", Category: "projectile", Attack: 14, Defense: 4, HP: 20, Cost: resource.Amounts{Matter: 15, Energy: 15}, RecruitTime: 25, Era: 1},
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
