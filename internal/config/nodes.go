package config

import (
	"math"
	"math/rand"
)

// Nós de recurso do MUNDO COMPARTILHADO (SW2). Modelo RoK: tiles farmáveis sem dono; qualquer um
// marcha até lá, coleta ao longo do tempo (∝ carga÷taxa, taxa POR TROPA), volta com loot; depleção
// PARCIAL; respawnam ao zerar. 1 ocupante por vez. Valores dev-scale (tunáveis).
const (
	NodesPerRegion  = 6  // nós vivos mantidos em cada região
	nodeSpawnRadius = 10 // raio (hex) de espalhamento dos nós em torno do centro da região
)

// nodeResources: recursos que um nó pode render (sorteado no spawn).
var nodeResources = []string{"matter", "energy", "knowledge"}

// nodeResourceMult escala a quantidade do nó por recurso, ESPELHANDO as taxas de produção da
// cidade (Viveiro 20 / Fogueira 15 / Pedra da Memória 8). Conhecimento é mais escasso: gera menos
// na cidade, então seus nós também rendem menos (estilo RoK, onde depósitos de ouro são menores).
var nodeResourceMult = map[string]float64{"matter": 1.0, "energy": 0.75, "knowledge": 0.4}

// nodeBaseAmount é a quantidade base de um nó por nível (antes do multiplicador de recurso).
// Dev-scale (em produção pode chegar a dezenas/centenas de milhares — GDD/§memória).
func nodeBaseAmount(level int) float64 {
	switch level {
	case 1:
		return 300
	case 2:
		return 700
	default: // 3+
		return 1500
	}
}

// NodeAmountFor é a quantidade TOTAL de recurso de um nó (cresce com o nível, escala pelo recurso).
func NodeAmountFor(resource string, level int) float64 {
	m := nodeResourceMult[resource]
	if m == 0 {
		m = 1
	}
	return math.Floor(nodeBaseAmount(level) * m)
}

// MarchSecondsBetween é o tempo de UM trecho de marcha entre dois tiles do mundo (coords reais).
// Diferente de MarchSecondsTo (que assume a capital em 0,0) — aqui a cidade está num tile do mundo.
func MarchSecondsBetween(x1, y1, x2, y2 int) int {
	d := HexDistance(x1, y1, x2, y2)
	if d < 1 {
		d = 1
	}
	return d * MarchSecondsPerHex
}

// CollectPlan calcula quanto um exército coleta de um nó e quanto tempo leva. Modelo RoK por-tropa:
//   - coletado = min(carga_total, disponível); carga_total = Σ count×Carry
//   - tempo    = coletado / taxa_total;        taxa_total  = Σ count×GatherRate
//
// Consequência: encher a carga cheia leva ~constante; mandar exército grande num nó PEQUENO o
// drena mais rápido. (⚠ não-intuitivo — explicar ao jogador no futuro.)
func CollectPlan(troops map[string]int, available float64) (collected, seconds float64) {
	var totalCarry, totalRate float64
	for ut, c := range troops {
		if c <= 0 {
			continue
		}
		if u, ok := UnitByKey(ut); ok {
			totalCarry += float64(u.Carry) * float64(c)
			totalRate += u.GatherRate * float64(c)
		}
	}
	collected = totalCarry
	if available < collected {
		collected = available
	}
	if collected <= 0 || totalRate <= 0 {
		return 0, 0
	}
	return collected, collected / totalRate
}

// NodeSpawn descreve um nó a ser criado (posição + nível + recurso).
type NodeSpawn struct {
	X, Y, Level int
	Resource    string
}

// PlaceWorldNodes gera `NodesPerRegion` nós ESPALHADOS em torno do centro de CADA região, evitando
// coords já ocupadas (cidades/outros nós). Nível e recurso são sorteados pelo rng.
func PlaceWorldNodes(rng *rand.Rand, taken map[[2]int]bool) []NodeSpawn {
	out := make([]NodeSpawn, 0, NodesPerRegion*len(WorldRegions))
	for _, region := range WorldRegions {
		for i := 0; i < NodesPerRegion; i++ {
			if sp, ok := placeOneNode(rng, region.CenterX, region.CenterY, taken); ok {
				out = append(out, sp)
			}
		}
	}
	return out
}

// PlaceRespawnNode escolhe uma região ALEATÓRIA e um tile livre nela para o respawn de um nó.
func PlaceRespawnNode(rng *rand.Rand, taken map[[2]int]bool) (NodeSpawn, bool) {
	region := WorldRegions[rng.Intn(len(WorldRegions))]
	return placeOneNode(rng, region.CenterX, region.CenterY, taken)
}

func placeOneNode(rng *rand.Rand, cx, cy int, taken map[[2]int]bool) (NodeSpawn, bool) {
	for attempt := 0; attempt < 200; attempt++ {
		ang := rng.Float64() * 2 * math.Pi
		dist := rng.Float64() * float64(nodeSpawnRadius)
		x := cx + int(dist*math.Cos(ang))
		y := cy + int(dist*math.Sin(ang))
		if taken[[2]int{x, y}] {
			continue
		}
		taken[[2]int{x, y}] = true
		return NodeSpawn{X: x, Y: y, Level: 1 + rng.Intn(3), Resource: nodeResources[rng.Intn(len(nodeResources))]}, true
	}
	return NodeSpawn{}, false
}
