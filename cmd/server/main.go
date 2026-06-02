// Comando server: entrypoint do backend do Velarum.
//
// Rotas (o frontend acessa via proxy /api -> backend, que remove o prefixo /api):
//   GET  /health
//   POST /games                                    -> cria mundo+jogador+cidade inicial
//   GET  /cities/{id}                              -> estado da cidade (recursos + grade)
//   POST /cities/{id}/buildings                    -> constrói {building_type, x, y}
//   POST /cities/{id}/buildings/{bid}/upgrade      -> upgrade do edifício {bid}
//   POST /cities/{id}/buildings/{bid}/move         -> move o edifício {bid} para {x, y}
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/internal/city"
	"backend/internal/config"
	"backend/internal/eventstore"
	"backend/internal/pg"
	"backend/internal/scheduler"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("velarum/backend: Era 1 com %d edifícios", len(config.Era1Buildings))

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL não definido — o backend requer PostgreSQL")
	}
	if err := pg.Migrate(url); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	pool, err := pg.Connect(ctx, url)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()
	log.Println("postgres: conectado e migrations aplicadas")

	citySvc := city.NewService(pool)

	sch := scheduler.New(eventstore.NewPgStore(pool), time.Second)
	sch.Handle(city.EventBuildComplete, func(ctx context.Context, e scheduler.Event) error {
		return citySvc.CompleteBuildEvent(ctx, e.Payload, time.Now().UTC())
	})
	go sch.Run(ctx)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "velarum-backend"})
	})

	mux.HandleFunc("POST /games", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Faction  string `json:"faction"`
			CityName string `json:"city_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Faction == "" {
			body.Faction = "aurenthos"
		}
		if body.CityName == "" {
			body.CityName = "Capital"
		}
		c, err := citySvc.CreateNewGame(r.Context(), city.NewGameInput{
			WorldName: "Velarum", Username: "jogador", Email: "jogador@velarum.local",
			Faction: body.Faction, CityName: body.CityName, CoordX: 0, CoordY: 0,
		}, time.Now().UTC())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, c)
	})

	mux.HandleFunc("GET /cities/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, err := citySvc.LoadCity(r.Context(), r.PathValue("id"), time.Now().UTC())
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	})

	mux.HandleFunc("POST /cities/{id}/buildings", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			BuildingType string `json:"building_type"`
			X            int    `json:"x"`
			Y            int    `json:"y"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		bq, err := citySvc.EnqueueConstruct(r.Context(), r.PathValue("id"), body.BuildingType, body.X, body.Y, time.Now().UTC())
		if err != nil {
			writeErr(w, statusForBuildErr(err), err)
			return
		}
		writeJSON(w, http.StatusAccepted, bq)
	})

	mux.HandleFunc("POST /cities/{id}/buildings/{bid}/upgrade", func(w http.ResponseWriter, r *http.Request) {
		bq, err := citySvc.EnqueueUpgrade(r.Context(), r.PathValue("id"), r.PathValue("bid"), time.Now().UTC())
		if err != nil {
			writeErr(w, statusForBuildErr(err), err)
			return
		}
		writeJSON(w, http.StatusAccepted, bq)
	})

	mux.HandleFunc("POST /cities/{id}/buildings/{bid}/move", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			X int `json:"x"`
			Y int `json:"y"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := citySvc.MoveBuilding(r.Context(), r.PathValue("id"), r.PathValue("bid"), body.X, body.Y, time.Now().UTC()); err != nil {
			writeErr(w, statusForBuildErr(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		log.Println("velarum/backend: ouvindo em :8080")
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

func statusForBuildErr(err error) int {
	switch {
	case errors.Is(err, city.ErrBuildingUnknown):
		return http.StatusBadRequest
	case errors.Is(err, city.ErrBuildingNotFound):
		return http.StatusNotFound
	case errors.Is(err, city.ErrInsufficient), errors.Is(err, city.ErrPrereqNotMet),
		errors.Is(err, city.ErrMaxCopies), errors.Is(err, city.ErrBadPlacement),
		errors.Is(err, city.ErrBuildingBusy):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
