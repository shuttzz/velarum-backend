// Package eventstore implementa scheduler.Store sobre a tabela scheduled_events (PostgreSQL),
// substituindo o MemStore. Eventos sobrevivem a restart: o loop do scheduler lê do banco.
package eventstore

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/db"
	"backend/internal/scheduler"
)

// PgStore é o Store do scheduler respaldado por PostgreSQL.
type PgStore struct {
	q *db.Queries
}

// NewPgStore cria um PgStore sobre o pool informado.
func NewPgStore(pool *pgxpool.Pool) *PgStore {
	return &PgStore{q: db.New(pool)}
}

// Save persiste um evento futuro (id gerado pelo banco — uuidv7).
func (p *PgStore) Save(ctx context.Context, e scheduler.Event) error {
	_, err := p.q.InsertScheduledEvent(ctx, db.InsertScheduledEventParams{
		Type:    e.Type,
		FiresAt: e.FiresAt,
		Payload: e.Payload,
	})
	return err
}

// DuePending retorna os eventos pendentes cujo fires_at já venceu.
func (p *PgStore) DuePending(ctx context.Context, before time.Time) ([]scheduler.Event, error) {
	rows, err := p.q.DuePendingEvents(ctx, before)
	if err != nil {
		return nil, err
	}
	events := make([]scheduler.Event, 0, len(rows))
	for _, r := range rows {
		events = append(events, scheduler.Event{
			ID:      db.UUIDString(r.ID),
			Type:    r.Type,
			FiresAt: r.FiresAt,
			Payload: r.Payload,
		})
	}
	return events, nil
}

// MarkProcessed marca um evento como processado.
func (p *PgStore) MarkProcessed(ctx context.Context, id string) error {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return p.q.MarkEventProcessed(ctx, uid)
}
