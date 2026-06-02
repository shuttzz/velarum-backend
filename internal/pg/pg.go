// Package pg cuida da conexão com o PostgreSQL e da aplicação das migrations (goose).
package pg

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registra o driver database/sql "pgx" (usado pelo goose)
	"github.com/pressly/goose/v3"

	"backend/migrations"
)

// Connect abre um pool de conexões com o PostgreSQL e valida com um ping.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Migrate aplica todas as migrations pendentes usando as migrations embutidas (idempotente).
func Migrate(url string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("abrir db para migration: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
