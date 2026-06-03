package resource

import (
	"testing"
	"time"
)

func TestAt_AcumulaSemTeto(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := State{
		Stored:      Amounts{Matter: 100},
		RatePerHour: Amounts{Matter: 50},
		Capacity:    Amounts{Matter: 300}, // parcela protegida, NÃO teto
		UpdatedAt:   t0,
	}

	// 2h depois: 100 + 50*2 = 200
	if got := s.At(t0.Add(2 * time.Hour)).Matter; got != 200 {
		t.Fatalf("Matter após 2h: esperado 200, obtido %v", got)
	}
	// 10h depois: 100 + 50*10 = 600 — sobe livre, SEM teto (Capacity não limita)
	if got := s.At(t0.Add(10 * time.Hour)).Matter; got != 600 {
		t.Fatalf("Matter após 10h: esperado 600 (sem teto), obtido %v", got)
	}
}

func TestRaidable(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := State{Stored: Amounts{Matter: 500}, Capacity: Amounts{Matter: 300}, UpdatedAt: t0}
	// 500 estocado, 300 protegido → 200 saqueável.
	if got := s.Raidable(t0).Matter; got != 200 {
		t.Fatalf("saqueável: esperado 200, obtido %v", got)
	}
	// Abaixo da parcela protegida → nada saqueável.
	s2 := State{Stored: Amounts{Matter: 100}, Capacity: Amounts{Matter: 300}, UpdatedAt: t0}
	if got := s2.Raidable(t0).Matter; got != 0 {
		t.Fatalf("saqueável (abaixo do protegido): esperado 0, obtido %v", got)
	}
}

func TestAt_TempoNegativoNaoRegride(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := State{Stored: Amounts{Matter: 100}, RatePerHour: Amounts{Matter: 50}, Capacity: Amounts{Matter: 1000}, UpdatedAt: t0}
	if got := s.At(t0.Add(-3 * time.Hour)).Matter; got != 100 {
		t.Fatalf("tempo negativo não deve regredir recursos: esperado 100, obtido %v", got)
	}
}

func TestSpend(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := State{Stored: Amounts{Matter: 100}, RatePerHour: Amounts{Matter: 50}, Capacity: Amounts{Matter: 1000}, UpdatedAt: t0}
	now := t0.Add(1 * time.Hour) // 100 + 50 = 150 disponível

	ns, ok := s.Spend(Amounts{Matter: 120}, now)
	if !ok {
		t.Fatal("deveria haver recursos para gastar 120 de 150")
	}
	if ns.Stored.Matter != 30 {
		t.Fatalf("após gastar 120 de 150: esperado 30, obtido %v", ns.Stored.Matter)
	}
	if !ns.UpdatedAt.Equal(now) {
		t.Fatalf("Spend deve atualizar UpdatedAt para now")
	}

	if _, ok := s.Spend(Amounts{Matter: 1000}, now); ok {
		t.Fatal("não deveria conseguir gastar 1000 com apenas 150 disponíveis")
	}
}
