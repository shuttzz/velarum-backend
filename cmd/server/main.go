// Comando server: entrypoint do backend do Velarum.
//
// Rotas (o frontend acessa via proxy /api -> backend, que remove o prefixo /api):
//   GET  /health
//   GET  /catalog                                  -> catálogo de edifícios (Era 1) + crescimento
//   POST /auth/register                            -> cria conta {username, email, password} (auto-login)
//   POST /auth/login                               -> autentica {email, password} (seta cookie de sessão)
//   POST /auth/logout                              -> encerra a sessão
//   GET  /auth/me                                  -> conta autenticada (protegida)
//   POST /games                                    -> entra no mundo: cria/retorna a cidade da conta (protegida)
//   GET  /cities/{id}                              -> estado da cidade (recursos + grade) — só do dono
//   POST /cities/{id}/buildings                    -> constrói {building_type, x, y} — só do dono
//   POST /cities/{id}/buildings/{bid}/upgrade      -> upgrade do edifício {bid} — só do dono
//   POST /cities/{id}/buildings/{bid}/move         -> move o edifício {bid} para {x, y} — só do dono
//   POST /cities/{id}/builds/{bid}/cancel          -> cancela a obra pendente {bid} (devolve 100%) — só do dono
//   POST /cities/{id}/recruit                      -> recruta {unit_type, count} — só do dono
//   POST /cities/{id}/recruits/{rid}/cancel        -> cancela recrutamento {rid} (devolve 100%) — só do dono
//   GET  /cities/{id}/reports                      -> relatórios (caixa de entrada) — só do dono
//   POST /cities/{id}/reports/read                 -> marca todos os relatórios como lidos — só do dono
//   GET  /cities/{id}/provinces                    -> províncias PvE do jogador (mapa) — só do dono
//   POST /cities/{id}/march                        -> marcha {province_id, troops} — só do dono
//   POST /cities/{id}/provinces/{pid}/battle       -> inicia batalha tática contra a província {pid} {troops} — só do dono
//   GET  /cities/{id}/battles/{bid}                -> estado da batalha tática {bid} — só do dono
//   POST /cities/{id}/battles/{bid}/act            -> ação na batalha {unit_id, move_to?{q,r}, target_id?} — só do dono
//   POST /cities/{id}/battles/{bid}/end-turn       -> encerra o turno (roda a IA defensora) — só do dono
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

	"backend/internal/auth"
	"backend/internal/city"
	"backend/internal/config"
	"backend/internal/domain/battle"
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
	// Secure só em produção (HTTPS); em dev sob http o cookie Secure não é enviado.
	authSvc := auth.NewService(pool, os.Getenv("SESSION_COOKIE_SECURE") == "true")

	// Tick curto (250ms) para que obras/recrutamentos/marchas concluam logo após o finish_at,
	// mantendo o contador visual em sincronia com o servidor (o frontend refaz o fetch no zero).
	sch := scheduler.New(eventstore.NewPgStore(pool), 250*time.Millisecond)
	sch.Handle(city.EventBuildComplete, func(ctx context.Context, e scheduler.Event) error {
		return citySvc.CompleteBuildEvent(ctx, e.Payload, time.Now().UTC())
	})
	sch.Handle(city.EventRecruitComplete, func(ctx context.Context, e scheduler.Event) error {
		return citySvc.CompleteRecruitEvent(ctx, e.Payload, time.Now().UTC())
	})
	sch.Handle(city.EventTroopArrival, func(ctx context.Context, e scheduler.Event) error {
		return citySvc.ResolveArrivalEvent(ctx, e.Payload, time.Now().UTC())
	})
	sch.Handle(city.EventTroopReturn, func(ctx context.Context, e scheduler.Event) error {
		return citySvc.ResolveReturnEvent(ctx, e.Payload, time.Now().UTC())
	})
	go sch.Run(ctx)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "velarum-backend"})
	})

	mux.HandleFunc("GET /catalog", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, config.Catalog())
	})

	// Cidades do mundo COMPARTILHADO (vizinhos no mapa). Protegida (basta estar logado).
	mux.HandleFunc("GET /world/cities", authSvc.Require(func(w http.ResponseWriter, r *http.Request) {
		cities, err := citySvc.WorldCities(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, cities)
	}))

	mux.HandleFunc("POST /auth/register", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		acc, err := authSvc.Register(r.Context(), body.Username, body.Email, body.Password)
		if err != nil {
			writeErr(w, statusForAuthErr(err), err)
			return
		}
		// Auto-login após registro: já entrega a sessão.
		if token, exp, _, err := authSvc.Login(r.Context(), body.Email, body.Password, time.Now().UTC()); err == nil {
			authSvc.SetSessionCookie(w, token, exp)
		}
		writeJSON(w, http.StatusCreated, acc)
	})

	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		token, exp, acc, err := authSvc.Login(r.Context(), body.Email, body.Password, time.Now().UTC())
		if err != nil {
			writeErr(w, statusForAuthErr(err), err)
			return
		}
		authSvc.SetSessionCookie(w, token, exp)
		writeJSON(w, http.StatusOK, acc)
	})

	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(auth.SessionCookieName); err == nil {
			_ = authSvc.Logout(r.Context(), c.Value)
		}
		authSvc.ClearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /auth/me", authSvc.Require(func(w http.ResponseWriter, r *http.Request) {
		accountID, _ := auth.AccountID(r.Context())
		acc, err := authSvc.AccountByID(r.Context(), accountID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, acc)
	}))

	mux.HandleFunc("POST /games", authSvc.Require(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Faction  string `json:"faction"`
			CityName string `json:"city_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		accountID, _ := auth.AccountID(r.Context())
		c, err := citySvc.EnterWorld(r.Context(), accountID, body.Faction, body.CityName, time.Now().UTC())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, c)
	}))

	// ownedCity exige sessão E que a cidade {id} pertença à conta autenticada (senão 403/404).
	ownedCity := func(next http.HandlerFunc) http.HandlerFunc {
		return authSvc.Require(func(w http.ResponseWriter, r *http.Request) {
			accountID, _ := auth.AccountID(r.Context())
			owner, err := citySvc.OwnerAccountID(r.Context(), r.PathValue("id"))
			if err != nil {
				writeCode(w, http.StatusNotFound, "city_not_found", err.Error())
				return
			}
			if owner != accountID {
				writeCode(w, http.StatusForbidden, "forbidden_not_owner", "cidade não pertence à conta")
				return
			}
			next(w, r)
		})
	}

	mux.HandleFunc("GET /cities/{id}", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		c, err := citySvc.LoadCity(r.Context(), r.PathValue("id"), time.Now().UTC())
		if err != nil {
			writeCode(w, http.StatusNotFound, "city_not_found", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, c)
	}))

	mux.HandleFunc("POST /cities/{id}/buildings", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			BuildingType string `json:"building_type"`
			X            int    `json:"x"`
			Y            int    `json:"y"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		bq, err := citySvc.EnqueueConstruct(r.Context(), r.PathValue("id"), body.BuildingType, body.X, body.Y, time.Now().UTC())
		if err != nil {
			writeErr(w, statusForBuildErr(err), err)
			return
		}
		writeJSON(w, http.StatusAccepted, bq)
	}))

	mux.HandleFunc("POST /cities/{id}/buildings/{bid}/upgrade", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		bq, err := citySvc.EnqueueUpgrade(r.Context(), r.PathValue("id"), r.PathValue("bid"), time.Now().UTC())
		if err != nil {
			writeErr(w, statusForBuildErr(err), err)
			return
		}
		writeJSON(w, http.StatusAccepted, bq)
	}))

	mux.HandleFunc("POST /cities/{id}/builds/{bid}/cancel", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		if err := citySvc.CancelBuild(r.Context(), r.PathValue("id"), r.PathValue("bid"), time.Now().UTC()); err != nil {
			writeErr(w, statusForBuildErr(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /cities/{id}/reports", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		reps, err := citySvc.ListReports(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, reps)
	}))

	mux.HandleFunc("POST /cities/{id}/reports/read", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		if err := citySvc.MarkReportsRead(r.Context(), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /cities/{id}/recruits/{rid}/cancel", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		if err := citySvc.CancelRecruit(r.Context(), r.PathValue("id"), r.PathValue("rid"), time.Now().UTC()); err != nil {
			writeErr(w, statusForBuildErr(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /cities/{id}/provinces", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		provs, err := citySvc.ListProvinces(r.Context(), r.PathValue("id"), time.Now().UTC())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, provs)
	}))

	mux.HandleFunc("POST /cities/{id}/march", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProvinceID string         `json:"province_id"`
			Troops     map[string]int `json:"troops"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		m, err := citySvc.StartMarch(r.Context(), r.PathValue("id"), body.ProvinceID, body.Troops, time.Now().UTC())
		if err != nil {
			writeErr(w, statusForMarchErr(err), err)
			return
		}
		writeJSON(w, http.StatusAccepted, m)
	}))

	mux.HandleFunc("POST /cities/{id}/provinces/{pid}/battle", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Troops map[string]int `json:"troops"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		b, err := citySvc.StartBattle(r.Context(), r.PathValue("id"), r.PathValue("pid"), body.Troops, time.Now().UTC())
		if err != nil {
			writeErr(w, statusForBattleErr(err), err)
			return
		}
		writeJSON(w, http.StatusCreated, b)
	}))

	mux.HandleFunc("GET /cities/{id}/battles/{bid}", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		b, err := citySvc.GetBattle(r.Context(), r.PathValue("id"), r.PathValue("bid"))
		if err != nil {
			writeErr(w, statusForBattleErr(err), err)
			return
		}
		writeJSON(w, http.StatusOK, b)
	}))

	mux.HandleFunc("POST /cities/{id}/battles/{bid}/act", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UnitID   string     `json:"unit_id"`
			MoveTo   *battle.Hex `json:"move_to"`
			TargetID string     `json:"target_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		b, err := citySvc.BattleAct(r.Context(), r.PathValue("id"), r.PathValue("bid"), body.UnitID, body.MoveTo, body.TargetID, time.Now().UTC())
		if err != nil {
			writeErr(w, statusForBattleErr(err), err)
			return
		}
		writeJSON(w, http.StatusOK, b)
	}))

	mux.HandleFunc("POST /cities/{id}/battles/{bid}/end-turn", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		b, err := citySvc.BattleEndTurn(r.Context(), r.PathValue("id"), r.PathValue("bid"), time.Now().UTC())
		if err != nil {
			writeErr(w, statusForBattleErr(err), err)
			return
		}
		writeJSON(w, http.StatusOK, b)
	}))

	mux.HandleFunc("POST /cities/{id}/recruit", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UnitType string `json:"unit_type"`
			Count    int    `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		rq, err := citySvc.EnqueueRecruit(r.Context(), r.PathValue("id"), body.UnitType, body.Count, time.Now().UTC())
		if err != nil {
			writeErr(w, statusForRecruitErr(err), err)
			return
		}
		writeJSON(w, http.StatusAccepted, rq)
	}))

	mux.HandleFunc("POST /cities/{id}/buildings/{bid}/move", ownedCity(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			X int `json:"x"`
			Y int `json:"y"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if err := citySvc.MoveBuilding(r.Context(), r.PathValue("id"), r.PathValue("bid"), body.X, body.Y, time.Now().UTC()); err != nil {
			writeErr(w, statusForBuildErr(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

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
		errors.Is(err, city.ErrBuildingBusy), errors.Is(err, city.ErrNotCancelable),
		errors.Is(err, city.ErrQueueFull):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func statusForMarchErr(err error) int {
	switch {
	case errors.Is(err, city.ErrUnitUnknown), errors.Is(err, city.ErrBadCount):
		return http.StatusBadRequest
	case errors.Is(err, city.ErrProvinceNotFound):
		return http.StatusNotFound
	case errors.Is(err, city.ErrProvinceConquered), errors.Is(err, city.ErrNoTroops),
		errors.Is(err, city.ErrQueueFull):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func statusForBattleErr(err error) int {
	switch {
	case errors.Is(err, city.ErrUnitUnknown), errors.Is(err, city.ErrBadCount):
		return http.StatusBadRequest
	case errors.Is(err, city.ErrBattleNotFound), errors.Is(err, city.ErrProvinceNotFound):
		return http.StatusNotFound
	case errors.Is(err, city.ErrBattleActive), errors.Is(err, city.ErrProvinceConquered),
		errors.Is(err, city.ErrNoTroops), errors.Is(err, city.ErrInvalidAction):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func statusForRecruitErr(err error) int {
	switch {
	case errors.Is(err, city.ErrUnitUnknown), errors.Is(err, city.ErrBadCount):
		return http.StatusBadRequest
	case errors.Is(err, city.ErrNoBarracks), errors.Is(err, city.ErrArmyCapExceeded),
		errors.Is(err, city.ErrUnitLocked), errors.Is(err, city.ErrInsufficient),
		errors.Is(err, city.ErrRecruitBusy):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func statusForAuthErr(err error) int {
	switch {
	case errors.Is(err, auth.ErrInvalidInput), errors.Is(err, auth.ErrWeakPassword):
		return http.StatusBadRequest
	case errors.Is(err, auth.ErrEmailTaken), errors.Is(err, auth.ErrUsernameTaken):
		return http.StatusConflict
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrNoSession):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr responde um erro com um CÓDIGO estável (i18n: o frontend traduz pelo code) e o
// texto em pt-BR como fallback de dev/log. Cf. memory i18n-arquitetura.
func writeErr(w http.ResponseWriter, status int, err error) {
	writeCode(w, status, codeFor(err), err.Error())
}

// writeCode responde um erro com code explícito (ex.: erro de parsing de corpo).
func writeCode(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "error": msg})
}

// codeFor mapeia os erros sentinela do domínio para códigos estáveis (independentes de idioma).
func codeFor(err error) string {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, auth.ErrWeakPassword):
		return "weak_password"
	case errors.Is(err, auth.ErrEmailTaken):
		return "email_taken"
	case errors.Is(err, auth.ErrUsernameTaken):
		return "username_taken"
	case errors.Is(err, auth.ErrInvalidCredentials):
		return "invalid_credentials"
	case errors.Is(err, auth.ErrNoSession):
		return "unauthenticated"
	case errors.Is(err, city.ErrBuildingUnknown):
		return "building_unknown"
	case errors.Is(err, city.ErrBuildingNotFound):
		return "building_not_found"
	case errors.Is(err, city.ErrInsufficient):
		return "insufficient"
	case errors.Is(err, city.ErrPrereqNotMet):
		return "prereq_not_met"
	case errors.Is(err, city.ErrMaxCopies):
		return "max_copies"
	case errors.Is(err, city.ErrBadPlacement):
		return "bad_placement"
	case errors.Is(err, city.ErrBuildingBusy):
		return "building_busy"
	case errors.Is(err, city.ErrNotCancelable):
		return "not_cancelable"
	case errors.Is(err, city.ErrQueueFull):
		return "queue_full"
	case errors.Is(err, city.ErrUnitUnknown):
		return "unit_unknown"
	case errors.Is(err, city.ErrBadCount):
		return "bad_count"
	case errors.Is(err, city.ErrNoBarracks):
		return "no_barracks"
	case errors.Is(err, city.ErrArmyCapExceeded):
		return "army_cap_exceeded"
	case errors.Is(err, city.ErrUnitLocked):
		return "unit_locked"
	case errors.Is(err, city.ErrRecruitBusy):
		return "recruit_busy"
	case errors.Is(err, city.ErrProvinceNotFound):
		return "province_not_found"
	case errors.Is(err, city.ErrProvinceConquered):
		return "province_conquered"
	case errors.Is(err, city.ErrNoTroops):
		return "no_troops"
	case errors.Is(err, city.ErrBattleActive):
		return "battle_active"
	case errors.Is(err, city.ErrBattleNotFound):
		return "battle_not_found"
	case errors.Is(err, city.ErrInvalidAction):
		return "invalid_action"
	default:
		return "internal"
	}
}
