package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDispatch_ProcessaVencidosEMantemFuturos(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	s := New(store, time.Hour)

	var processado string
	s.Handle("build.complete", func(_ context.Context, e Event) error {
		processado = string(e.Payload)
		return nil
	})

	_ = s.Schedule(ctx, Event{ID: "1", Type: "build.complete", FiresAt: time.Now().Add(-time.Minute), Payload: []byte(`"vencido"`)})
	_ = s.Schedule(ctx, Event{ID: "2", Type: "build.complete", FiresAt: time.Now().Add(time.Hour), Payload: []byte(`"futuro"`)})

	s.dispatch(ctx)

	if processado != `"vencido"` {
		t.Fatalf("evento vencido deveria ser processado, got %q", processado)
	}
	// Evento 1 marcado processado (removido); 2 continua pendente.
	due, _ := store.DuePending(ctx, time.Now().Add(2*time.Hour))
	if len(due) != 1 || due[0].ID != "2" {
		t.Fatalf("apenas o evento 2 deveria restar pendente, got %+v", due)
	}
}

func TestDispatch_HandlerComErroEhRetentado(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	s := New(store, time.Hour)

	calls := 0
	s.Handle("x", func(_ context.Context, _ Event) error {
		calls++
		return errors.New("falha simulada")
	})
	_ = s.Schedule(ctx, Event{ID: "1", Type: "x", FiresAt: time.Now().Add(-time.Minute), Payload: []byte("{}")})

	s.dispatch(ctx)
	s.dispatch(ctx) // como falhou, não foi marcado processado → deve rodar de novo

	if calls != 2 {
		t.Fatalf("handler que falha deve ser re-tentado; chamadas = %d, quero 2", calls)
	}
}

func TestDispatch_SemHandlerNaoQuebra(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()
	s := New(store, time.Hour)
	_ = s.Schedule(ctx, Event{ID: "1", Type: "desconhecido", FiresAt: time.Now().Add(-time.Minute), Payload: []byte("{}")})
	s.dispatch(ctx) // não deve entrar em pânico nem marcar processado
	due, _ := store.DuePending(ctx, time.Now())
	if len(due) != 1 {
		t.Fatalf("evento sem handler deve continuar pendente, got %+v", due)
	}
}
