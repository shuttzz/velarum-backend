package config

import (
	"testing"
	"time"

	"backend/internal/domain/resource"
)

func TestProductionPerHour(t *testing.T) {
	cases := []struct {
		level int
		want  float64
	}{
		{1, 8},  // base
		{2, 12}, // floor(8 * 1.55)
		{3, 19}, // floor(8 * 1.55^2)
		{4, 29}, // floor(8 * 1.55^3)
	}
	for _, c := range cases {
		if got := ProductionPerHour(8, c.level); got != c.want {
			t.Errorf("ProductionPerHour(8,%d) = %v, quero %v", c.level, got, c.want)
		}
	}
}

func TestCostFor(t *testing.T) {
	base := resource.Amounts{Matter: 60, Energy: 20}
	// nível 2: fator 1.65
	got := CostFor(base, 2)
	if got.Matter != 99 || got.Energy != 33 {
		t.Errorf("CostFor nível 2 = %+v, quero Matter=99 Energy=33", got)
	}
	// nível 1: custo base inalterado
	if got1 := CostFor(base, 1); got1.Matter != 60 || got1.Energy != 20 {
		t.Errorf("CostFor nível 1 deve ser o custo base, got %+v", got1)
	}
}

func TestBuildTimeFor(t *testing.T) {
	if got := BuildTimeFor(30, 1); got != 30*time.Second {
		t.Errorf("BuildTimeFor(30,1) = %v, quero 30s", got)
	}
	if got := BuildTimeFor(30, 2); got != 54*time.Second {
		t.Errorf("BuildTimeFor(30,2) = %v, quero 54s", got)
	}
}

func TestStorageCapFor(t *testing.T) {
	if got := StorageCapFor(1); got != 500 {
		t.Errorf("StorageCapFor(1) = %v, quero 500", got)
	}
	if got := StorageCapFor(2); got != 850 {
		t.Errorf("StorageCapFor(2) = %v, quero 850", got)
	}
}

func TestEra1BuildingsConsistencia(t *testing.T) {
	seen := map[string]bool{}
	var temMarco, temCentral bool
	for _, b := range Era1Buildings {
		if b.Key == "" || b.Name == "" {
			t.Errorf("edifício sem key/nome: %+v", b)
		}
		if seen[b.Key] {
			t.Errorf("key duplicada: %s", b.Key)
		}
		seen[b.Key] = true
		if b.MaxCopies < 1 {
			t.Errorf("%s: MaxCopies deve ser >= 1", b.Key)
		}
		if b.Category == "marco" {
			temMarco = true
		}
		if b.Category == "central" {
			temCentral = true
		}
		// dependências devem apontar para edifícios existentes (verificado depois do loop)
	}
	if !temMarco {
		t.Error("Era 1 precisa ter um Marco da Era")
	}
	if !temCentral {
		t.Error("Era 1 precisa ter um edifício central")
	}
	// valida que toda dependência referencia uma key existente
	for _, b := range Era1Buildings {
		for _, r := range b.Requires {
			if !seen[r.BuildingKey] {
				t.Errorf("%s depende de %q que não existe na Era 1", b.Key, r.BuildingKey)
			}
		}
	}
}
