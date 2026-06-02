package auth

import "testing"

func TestHashTokenIsStableAndHex(t *testing.T) {
	h1 := hashToken("abc")
	h2 := hashToken("abc")
	if h1 != h2 {
		t.Fatal("hashToken não é determinístico")
	}
	if len(h1) != 64 {
		t.Fatalf("sha256 hex deve ter 64 chars, veio %d", len(h1))
	}
	if hashToken("abc") == hashToken("abd") {
		t.Fatal("tokens diferentes geraram o mesmo hash")
	}
}

func TestNewTokenIsRandomAndNonEmpty(t *testing.T) {
	a, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	b, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("token vazio")
	}
	if a == b {
		t.Fatal("dois tokens consecutivos iguais — entropia insuficiente")
	}
}

func TestValidUsername(t *testing.T) {
	ok := []string{"abc", "jogador_1", "Teste"}
	bad := []string{"ab", "", "tem espaco", "com@arroba", "umnomemuitolongoquepassadolimitemaximoxxx"}
	for _, u := range ok {
		if !validUsername(u) {
			t.Errorf("esperava válido: %q", u)
		}
	}
	for _, u := range bad {
		if validUsername(u) {
			t.Errorf("esperava inválido: %q", u)
		}
	}
}

func TestValidEmail(t *testing.T) {
	ok := []string{"a@b.com", "jogador@velarum.gg"}
	bad := []string{"", "semarroba.com", "@b.com", "a@bcom", "a@b.", "com espaco@b.com"}
	for _, e := range ok {
		if !validEmail(e) {
			t.Errorf("esperava válido: %q", e)
		}
	}
	for _, e := range bad {
		if validEmail(e) {
			t.Errorf("esperava inválido: %q", e)
		}
	}
}
