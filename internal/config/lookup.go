package config

import "backend/internal/domain/resource"

// BuildingByKey busca a definição de um edifício pela key (por ora, só Era 1).
func BuildingByKey(key string) (BuildingDef, bool) {
	for _, b := range Era1Buildings {
		if b.Key == key {
			return b, true
		}
	}
	return BuildingDef{}, false
}

// SlotsForEra retorna quantos slots de construção a cidade tem numa dada era.
func SlotsForEra(era int) int {
	if era >= 1 && era <= len(SlotsByEra) {
		return SlotsByEra[era-1]
	}
	return 0
}

// ProductionAt retorna a contribuição de produção/hora de um edifício num dado nível.
// Edifícios que não produzem recurso retornam zero.
func (d BuildingDef) ProductionAt(level int) resource.Amounts {
	if d.BaseRate == 0 || d.Produces == "" {
		return resource.Amounts{}
	}
	v := ProductionPerHour(d.BaseRate, level)
	switch d.Produces {
	case "matter":
		return resource.Amounts{Matter: v}
	case "energy":
		return resource.Amounts{Energy: v}
	case "knowledge":
		return resource.Amounts{Knowledge: v}
	}
	return resource.Amounts{}
}
