package city

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/db"
)

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
