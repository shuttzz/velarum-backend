package config

import "testing"

func TestArmyCap(t *testing.T) {
	if c := ArmyCap(0); c != 0 {
		t.Fatalf("sem Canteiro de Almas (nível 0) o teto deve ser 0, veio %d", c)
	}
	// Base 20 + 5 por nível.
	if c := ArmyCap(1); c != ArmyCapBase+ArmyCapPerBarracksLv {
		t.Fatalf("teto nível 1 = %d, quero %d", c, ArmyCapBase+ArmyCapPerBarracksLv)
	}
	if c := ArmyCap(3); c != ArmyCapBase+3*ArmyCapPerBarracksLv {
		t.Fatalf("teto nível 3 = %d, quero %d", c, ArmyCapBase+3*ArmyCapPerBarracksLv)
	}
}

func TestUnitByKey(t *testing.T) {
	if _, ok := UnitByKey("inexistente"); ok {
		t.Fatal("unidade inexistente não deveria existir")
	}
	u, ok := UnitByKey("lanceiro")
	if !ok {
		t.Fatal("lanceiro deveria existir no catálogo da Era 1")
	}
	if u.Category != "infantry" || u.HP <= 0 || u.Cost.Matter <= 0 {
		t.Fatalf("definição do lanceiro inesperada: %+v", u)
	}
}

func TestCatalogIncludesUnits(t *testing.T) {
	c := Catalog()
	if len(c.Units) != len(Era1Units) {
		t.Fatalf("catálogo deve expor %d unidades, veio %d", len(Era1Units), len(c.Units))
	}
}
