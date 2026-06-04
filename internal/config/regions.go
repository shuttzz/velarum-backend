package config

import (
	"math"
	"math/rand"
)

// Mundo compartilhado dividido em REGIÕES nomeadas (4 quadrantes de um mundo quadrado). As contas
// novas PREENCHEM uma região por vez (na ordem) até atingir RegionCapacity; só então a próxima
// região começa a ser povoada → novatos caem perto de gente (vizinhos imediatos), sem ficar
// sozinhos num quadrante vazio. O MIOLO do mapa fica reservado como zona especial/contestada
// futura (não é região de spawn). Quando TODAS enchem → abre-se um novo world (sharding, futuro).
// Coords são do mundo (tratadas como hex axial no render).

const (
	regionSpawnRadius = 14 // raio (hexes) em que as cidades se espalham dentro da região (tunável)
	citySpacing       = 3  // distância mínima (hex) entre cidades
	RegionCapacity    = 25 // teto de cidades por região antes de começar a povoar a próxima (tunável)
)

// Region é um quadrante nomeado do mundo.
type Region struct {
	Key     string
	CenterX int
	CenterY int
}

// WorldRegions: 4 quadrantes, centros nos cantos de um mundo quadrado (~70 de meia-largura).
// A ORDEM define a sequência de preenchimento.
var WorldRegions = []Region{
	{Key: "campos_da_aurora", CenterX: 70, CenterY: 70},
	{Key: "ermo_cinereo", CenterX: -70, CenterY: 70},
	{Key: "litoral_partido", CenterX: -70, CenterY: -70},
	{Key: "planalto_runico", CenterX: 70, CenterY: -70},
}

// PlaceNewCity escolhe a região de spawn (preenchimento sequencial até RegionCapacity) e um ponto
// livre dentro dela (espalhado, com espaçamento mínimo). `counts` = nº de cidades por região
// (key→n); `taken` = coords ocupadas.
func PlaceNewCity(rng *rand.Rand, counts map[string]int, taken map[[2]int]bool) (regionKey string, x, y int) {
	region := pickRegion(counts)
	for attempt := 0; attempt < 300; attempt++ {
		ang := rng.Float64() * 2 * math.Pi
		dist := rng.Float64() * float64(regionSpawnRadius)
		cx := region.CenterX + int(dist*math.Cos(ang))
		cy := region.CenterY + int(dist*math.Sin(ang))
		if taken[[2]int{cx, cy}] || tooCloseToAny(cx, cy, taken) {
			continue
		}
		return region.Key, cx, cy
	}
	// Fallback: anel crescente a partir do centro até achar uma casa livre.
	for d := 1; ; d++ {
		for dx := -d; dx <= d; dx++ {
			for dy := -d; dy <= d; dy++ {
				cx, cy := region.CenterX+dx, region.CenterY+dy
				if !taken[[2]int{cx, cy}] {
					return region.Key, cx, cy
				}
			}
		}
	}
}

// pickRegion: a primeira região (na ordem) que ainda não atingiu o teto; se todas cheias, a menos
// populada (transbordo — até o sharding por world existir).
func pickRegion(counts map[string]int) Region {
	for _, r := range WorldRegions {
		if counts[r.Key] < RegionCapacity {
			return r
		}
	}
	least := WorldRegions[0]
	for _, r := range WorldRegions {
		if counts[r.Key] < counts[least.Key] {
			least = r
		}
	}
	return least
}

func tooCloseToAny(x, y int, taken map[[2]int]bool) bool {
	for c := range taken {
		if HexDistance(x, y, c[0], c[1]) < citySpacing {
			return true
		}
	}
	return false
}
