package pg

import (
	"context"
	"os"
	"testing"
)

// Teste de INTEGRAÇÃO: roda apenas se TEST_DATABASE_URL estiver definido
// (ex: `make itest`, que sobe o Postgres local do docker compose).
// Princípio do projeto: integração de DB sempre contra Postgres LOCAL, nunca nuvem.
func TestMigrateEInsere_Integration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL não definido — pulando teste de integração")
	}

	if err := Migrate(url); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()

	// Insere um mundo e lê de volta — valida schema + DEFAULT uuidv7().
	var id, name string
	err = pool.QueryRow(ctx,
		`INSERT INTO worlds (name) VALUES ($1) RETURNING id, name`,
		"Mundo de Teste",
	).Scan(&id, &name)
	if err != nil {
		t.Fatalf("insert world: %v", err)
	}
	if name != "Mundo de Teste" {
		t.Fatalf("nome inesperado: %q", name)
	}
	if id == "" {
		t.Fatal("id (uuidv7) veio vazio")
	}
	t.Logf("mundo criado com id %s", id)
}
