package config

import (
	"math"
	"math/rand"
	"time"

	"backend/internal/domain/resource"
)

// Nós de recurso do MUNDO COMPARTILHADO (SW2). Modelo RoK: tiles farmáveis sem dono; qualquer um
// marcha até lá, coleta ao longo do tempo (∝ carga÷taxa, taxa POR TROPA), volta com loot; depleção
// PARCIAL; respawnam ao zerar. 1 ocupante por vez. Valores dev-scale (tunáveis).
const (
	NodesPerRegion     = 6  // nós de recurso vivos mantidos por região
	VillagesPerRegion  = 3  // aldeias hostis (combate one-shot) por região
	CreaturesPerRegion = 3  // criaturas da Lacuna (combate one-shot) por região
	targetSpawnRadius  = 18 // raio (hex) MÁX de espalhamento dos alvos a partir do centro da região
	// WorldHalf é a meia-largura do mundo (coords −WorldHalf..WorldHalf). Espelha WORLD_HALF do
	// frontend. Alvos não nascem fora dessas bordas.
	WorldHalf = 50
)

// CombatTargetFor devolve a defesa agregada (ataque, HP) e o loot de um alvo de COMBATE
// (village|creature) no nível dado. Aldeia = equilibrada; criatura = mais "tanque" (HP), loot só
// de matéria. Dev-scale: o nível 1 cai com um pelotão inicial; sobe ~1.6×def / 1.5×loot por nível.
func CombatTargetFor(kind string, level int) (defAttack, defHp int, reward resource.Amounts) {
	df := math.Pow(1.6, float64(level-1))
	rf := math.Pow(1.5, float64(level-1))
	if kind == "creature" {
		return int(math.Round(30 * df)), int(math.Round(180 * df)), resource.Amounts{Matter: math.Round(120 * rf)}
	}
	// village (padrão)
	return int(math.Round(40 * df)), int(math.Round(120 * df)), resource.Amounts{Matter: math.Round(80 * rf), Energy: math.Round(40 * rf)}
}

// RandomNodeResource sorteia o recurso de um nó.
func RandomNodeResource(rng *rand.Rand) string {
	return nodeResources[rng.Intn(len(nodeResources))]
}

// RandomTargetLevel sorteia um nível 1..3 para um alvo.
func RandomTargetLevel(rng *rand.Rand) int {
	return 1 + rng.Intn(3)
}

// TTL dos alvos de COMBATE (aldeias/criaturas): se ninguém atacar nesse tempo, despawnam (e a
// população é reposta em outro lugar) — mantém o mapa "vivo". Faixa tunável (dev mais curto).
const (
	combatTTLMinSeconds = 600  // 10 min
	combatTTLMaxSeconds = 1800 // 30 min
)

// RandomCombatTTL sorteia a duração de vida de um alvo de combate (entre min e max).
func RandomCombatTTL(rng *rand.Rand) time.Duration {
	span := combatTTLMaxSeconds - combatTTLMinSeconds
	return time.Duration(combatTTLMinSeconds+rng.Intn(span+1)) * time.Second
}

// PlaceOneNearAnyRegion acha um tile livre em torno de uma região ALEATÓRIA (spawn/top-up de
// qualquer alvo). Marca o tile como ocupado em `taken`.
func PlaceOneNearAnyRegion(rng *rand.Rand, taken map[[2]int]bool) (x, y int, ok bool) {
	region := WorldRegions[rng.Intn(len(WorldRegions))]
	return placeOneCoord(rng, region.CenterX, region.CenterY, taken)
}

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
		return 600
	case 2:
		return 1500
	default: // 3+
		return 3000
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

// CollectFillSeconds é o tempo (segundos) para um exército ENCHER sua CARGA TOTAL num nó. É o KNOB
// central do ritmo da coleta: lento o bastante para PRENDER as tropas e a fila de marcha (custo de
// oportunidade real), mantendo os recursos abundantes no mapa. Tunável.
const CollectFillSeconds = 300 // 5 min para encher a carga cheia

// CollectPlan calcula quanto um exército coleta de um nó e quanto tempo leva. Modelo RoK por-tropa:
//   - coletado = min(carga_total, disponível); carga_total = Σ count×Carry
//   - tempo    = coletado / carga_total × CollectFillSeconds
//
// Consequência: encher a carga cheia leva CollectFillSeconds (constante, independe do tamanho do
// exército); mandar exército grande num nó PEQUENO o drena PROPORCIONALMENTE mais rápido.
// (⚠ não-intuitivo — explicar ao jogador no futuro.)
func CollectPlan(troops map[string]int, available float64) (collected, seconds float64) {
	var totalCarry float64
	for ut, c := range troops {
		if c <= 0 {
			continue
		}
		if u, ok := UnitByKey(ut); ok {
			totalCarry += float64(u.Carry) * float64(c)
		}
	}
	collected = totalCarry
	if available < collected {
		collected = available
	}
	if collected <= 0 || totalCarry <= 0 {
		return 0, 0
	}
	return collected, collected / totalCarry * CollectFillSeconds
}

// NodeSpawn descreve um nó a ser criado (posição + nível + recurso).
type NodeSpawn struct {
	X, Y, Level int
	Resource    string
}

// PlaceRespawnNode escolhe uma região ALEATÓRIA e um tile livre nela para o respawn de um nó.
func PlaceRespawnNode(rng *rand.Rand, taken map[[2]int]bool) (NodeSpawn, bool) {
	region := WorldRegions[rng.Intn(len(WorldRegions))]
	return placeOneNode(rng, region.CenterX, region.CenterY, taken)
}

func placeOneNode(rng *rand.Rand, cx, cy int, taken map[[2]int]bool) (NodeSpawn, bool) {
	x, y, ok := placeOneCoord(rng, cx, cy, taken)
	if !ok {
		return NodeSpawn{}, false
	}
	return NodeSpawn{X: x, Y: y, Level: RandomTargetLevel(rng), Resource: RandomNodeResource(rng)}, true
}

// placeOneCoord acha um tile livre (não em `taken`, dentro do mundo) ESPALHADO em torno de (cx,cy).
// Distância CONTÍNUA e aleatória (sem padrão visível tipo "10/20/30") com `√rng` → distribuição
// uniforme por ÁREA, ou seja, menos amontoado perto do centro e mais alvos espalhados/longe.
func placeOneCoord(rng *rand.Rand, cx, cy int, taken map[[2]int]bool) (x, y int, ok bool) {
	for attempt := 0; attempt < 300; attempt++ {
		ang := rng.Float64() * 2 * math.Pi
		dist := float64(targetSpawnRadius) * math.Sqrt(rng.Float64())
		px := cx + int(dist*math.Cos(ang))
		py := cy + int(dist*math.Sin(ang))
		if px < -WorldHalf+1 || px > WorldHalf-1 || py < -WorldHalf+1 || py > WorldHalf-1 {
			continue // fora das bordas do mundo → tenta outra posição
		}
		if taken[[2]int{px, py}] {
			continue
		}
		taken[[2]int{px, py}] = true
		return px, py, true
	}
	return 0, 0, false
}
