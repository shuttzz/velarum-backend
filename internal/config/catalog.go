package config

import "backend/internal/domain/resource"

// CatalogRequirement é a forma serializável (JSON) de Requirement.
type CatalogRequirement struct {
	BuildingKey string `json:"building_key"`
	Level       int    `json:"level"`
}

// CatalogBuilding é a forma serializável (JSON) de um BuildingDef, exposta ao cliente
// para que ele monte a paleta de construção (nome, custo/tempo base, pré-requisitos).
type CatalogBuilding struct {
	Key       string               `json:"key"`
	Name      string               `json:"name"`
	Category  string               `json:"category"`
	Produces  string               `json:"produces"`
	BaseRate  float64              `json:"base_rate"`
	BaseCost  resource.Amounts     `json:"base_cost"`
	BaseTime  float64              `json:"base_time"`
	MaxCopies int                  `json:"max_copies"`
	Era       int                  `json:"era"`
	Width     int                  `json:"w"`
	Height    int                  `json:"h"`
	Requires  []CatalogRequirement `json:"requires"`
}

// CatalogGrowth expõe as taxas de crescimento por nível ao cliente, para que ele
// calcule custo/tempo de upgrade (nível N) sem duplicar os valores base no frontend.
type CatalogGrowth struct {
	Production float64 `json:"production"`
	Cost       float64 `json:"cost"`
	BuildTime  float64 `json:"build_time"`
}

// CatalogPayload é o corpo do endpoint GET /catalog: todos os edifícios disponíveis
// (hoje, Era 1) + as constantes de crescimento.
type CatalogPayload struct {
	Growth    CatalogGrowth     `json:"growth"`
	Buildings []CatalogBuilding `json:"buildings"`
}

// Catalog monta o catálogo serializável a partir das definições estáticas (Era 1).
func Catalog() CatalogPayload {
	buildings := make([]CatalogBuilding, 0, len(Era1Buildings))
	for _, d := range Era1Buildings {
		w, h := d.Footprint()
		reqs := make([]CatalogRequirement, 0, len(d.Requires))
		for _, r := range d.Requires {
			reqs = append(reqs, CatalogRequirement{BuildingKey: r.BuildingKey, Level: r.Level})
		}
		buildings = append(buildings, CatalogBuilding{
			Key:       d.Key,
			Name:      d.Name,
			Category:  d.Category,
			Produces:  d.Produces,
			BaseRate:  d.BaseRate,
			BaseCost:  d.BaseCost,
			BaseTime:  d.BaseTime,
			MaxCopies: d.MaxCopies,
			Era:       d.Era,
			Width:     w,
			Height:    h,
			Requires:  reqs,
		})
	}
	return CatalogPayload{
		Growth:    CatalogGrowth{Production: ProductionGrowth, Cost: CostGrowth, BuildTime: BuildTimeGrowth},
		Buildings: buildings,
	}
}
