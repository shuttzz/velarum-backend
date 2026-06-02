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

// Footprint retorna o tamanho do edifício em células (mínimo 1×1).
func (d BuildingDef) Footprint() (w, h int) {
	w, h = d.Width, d.Height
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// gridByEra: tamanho do grid da cidade (largura, altura) por era. A grade cresce
// com a era — é a forma espacial de "abrir mais espaço" sem ser paywall.
var gridByEra = [][2]int{{8, 6}, {10, 8}, {12, 10}, {14, 12}, {16, 14}, {18, 16}, {20, 18}}

// GridForEra retorna as dimensões do grid da cidade numa dada era.
func GridForEra(era int) (width, height int) {
	if era >= 1 && era <= len(gridByEra) {
		return gridByEra[era-1][0], gridByEra[era-1][1]
	}
	return gridByEra[0][0], gridByEra[0][1]
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
