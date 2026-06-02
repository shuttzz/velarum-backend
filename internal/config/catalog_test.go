package config

import "testing"

func TestCatalogExposesAllEra1Buildings(t *testing.T) {
	c := Catalog()
	if len(c.Buildings) != len(Era1Buildings) {
		t.Fatalf("esperava %d edifícios no catálogo, veio %d", len(Era1Buildings), len(c.Buildings))
	}
	if c.Growth.Production != ProductionGrowth || c.Growth.Cost != CostGrowth || c.Growth.BuildTime != BuildTimeGrowth {
		t.Fatalf("constantes de crescimento incorretas: %+v", c.Growth)
	}
}

func TestCatalogNormalizesFootprintAndPreservesRequires(t *testing.T) {
	byKey := map[string]CatalogBuilding{}
	for _, b := range Catalog().Buildings {
		if b.Width < 1 || b.Height < 1 {
			t.Errorf("%s: footprint não normalizado (%dx%d)", b.Key, b.Width, b.Height)
		}
		byKey[b.Key] = b
	}

	fog, ok := byKey["fogueira_comunal"]
	if !ok {
		t.Fatal("fogueira_comunal ausente do catálogo")
	}
	if len(fog.Requires) != 1 || fog.Requires[0].BuildingKey != "lar_do_cla" || fog.Requires[0].Level != 2 {
		t.Fatalf("requires de fogueira_comunal incorreto: %+v", fog.Requires)
	}

	// O Marco tem 2 pré-requisitos — todos devem ser preservados na serialização.
	marco, ok := byKey["marco_primeiros_fogos"]
	if !ok {
		t.Fatal("marco_primeiros_fogos ausente do catálogo")
	}
	if len(marco.Requires) != 2 {
		t.Fatalf("esperava 2 requires no marco, veio %d: %+v", len(marco.Requires), marco.Requires)
	}
}
