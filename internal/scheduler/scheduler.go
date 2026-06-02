// Package scheduler executa "eventos futuros agendados" — o coração temporal do gênero
// (construção termina às 14h32, tropa chega às 03h, etc.).
//
// A fonte de verdade dos eventos é o Store (será PostgreSQL). Por isso, após um restart,
// os eventos pendentes continuam sendo processados: o loop apenas lê o que está vencido no Store.
// O processamento é idempotente — um handler que falha NÃO marca o evento como processado,
// então ele é re-tentado no próximo ciclo.
package scheduler

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
)

// Event é um evento agendado para disparar em FiresAt.
type Event struct {
	ID      string
	Type    string
	FiresAt time.Time
	Payload json.RawMessage
}

// Store é a fonte de verdade dos eventos. Implementação de produção: PostgreSQL
// (tabela scheduled_events). Há um MemStore para desenvolvimento/testes.
type Store interface {
	Save(ctx context.Context, e Event) error
	DuePending(ctx context.Context, before time.Time) ([]Event, error)
	MarkProcessed(ctx context.Context, id string) error
}

// Handler processa um evento de um dado tipo. Deve ser IDEMPOTENTE.
type Handler func(ctx context.Context, e Event) error

// Scheduler despacha eventos vencidos para seus handlers em intervalos regulares.
type Scheduler struct {
	store    Store
	handlers map[string]Handler
	interval time.Duration
}

// New cria um Scheduler que verifica eventos vencidos a cada `interval`.
func New(store Store, interval time.Duration) *Scheduler {
	return &Scheduler{store: store, handlers: map[string]Handler{}, interval: interval}
}

// Handle registra um handler para um tipo de evento.
func (s *Scheduler) Handle(eventType string, h Handler) { s.handlers[eventType] = h }

// Schedule persiste um evento futuro. A verdade temporal é o FiresAt no Store.
func (s *Scheduler) Schedule(ctx context.Context, e Event) error { return s.store.Save(ctx, e) }

// Run roda o loop de despacho até o ctx ser cancelado.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.dispatch(ctx)
		}
	}
}

// dispatch processa todos os eventos vencidos uma vez.
func (s *Scheduler) dispatch(ctx context.Context) {
	due, err := s.store.DuePending(ctx, time.Now().UTC())
	if err != nil {
		log.Printf("scheduler: erro lendo eventos vencidos: %v", err)
		return
	}
	for _, e := range due {
		h, ok := s.handlers[e.Type]
		if !ok {
			log.Printf("scheduler: sem handler para tipo %q (evento %s)", e.Type, e.ID)
			continue
		}
		if err := h(ctx, e); err != nil {
			log.Printf("scheduler: handler %q falhou (evento %s): %v — será re-tentado", e.Type, e.ID, err)
			continue // não marca processado → re-tentativa no próximo ciclo
		}
		if err := s.store.MarkProcessed(ctx, e.ID); err != nil {
			log.Printf("scheduler: erro marcando evento %s como processado: %v", e.ID, err)
		}
	}
}

// MemStore é uma implementação em memória do Store, para desenvolvimento e testes.
// TODO: criar PgStore (PostgreSQL) lendo/gravando em scheduled_events, sem mudar a lógica do Scheduler.
type MemStore struct {
	mu     sync.Mutex
	events map[string]Event
}

// NewMemStore cria um Store em memória vazio.
func NewMemStore() *MemStore { return &MemStore{events: map[string]Event{}} }

func (m *MemStore) Save(_ context.Context, e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[e.ID] = e
	return nil
}

func (m *MemStore) DuePending(_ context.Context, before time.Time) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Event
	for _, e := range m.events {
		if !e.FiresAt.After(before) { // FiresAt <= before
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *MemStore) MarkProcessed(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.events, id)
	return nil
}
