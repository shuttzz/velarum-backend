// Package config contém as constantes de jogo (game design tuning) do Velarum.
// Os valores da Era 1 vêm da especificação de sistemas "Primeiros Fogos".
//
// Princípio firme: espaço/edifícios NUNCA são paywall. Slots abrem ao avançar de era.
package config

import (
	"math"
	"time"

	"backend/internal/domain/resource"
)

// Taxas de crescimento por nível (spec Era 1).
const (
	ProductionGrowth = 1.55 // produção/h
	CostGrowth       = 1.65 // custo de upgrade
	BuildTimeGrowth  = 1.80 // tempo de construção
)

// DefaultWorldID é o UUID FIXO do mundo padrão (compartilhado), seedado na migration.
// Por ora há um único mundo; mais mundos/temporadas virão com matchmaking dedicado.
const DefaultWorldID = "00000000-0000-7000-8000-000000000001"

// Estado inicial de uma cidade nova. StartingResources é generoso o bastante para construir
// os ~5 edifícios básicos no turno 1 (onboarding do gênero). StartingStorage é a PARCELA
// PROTEGIDA inicial contra saque (não é teto — recursos sobem sem limite; cf. resource.State).
var (
	StartingResources = resource.Amounts{Matter: 500, Energy: 500, Knowledge: 200}
	StartingStorage   = resource.Amounts{Matter: 500, Energy: 500, Knowledge: 200}
)

// SlotsByEra: slots de construção por era (índice 0 = Era 1).
// Abrem ao AVANÇAR DE ERA (de graça) — nunca por pagamento.
var SlotsByEra = []int{12, 16, 20, 24, 28, 32, 36}

// Teto de exército (cresce com o nível do Canteiro de Almas).
const (
	ArmyCapBase          = 20
	ArmyCapPerBarracksLv = 5
)

// ProductionPerHour calcula a produção/hora de um edifício de recurso por nível.
func ProductionPerHour(base float64, level int) float64 {
	return math.Floor(base * math.Pow(ProductionGrowth, float64(level-1)))
}

// CostFor calcula o custo de um upgrade a um dado nível, a partir do custo base (nível 1).
func CostFor(base resource.Amounts, level int) resource.Amounts {
	f := math.Pow(CostGrowth, float64(level-1))
	return resource.Amounts{
		Matter:    math.Round(base.Matter * f),
		Energy:    math.Round(base.Energy * f),
		Knowledge: math.Round(base.Knowledge * f),
	}
}

// BuildTimeFor calcula o tempo de construção/upgrade a um dado nível.
func BuildTimeFor(baseSeconds float64, level int) time.Duration {
	secs := baseSeconds * math.Pow(BuildTimeGrowth, float64(level-1))
	return time.Duration(secs * float64(time.Second))
}

// StorageCapFor calcula a capacidade de armazém (por recurso) de um Celeiro de Argila por nível.
func StorageCapFor(level int) float64 {
	n := float64(level - 1)
	return 500 + n*300 + math.Floor(n*n*50)
}

// Requirement é uma dependência de construção: BuildingKey precisa estar em nível >= Level.
type Requirement struct {
	BuildingKey string
	Level       int
}

// BuildingDef é a definição estática de um tipo de edifício.
type BuildingDef struct {
	Key       string
	Name      string
	Category  string           // production|storage|central|military|culture|defense|research|social|military_upgrade|marco
	Produces  string           // "matter" | "energy" | "knowledge" | "" (não produz)
	BaseRate  float64          // produção/h no nível 1 (0 se não produz)
	BaseCost  resource.Amounts // custo do nível 1
	BaseTime  float64          // tempo de construção do nível 1, em segundos
	MaxCopies int              // quantas instâncias podem existir na cidade
	Requires  []Requirement    // dependências para poder construir
	Era       int
	Width     int // footprint em células (0 = 1)
	Height    int // footprint em células (0 = 1)
}

// Era1Buildings: catálogo de edifícios da Era 1 "Primeiros Fogos".
// Onboarding: os 5 básicos (3 produtores + armazém + quartel) NÃO têm pré-requisito — dá pra
// construir vários no turno 1. As estruturas avançadas mantêm a árvore de progressão.
// Tempos baixos no início (1º build ~5s), seguindo o padrão do gênero.
var Era1Buildings = []BuildingDef{
	{Key: "lar_do_cla", Name: "Lar do Clã", Category: "central", BaseCost: resource.Amounts{Matter: 120, Energy: 60, Knowledge: 30}, BaseTime: 30, MaxCopies: 1, Era: 1},
	{Key: "viveiro_de_pedra", Name: "Viveiro de Pedra", Category: "production", Produces: "matter", BaseRate: 8, BaseCost: resource.Amounts{Matter: 50, Energy: 20}, BaseTime: 5, MaxCopies: 3, Era: 1},
	{Key: "fogueira_comunal", Name: "Fogueira Comunal", Category: "production", Produces: "energy", BaseRate: 6, BaseCost: resource.Amounts{Matter: 50, Energy: 20}, BaseTime: 5, MaxCopies: 3, Era: 1},
	{Key: "pedra_da_memoria", Name: "Pedra da Memória", Category: "production", Produces: "knowledge", BaseRate: 3, BaseCost: resource.Amounts{Matter: 60, Energy: 40}, BaseTime: 8, MaxCopies: 2, Era: 1},
	{Key: "celeiro_de_argila", Name: "Celeiro de Argila", Category: "storage", BaseCost: resource.Amounts{Matter: 80, Energy: 40}, BaseTime: 8, MaxCopies: 2, Era: 1},
	{Key: "canteiro_de_almas", Name: "Canteiro de Almas", Category: "military", BaseCost: resource.Amounts{Matter: 120, Energy: 80, Knowledge: 20}, BaseTime: 15, MaxCopies: 1, Era: 1},
	{Key: "altar_das_fogueiras", Name: "Altar das Fogueiras", Category: "culture", BaseCost: resource.Amounts{Matter: 70, Energy: 80, Knowledge: 30}, BaseTime: 60, MaxCopies: 1, Era: 1, Requires: []Requirement{{"fogueira_comunal", 2}}},
	{Key: "torre_do_vigia", Name: "Torre do Vigia", Category: "defense", BaseCost: resource.Amounts{Matter: 90, Energy: 30, Knowledge: 10}, BaseTime: 45, MaxCopies: 1, Era: 1, Requires: []Requirement{{"canteiro_de_almas", 1}}},
	{Key: "circulo_runico", Name: "Círculo Rúnico", Category: "research", BaseCost: resource.Amounts{Matter: 60, Energy: 40, Knowledge: 40}, BaseTime: 60, MaxCopies: 1, Era: 1, Requires: []Requirement{{"pedra_da_memoria", 2}}},
	{Key: "praca_do_conselho", Name: "Praça do Conselho", Category: "social", BaseCost: resource.Amounts{Matter: 150, Energy: 100, Knowledge: 50}, BaseTime: 120, MaxCopies: 1, Era: 1, Requires: []Requirement{{"lar_do_cla", 4}}},
	{Key: "pira_dos_guerreiros", Name: "Pira dos Guerreiros", Category: "military_upgrade", BaseCost: resource.Amounts{Matter: 80, Energy: 60, Knowledge: 40}, BaseTime: 90, MaxCopies: 1, Era: 1, Requires: []Requirement{{"canteiro_de_almas", 2}}},
	{Key: "marco_primeiros_fogos", Name: "Marco dos Primeiros Fogos", Category: "marco", BaseCost: resource.Amounts{Matter: 500, Energy: 300, Knowledge: 200}, BaseTime: 600, MaxCopies: 1, Era: 1, Requires: []Requirement{{"lar_do_cla", 5}, {"pedra_da_memoria", 3}}},
}

// EraAdvanceReq descreve os requisitos para avançar de era.
type EraAdvanceReq struct {
	MarcoBuilt        bool
	CentralMinLevel   int     // nível mínimo do Lar do Clã
	ResearchPct       int     // % da árvore de pesquisa da era
	KnowledgeSpentMin float64 // Conhecimento gasto historicamente
	MinHoursInEra     int     // tempo mínimo desde criação da conta (anti-rush de novato)
}

// Era1Advance: requisitos para sair da Era 1 → Era 2.
var Era1Advance = EraAdvanceReq{
	MarcoBuilt:        true,
	CentralMinLevel:   5,
	ResearchPct:       100,
	KnowledgeSpentMin: 800,
	MinHoursInEra:     48,
}
