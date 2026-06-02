package city

import (
	"context"
	"os"
	"testing"
	"time"

	"backend/internal/domain/resource"
	"backend/internal/pg"
)

// Teste de INTEGRAÇÃO do ciclo de vida da cidade — roda só com TEST_DATABASE_URL
// (ex: `make itest`, contra o Postgres LOCAL do docker compose).
func TestCityLifecycle_Integration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL não definido — pulando teste de integração")
	}
	if err := pg.Migrate(url); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	pool, err := pg.Connect(ctx, url)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()

	svc := NewService(pool)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Criar novo jogo: mundo + jogador + cidade inicial (Era 1).
	city, err := svc.CreateNewGame(ctx, NewGameInput{
		WorldName: "Mundo Teste",
		Username:  "tester",
		Email:     "tester@example.com",
		Faction:   "aurenthos",
		CityName:  "Capital",
		CoordX:    0,
		CoordY:    0,
	}, now)
	if err != nil {
		t.Fatalf("CreateNewGame: %v", err)
	}
	if city.ID == "" {
		t.Fatal("cidade criada sem id")
	}
	if city.Era != 1 {
		t.Fatalf("era inicial = %d, quero 1", city.Era)
	}
	if city.Resources.Matter != 200 || city.Resources.Energy != 100 || city.Resources.Knowledge != 0 {
		t.Fatalf("recursos iniciais inesperados: %+v", city.Resources)
	}

	// Persistência: recarregar traz os mesmos dados (rate 0 → sem crescimento).
	loaded, err := svc.LoadCity(ctx, city.ID, now)
	if err != nil {
		t.Fatalf("LoadCity: %v", err)
	}
	if loaded.ID != city.ID || loaded.Name != "Capital" || loaded.Resources.Matter != 200 {
		t.Fatalf("cidade recarregada divergente: %+v", loaded)
	}

	// Lazy evaluation através do banco: liga produção de 60 Matéria/h e carrega 2h depois.
	if err := svc.SetProduction(ctx, city.ID, resource.Amounts{Matter: 60}, now); err != nil {
		t.Fatalf("SetProduction: %v", err)
	}
	later, err := svc.LoadCity(ctx, city.ID, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("LoadCity (2h depois): %v", err)
	}
	if later.Resources.Matter != 320 { // 200 + 60*2
		t.Fatalf("Matéria após 2h = %v, quero 320", later.Resources.Matter)
	}
}
