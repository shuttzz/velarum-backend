package city

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/config"
	"backend/internal/db"
	"backend/internal/pg"
)

func setupNodeTest(t *testing.T) (*Service, *pgxpool.Pool, context.Context, time.Time) {
	t.Helper()
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
	t.Cleanup(pool.Close)
	return NewService(pool), pool, ctx, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
}

// newNode cria um nó de recurso direto no banco, em coords FORA das zonas de spawn (cx+100, único
// por cidade) → sem colisão com cidades/nós semeados nem entre execuções.
func newNode(t *testing.T, q *db.Queries, ctx context.Context, res string, level, x, y int) db.WorldTarget {
	t.Helper()
	worldUUID, _ := db.ParseUUID(config.DefaultWorldID)
	n, err := q.InsertWorldTarget(ctx, db.InsertWorldTargetParams{
		WorldID: worldUUID, Kind: "node", Resource: res, Level: int32(level),
		CoordX: int32(x), CoordY: int32(y), AmountTotal: config.NodeAmountFor(res, level),
	})
	if err != nil {
		t.Fatalf("InsertWorldTarget: %v", err)
	}
	return n
}

// newCombatTarget cria uma aldeia/criatura (alvo de combate) direto no banco.
func newCombatTarget(t *testing.T, q *db.Queries, ctx context.Context, kind string, level, x, y int) db.WorldTarget {
	t.Helper()
	worldUUID, _ := db.ParseUUID(config.DefaultWorldID)
	defA, defH, reward := config.CombatTargetFor(kind, level)
	n, err := q.InsertWorldTarget(ctx, db.InsertWorldTargetParams{
		WorldID: worldUUID, Kind: kind, Level: int32(level), CoordX: int32(x), CoordY: int32(y),
		DefAttack: int32(defA), DefHp: int32(defH), RewardMatter: reward.Matter, RewardEnergy: reward.Energy, RewardKnowledge: reward.Knowledge,
	})
	if err != nil {
		t.Fatalf("InsertWorldTarget(combat): %v", err)
	}
	return n
}

// TTL: alvo de combate vencido SEM marcha a caminho é listado para expirar; COM marcha outbound
// (alguém indo atacar) ele NÃO expira (a marcha trava o TTL).
func TestCombatTargetTTL_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	c := enterTestGame(t, svc, pool, "brevali", now)
	worldUUID, _ := db.ParseUUID(config.DefaultWorldID)
	cityUUID, _ := db.ParseUUID(c.ID)
	past := pgTime(now.Add(-time.Minute))

	mk := func(x int) db.WorldTarget {
		defA, defH, reward := config.CombatTargetFor("village", 1)
		row, err := q.InsertWorldTarget(ctx, db.InsertWorldTargetParams{
			WorldID: worldUUID, Kind: "village", Level: 1, CoordX: int32(x), CoordY: int32(c.CoordY),
			DefAttack: int32(defA), DefHp: int32(defH), RewardMatter: reward.Matter, RewardEnergy: reward.Energy, RewardKnowledge: reward.Knowledge,
			ExpiresAt: past,
		})
		if err != nil {
			t.Fatalf("InsertWorldTarget: %v", err)
		}
		return row
	}
	expireFree := mk(c.CoordX + 100) // vencido, sem marcha → expira
	locked := mk(c.CoordX + 101)     // vencido, mas com marcha a caminho → NÃO expira

	troopsJSON, _ := json.Marshal(map[string]int{"lanceiro": 1})
	if _, err := q.InsertWorldMarch(ctx, db.InsertWorldMarchParams{
		WorldID: worldUUID, CityID: cityUUID, TargetID: locked.ID, Troops: troopsJSON, DepartAt: now, ArriveAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("InsertWorldMarch: %v", err)
	}

	ids, err := q.ListExpiredCombatTargets(ctx, db.ListExpiredCombatTargetsParams{WorldID: worldUUID, ExpiresAt: pgTime(now)})
	if err != nil {
		t.Fatalf("ListExpiredCombatTargets: %v", err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[db.UUIDString(id)] = true
	}
	if !got[db.UUIDString(expireFree.ID)] {
		t.Fatal("alvo vencido sem marcha deveria estar na lista de expiração")
	}
	if got[db.UUIDString(locked.ID)] {
		t.Fatal("alvo com marcha a caminho NÃO deveria expirar (TTL travado)")
	}
}

// Raid VITORIOSA numa aldeia: 20 lanceiros batem a defesa nível 1 → loot + alvo consumido
// (depleted) + sobreviventes voltam + relatório de raid.
func TestRaidWin_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	c := enterTestGame(t, svc, pool, "brevali", now)
	cityUUID, _ := db.ParseUUID(c.ID)
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: cityUUID, UnitType: "lanceiro", Count: 20}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}
	target := newCombatTarget(t, q, ctx, "village", 1, c.CoordX+100, c.CoordY)
	_, _, reward := config.CombatTargetFor("village", 1)

	m, err := svc.StartCollect(ctx, c.ID, db.UUIDString(target.ID), map[string]int{"lanceiro": 20}, now)
	if err != nil {
		t.Fatalf("StartCollect: %v", err)
	}
	if err := svc.ResolveWorldArrival(ctx, m.ID, m.ArriveAt); err != nil {
		t.Fatalf("ResolveWorldArrival: %v", err)
	}
	loaded, _ := svc.LoadCity(ctx, c.ID, m.ArriveAt)
	wm := loaded.WorldMarches[0]
	if wm.Status != "returning" || wm.AttackerWon == nil || !*wm.AttackerWon {
		t.Fatalf("esperava raid vitoriosa voltando: %+v (won=%v)", wm, wm.AttackerWon)
	}
	if wm.Loot.Matter != reward.Matter || wm.Loot.Energy != reward.Energy {
		t.Fatalf("loot esperado %+v, veio %+v", reward, wm.Loot)
	}
	got, _ := q.GetWorldTargetForUpdate(ctx, target.ID)
	if got.Status != "depleted" {
		t.Fatalf("alvo deveria estar consumido (depleted), veio %q", got.Status)
	}

	if err := svc.ResolveWorldReturn(ctx, m.ID, *wm.ReturnAt); err != nil {
		t.Fatalf("ResolveWorldReturn: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, c.ID, *wm.ReturnAt)
	if len(loaded.Troops) != 1 || loaded.Troops[0].Count <= 0 || loaded.Troops[0].Count > 20 {
		t.Fatalf("sobreviventes (1..20) deveriam voltar: %+v", loaded.Troops)
	}
	if loaded.Resources.Matter != 500+reward.Matter {
		t.Fatalf("loot não creditado: matéria %v, quero %v", loaded.Resources.Matter, 500+reward.Matter)
	}
	reps, _ := svc.ListReports(ctx, c.ID)
	if len(reps) != 1 || reps[0].Type != "raid" {
		t.Fatalf("esperava 1 relatório de raid, veio %+v", reps)
	}
}

// Raid DERROTA: pelotão fraco contra criatura nível 3 → perde, sem loot, alvo segue vivo.
func TestRaidLose_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	c := enterTestGame(t, svc, pool, "brevali", now)
	cityUUID, _ := db.ParseUUID(c.ID)
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: cityUUID, UnitType: "lanceiro", Count: 3}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}
	target := newCombatTarget(t, q, ctx, "creature", 3, c.CoordX+100, c.CoordY)

	m, err := svc.StartCollect(ctx, c.ID, db.UUIDString(target.ID), map[string]int{"lanceiro": 3}, now)
	if err != nil {
		t.Fatalf("StartCollect: %v", err)
	}
	if err := svc.ResolveWorldArrival(ctx, m.ID, m.ArriveAt); err != nil {
		t.Fatalf("ResolveWorldArrival: %v", err)
	}
	loaded, _ := svc.LoadCity(ctx, c.ID, m.ArriveAt)
	wm := loaded.WorldMarches[0]
	if wm.AttackerWon == nil || *wm.AttackerWon {
		t.Fatalf("esperava derrota: %+v", wm)
	}
	if wm.Loot.Matter != 0 {
		t.Fatalf("derrota não deveria ter loot: %+v", wm.Loot)
	}
	got, _ := q.GetWorldTargetForUpdate(ctx, target.ID)
	if got.Status == "depleted" {
		t.Fatal("alvo NÃO deveria ser consumido numa derrota")
	}
}

// Disputa: A mata o alvo; B (que já estava a caminho) chega depois → volta sem combater (bounce).
func TestRaidBounce_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	a := enterTestGame(t, svc, pool, "brevali", now)
	b := enterTestGame(t, svc, pool, "brevali", now)
	aUUID, _ := db.ParseUUID(a.ID)
	bUUID, _ := db.ParseUUID(b.ID)
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: aUUID, UnitType: "lanceiro", Count: 20}); err != nil {
		t.Fatalf("AddCityTroops A: %v", err)
	}
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: bUUID, UnitType: "lanceiro", Count: 20}); err != nil {
		t.Fatalf("AddCityTroops B: %v", err)
	}
	target := newCombatTarget(t, q, ctx, "village", 1, a.CoordX+100, a.CoordY)

	// Ambos partem com o alvo VIVO.
	ma, err := svc.StartCollect(ctx, a.ID, db.UUIDString(target.ID), map[string]int{"lanceiro": 20}, now)
	if err != nil {
		t.Fatalf("StartCollect A: %v", err)
	}
	mb, err := svc.StartCollect(ctx, b.ID, db.UUIDString(target.ID), map[string]int{"lanceiro": 20}, now)
	if err != nil {
		t.Fatalf("StartCollect B: %v", err)
	}
	// A chega e mata; B chega depois e dá bounce.
	if err := svc.ResolveWorldArrival(ctx, ma.ID, ma.ArriveAt); err != nil {
		t.Fatalf("ResolveWorldArrival A: %v", err)
	}
	if err := svc.ResolveWorldArrival(ctx, mb.ID, mb.ArriveAt); err != nil {
		t.Fatalf("ResolveWorldArrival B: %v", err)
	}
	loaded, _ := svc.LoadCity(ctx, b.ID, mb.ArriveAt)
	wm := loaded.WorldMarches[0]
	if wm.Status != "returning" || wm.AttackerWon != nil {
		t.Fatalf("B deveria dar bounce (sem combate): %+v", wm)
	}
	if wm.Loot.Matter != 0 || wm.Loot.Energy != 0 {
		t.Fatalf("B não deveria ter loot (bounce): %+v", wm.Loot)
	}
	if err := svc.ResolveWorldReturn(ctx, mb.ID, *wm.ReturnAt); err != nil {
		t.Fatalf("ResolveWorldReturn B: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, b.ID, *wm.ReturnAt)
	if len(loaded.Troops) != 1 || loaded.Troops[0].Count != 20 {
		t.Fatalf("20 lanceiros de B deveriam voltar intactos: %+v", loaded.Troops)
	}
}

// Loop completo de coleta com DEPLEÇÃO PARCIAL: 20 lanceiros (carga 500) num nó nível 3 (1500) →
// coleta 500, restam 1000; tropas voltam; loot creditado; relatório gerado.
func TestCollectFlow_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	c := enterTestGame(t, svc, pool, "brevali", now)

	cityUUID, _ := db.ParseUUID(c.ID)
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: cityUUID, UnitType: "lanceiro", Count: 20}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}
	node := newNode(t, q, ctx, "matter", 3, c.CoordX+100, c.CoordY)
	total := config.NodeAmountFor("matter", 3) // carga 500 < total → depleção PARCIAL
	rem := total - 500

	m, err := svc.StartCollect(ctx, c.ID, db.UUIDString(node.ID), map[string]int{"lanceiro": 20}, now)
	if err != nil {
		t.Fatalf("StartCollect: %v", err)
	}
	if m.Status != "outbound" {
		t.Fatalf("marcha deveria estar 'outbound', veio %q", m.Status)
	}
	loaded, _ := svc.LoadCity(ctx, c.ID, now)
	if len(loaded.Troops) != 0 {
		t.Fatalf("guarnição deveria estar vazia durante a marcha: %+v", loaded.Troops)
	}
	if len(loaded.WorldMarches) != 1 {
		t.Fatalf("esperava 1 marcha de nó ativa, veio %d", len(loaded.WorldMarches))
	}

	// Chegada → ocupa e coleta.
	if err := svc.ResolveWorldArrival(ctx, m.ID, m.ArriveAt); err != nil {
		t.Fatalf("ResolveWorldArrival: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, c.ID, m.ArriveAt)
	wm := loaded.WorldMarches[0]
	if wm.Status != "collecting" || wm.CollectUntil == nil {
		t.Fatalf("marcha deveria estar 'collecting' com collect_until: %+v", wm)
	}
	if wm.Loot.Matter != 500 {
		t.Fatalf("loot esperado 500 de matéria, veio %v", wm.Loot.Matter)
	}
	got, _ := q.GetWorldTargetForUpdate(ctx, node.ID)
	if got.Status != "occupied" || got.AmountRemaining != rem {
		t.Fatalf("nó deveria estar ocupado com %v restantes (depleção parcial), veio status=%q rem=%v", rem, got.Status, got.AmountRemaining)
	}

	// Coleta concluída → libera o nó e manda voltar.
	if err := svc.ResolveWorldCollect(ctx, m.ID, *wm.CollectUntil); err != nil {
		t.Fatalf("ResolveWorldCollect: %v", err)
	}
	got, _ = q.GetWorldTargetForUpdate(ctx, node.ID)
	if got.Status != "idle" || got.OccupiedBy.Valid {
		t.Fatalf("nó deveria estar liberado (idle, sem ocupante): status=%q occupied=%v", got.Status, got.OccupiedBy.Valid)
	}
	if got.AmountRemaining != rem {
		t.Fatalf("nó deveria manter %v (não respawna em depleção parcial), veio %v", rem, got.AmountRemaining)
	}
	loaded, _ = svc.LoadCity(ctx, c.ID, *wm.CollectUntil)
	rm := loaded.WorldMarches[0]
	if rm.Status != "returning" || rm.ReturnAt == nil {
		t.Fatalf("marcha deveria estar 'returning' com return_at: %+v", rm)
	}

	// Volta → tropas e loot chegam; relatório de coleta gerado.
	if err := svc.ResolveWorldReturn(ctx, m.ID, *rm.ReturnAt); err != nil {
		t.Fatalf("ResolveWorldReturn: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, c.ID, *rm.ReturnAt)
	if len(loaded.WorldMarches) != 0 {
		t.Fatalf("marcha deveria estar encerrada (done): %+v", loaded.WorldMarches)
	}
	if len(loaded.Troops) != 1 || loaded.Troops[0].Count != 20 {
		t.Fatalf("20 lanceiros deveriam voltar (sem batalha): %+v", loaded.Troops)
	}
	if loaded.Resources.Matter != 500+500 { // estoque inicial 500 + loot 500
		t.Fatalf("loot não creditado: matéria = %v, quero 1000", loaded.Resources.Matter)
	}
	reps, _ := svc.ListReports(ctx, c.ID)
	if len(reps) != 1 || reps[0].Type != "collection" {
		t.Fatalf("esperava 1 relatório de coleta, veio %+v", reps)
	}
}

// Drena um nó pequeno por completo → respawna (mesma linha, recarregada noutro lugar).
func TestCollectRespawn_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	c := enterTestGame(t, svc, pool, "brevali", now)

	cityUUID, _ := db.ParseUUID(c.ID)
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: cityUUID, UnitType: "lanceiro", Count: 20}); err != nil {
		t.Fatalf("AddCityTroops: %v", err)
	}
	node := newNode(t, q, ctx, "energy", 1, c.CoordX+100, c.CoordY) // energia nível 1 (escala 0.75); carga 500 → drena
	wantLoot := config.NodeAmountFor("energy", 1)

	m, err := svc.StartCollect(ctx, c.ID, db.UUIDString(node.ID), map[string]int{"lanceiro": 20}, now)
	if err != nil {
		t.Fatalf("StartCollect: %v", err)
	}
	if err := svc.ResolveWorldArrival(ctx, m.ID, m.ArriveAt); err != nil {
		t.Fatalf("ResolveWorldArrival: %v", err)
	}
	loaded, _ := svc.LoadCity(ctx, c.ID, m.ArriveAt)
	wm := loaded.WorldMarches[0]
	if wm.Loot.Energy != wantLoot {
		t.Fatalf("loot esperado %v de energia (drenou o nó), veio %v", wantLoot, wm.Loot.Energy)
	}
	if err := svc.ResolveWorldCollect(ctx, m.ID, *wm.CollectUntil); err != nil {
		t.Fatalf("ResolveWorldCollect: %v", err)
	}
	// Respawnou: mesma linha, recarregada (remaining == total > 0), idle.
	got, _ := q.GetWorldTargetForUpdate(ctx, node.ID)
	if got.Status != "idle" || got.AmountRemaining != got.AmountTotal || got.AmountRemaining <= 0 {
		t.Fatalf("nó deveria ter respawnado cheio: status=%q rem=%v total=%v", got.Status, got.AmountRemaining, got.AmountTotal)
	}
}

// Dois jogadores miram o MESMO nó: o 2º chega com o nó ocupado → volta sem coletar (bounce).
func TestCollectBounce_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	a := enterTestGame(t, svc, pool, "brevali", now)
	b := enterTestGame(t, svc, pool, "brevali", now)

	aUUID, _ := db.ParseUUID(a.ID)
	bUUID, _ := db.ParseUUID(b.ID)
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: aUUID, UnitType: "lanceiro", Count: 10}); err != nil {
		t.Fatalf("AddCityTroops A: %v", err)
	}
	if err := q.AddCityTroops(ctx, db.AddCityTroopsParams{CityID: bUUID, UnitType: "lanceiro", Count: 10}); err != nil {
		t.Fatalf("AddCityTroops B: %v", err)
	}
	node := newNode(t, q, ctx, "matter", 3, a.CoordX+100, a.CoordY)

	// A ocupa o nó.
	ma, err := svc.StartCollect(ctx, a.ID, db.UUIDString(node.ID), map[string]int{"lanceiro": 10}, now)
	if err != nil {
		t.Fatalf("StartCollect A: %v", err)
	}
	if err := svc.ResolveWorldArrival(ctx, ma.ID, ma.ArriveAt); err != nil {
		t.Fatalf("ResolveWorldArrival A: %v", err)
	}

	// B marcha e chega com o nó ocupado → bounce (volta sem loot).
	mb, err := svc.StartCollect(ctx, b.ID, db.UUIDString(node.ID), map[string]int{"lanceiro": 10}, now)
	if err != nil {
		t.Fatalf("StartCollect B: %v", err)
	}
	if err := svc.ResolveWorldArrival(ctx, mb.ID, mb.ArriveAt); err != nil {
		t.Fatalf("ResolveWorldArrival B: %v", err)
	}
	loaded, _ := svc.LoadCity(ctx, b.ID, mb.ArriveAt)
	wm := loaded.WorldMarches[0]
	if wm.Status != "returning" {
		t.Fatalf("marcha de B deveria voltar (bounce): %+v", wm)
	}
	if wm.Loot.Matter != 0 || wm.Loot.Energy != 0 || wm.Loot.Knowledge != 0 {
		t.Fatalf("B não deveria ter loot (bounce): %+v", wm.Loot)
	}
	// Volta de B: tropas intactas, sem loot, relatório bounced.
	if err := svc.ResolveWorldReturn(ctx, mb.ID, *wm.ReturnAt); err != nil {
		t.Fatalf("ResolveWorldReturn B: %v", err)
	}
	loaded, _ = svc.LoadCity(ctx, b.ID, *wm.ReturnAt)
	if len(loaded.Troops) != 1 || loaded.Troops[0].Count != 10 {
		t.Fatalf("10 lanceiros de B deveriam voltar intactos: %+v", loaded.Troops)
	}
	if loaded.Resources.Matter != 500 {
		t.Fatalf("B não deveria ganhar recurso (bounce): %v", loaded.Resources.Matter)
	}
}
