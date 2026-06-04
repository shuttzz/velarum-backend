package config

import "testing"

func TestQueuesForEra(t *testing.T) {
	cases := map[int]int{1: 1, 2: 1, 3: 2, 4: 2, 5: 3, 6: 4, 7: 5}
	for era, want := range cases {
		if got := QueuesForEra(era); got != want {
			t.Errorf("QueuesForEra(%d) = %d, quero %d", era, got, want)
		}
	}
	// Teto absoluto = 5 mesmo em eras além de 7 (defensivo).
	if got := QueuesForEra(9); got != 5 {
		t.Errorf("QueuesForEra(9) = %d, quero 5 (teto)", got)
	}
}
