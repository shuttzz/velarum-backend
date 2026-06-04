package config

import (
	"math/rand"
	"testing"
)

func TestPlaceNewCity_SequentialFill(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	// 1ª cidade (todas as regiões vazias) → primeira região da ordem.
	k, _, _ := PlaceNewCity(rng, map[string]int{}, map[[2]int]bool{})
	if k != WorldRegions[0].Key {
		t.Fatalf("1ª cidade deveria ir para a 1ª região (%s), foi %s", WorldRegions[0].Key, k)
	}
	// Região 1 no teto → próxima vai para a 2ª.
	k2, _, _ := PlaceNewCity(rng, map[string]int{WorldRegions[0].Key: RegionCapacity}, map[[2]int]bool{})
	if k2 != WorldRegions[1].Key {
		t.Fatalf("região 1 cheia → deveria ir para a 2ª (%s), foi %s", WorldRegions[1].Key, k2)
	}
}

func TestPlaceNewCity_DistinctAndSpaced(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	counts := map[string]int{}
	taken := map[[2]int]bool{}
	coords := [][2]int{}
	for i := 0; i < 20; i++ {
		k, x, y := PlaceNewCity(rng, counts, taken)
		if taken[[2]int{x, y}] {
			t.Fatalf("posição duplicada na cidade %d: (%d,%d)", i, x, y)
		}
		for _, c := range coords {
			if HexDistance(x, y, c[0], c[1]) < citySpacing {
				t.Fatalf("cidades perto demais: (%d,%d) e (%d,%d)", x, y, c[0], c[1])
			}
		}
		coords = append(coords, [2]int{x, y})
		taken[[2]int{x, y}] = true
		counts[k]++
	}
}
