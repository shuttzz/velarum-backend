package config

import "testing"

func implementedCount() int {
	n := 0
	for _, b := range Era1Buildings {
		if b.Implemented {
			n++
		}
	}
	return n
}

func TestCatalogOnlyExposesImplemented(t *testing.T) {
	c := Catalog()
	if len(c.Buildings) != implementedCount() {
		t.Fatalf("catálogo deveria expor só os implementados (%d), veio %d", implementedCount(), len(c.Buildings))
	}
	for _, b := range c.Buildings {
		if def, ok := BuildingByKey(b.Key); !ok || !def.Implemented {
			t.Fatalf("catálogo expôs edifício não-implementado: %s", b.Key)
		}
	}
	// Placeholders (ainda sem efeito) ficam FORA do catálogo.
	present := map[string]bool{}
	for _, b := range c.Buildings {
		present[b.Key] = true
	}
	for _, ph := range []string{"altar_das_fogueiras", "torre_do_vigia", "circulo_runico", "praca_do_conselho", "pira_dos_guerreiros", "marco_primeiros_fogos"} {
		if present[ph] {
			t.Fatalf("placeholder não deveria estar no catálogo: %s", ph)
		}
	}
	if c.Growth.Production != ProductionGrowth || c.Growth.Cost != CostGrowth || c.Growth.BuildTime != BuildTimeGrowth {
		t.Fatalf("constantes de crescimento incorretas: %+v", c.Growth)
	}
}

func TestCatalogNormalizesFootprint(t *testing.T) {
	byKey := map[string]CatalogBuilding{}
	for _, b := range Catalog().Buildings {
		if b.Width < 1 || b.Height < 1 {
			t.Errorf("%s: footprint não normalizado (%dx%d)", b.Key, b.Width, b.Height)
		}
		byKey[b.Key] = b
	}
	// Os básicos (onboarding) não têm pré-requisito.
	if len(byKey["fogueira_comunal"].Requires) != 0 || len(byKey["canteiro_de_almas"].Requires) != 0 {
		t.Fatalf("básicos não deveriam ter pré-requisito: fogueira=%+v canteiro=%+v",
			byKey["fogueira_comunal"].Requires, byKey["canteiro_de_almas"].Requires)
	}
}
