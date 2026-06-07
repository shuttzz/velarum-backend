package city

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/db"
)

// uniqAlliance gera nome+tag únicos e válidos (robusto a reexecuções contra o mesmo banco de
// teste — o nome/tag de aliança é UNIQUE por mundo; valores fixos colidem na 2ª execução).
func uniqAlliance(t *testing.T) (name, tag string) {
	t.Helper()
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	h := strings.ToUpper(hex.EncodeToString(b)) // 6 hex chars (uppercase)
	return "Clan " + h, h[:5]                    // nome 11 chars (3–24); tag 5 chars (2–5)
}

// enterTestGame cria uma conta única (email/username aleatórios, robusto a reexecuções
// contra o mesmo banco) e entra no mundo padrão, devolvendo a cidade inicial.
func enterTestGame(t *testing.T, svc *Service, pool *pgxpool.Pool, faction string, now time.Time) City {
	t.Helper()
	ctx := context.Background()

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	uniq := "t" + hex.EncodeToString(b)

	acc, err := db.New(pool).CreateAccount(ctx, db.CreateAccountParams{
		Username: uniq, Email: uniq + "@test.local", PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	c, err := svc.EnterWorld(ctx, db.UUIDString(acc.ID), faction, "Capital", now)
	if err != nil {
		t.Fatalf("EnterWorld: %v", err)
	}
	return c
}
