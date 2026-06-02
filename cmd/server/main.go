// Comando server: entrypoint do backend do Velarum.
//
// Sobe HTTP mínimo (GET /health) e o scheduler de eventos. Se DATABASE_URL estiver
// definido, aplica as migrations (goose) e abre o pool do PostgreSQL.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/internal/config"
	"backend/internal/pg"
	"backend/internal/scheduler"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("velarum/backend: Era 1 com %d edifícios; recursos iniciais M=%.0f E=%.0f C=%.0f",
		len(config.Era1Buildings),
		config.StartingResources.Matter, config.StartingResources.Energy, config.StartingResources.Knowledge)

	// PostgreSQL: aplica migrations e conecta, se DATABASE_URL estiver definido.
	if url := os.Getenv("DATABASE_URL"); url != "" {
		if err := pg.Migrate(url); err != nil {
			log.Fatalf("migrations: %v", err)
		}
		pool, err := pg.Connect(ctx, url)
		if err != nil {
			log.Fatalf("postgres: %v", err)
		}
		defer pool.Close()
		log.Println("postgres: conectado e migrations aplicadas")
		_ = pool // será usado pelos repositórios nos próximos passos
	} else {
		log.Println("postgres: DATABASE_URL não definido — rodando sem banco (apenas /health e scheduler)")
	}

	// Scheduler de eventos futuros (store em memória por ora; PgStore depois).
	sch := scheduler.New(scheduler.NewMemStore(), time.Second)
	sch.Handle("build.complete", func(_ context.Context, e scheduler.Event) error {
		log.Printf("[evento] construção concluída: %s", string(e.Payload))
		return nil
	})
	go sch.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "velarum-backend"})
	})

	srv := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		log.Println("velarum/backend: ouvindo em :8080 (GET /health)")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("servidor: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("velarum/backend: encerrando...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
