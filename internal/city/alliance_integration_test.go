package city

import (
	"errors"
	"testing"

	"backend/internal/db"
)

// Núcleo de alianças: criar (cobra premium) → B pede → A aprova → A promove B a oficial → B sai.
func TestAllianceFlow_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	q := db.New(pool)
	a := enterTestGame(t, svc, pool, "brevali", now)
	b := enterTestGame(t, svc, pool, "brevali", now)
	accA, _ := svc.OwnerAccountID(ctx, a.ID)
	accB, _ := svc.OwnerAccountID(ctx, b.ID)
	accAUUID, _ := db.ParseUUID(accA)

	// Criar aliança (A vira dono; debita 300 de premium dos 1000 iniciais).
	anName, anTag := uniqAlliance(t)
	mine, err := svc.CreateAlliance(ctx, accA, anName, anTag, now)
	if err != nil {
		t.Fatalf("CreateAlliance: %v", err)
	}
	if mine.MyRole != "owner" || mine.Alliance.Members != 1 {
		t.Fatalf("criador deveria ser owner com 1 membro: %+v", mine)
	}
	acc, _ := q.GetAccountByID(ctx, accAUUID)
	if acc.Premium != 1000-300 {
		t.Fatalf("premium deveria ser 700, veio %d", acc.Premium)
	}
	allianceID := mine.Alliance.ID

	// Não pode criar uma 2ª.
	if _, err := svc.CreateAlliance(ctx, accA, "Outra", "OUT", now); !errors.Is(err, ErrAlreadyInAlliance) {
		t.Fatalf("esperava ErrAlreadyInAlliance, veio %v", err)
	}

	// B pede pra entrar (modo padrão = approval).
	res, err := svc.JoinOrRequest(ctx, accB, allianceID, now)
	if err != nil || res != "requested" {
		t.Fatalf("JoinOrRequest: res=%q err=%v", res, err)
	}
	mine, _ = svc.MyAlliance(ctx, accA)
	if len(mine.Requests) != 1 {
		t.Fatalf("A deveria ver 1 pedido, veio %d", len(mine.Requests))
	}

	// A aprova.
	if err := svc.ApproveRequest(ctx, accA, mine.Requests[0].ID, true, now); err != nil {
		t.Fatalf("ApproveRequest: %v", err)
	}
	mine, _ = svc.MyAlliance(ctx, accA)
	if mine.Alliance.Members != 2 || len(mine.Requests) != 0 {
		t.Fatalf("deveria ter 2 membros e 0 pedidos: %+v", mine)
	}
	bMine, _ := svc.MyAlliance(ctx, accB)
	if bMine.MyRole != "member" {
		t.Fatalf("B deveria ser member, veio %q", bMine.MyRole)
	}

	// A promove B a oficial.
	if err := svc.SetMemberRole(ctx, accA, b.PlayerID, "officer"); err != nil {
		t.Fatalf("SetMemberRole: %v", err)
	}
	bMine, _ = svc.MyAlliance(ctx, accB)
	if bMine.MyRole != "officer" {
		t.Fatalf("B deveria ser officer, veio %q", bMine.MyRole)
	}

	// B sai.
	if err := svc.Leave(ctx, accB); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	mine, _ = svc.MyAlliance(ctx, accA)
	if mine.Alliance.Members != 1 {
		t.Fatalf("deveria voltar a 1 membro, veio %d", mine.Alliance.Members)
	}
	if _, err := svc.MyAlliance(ctx, accB); !errors.Is(err, ErrNotInAlliance) {
		t.Fatalf("B não deveria ter aliança, veio %v", err)
	}
}

// Entrada ABERTA: jogador entra direto, sem pedido.
func TestAllianceOpenJoin_Integration(t *testing.T) {
	svc, pool, ctx, now := setupNodeTest(t)
	_ = pool
	a := enterTestGame(t, svc, pool, "brevali", now)
	b := enterTestGame(t, svc, pool, "brevali", now)
	accA, _ := svc.OwnerAccountID(ctx, a.ID)
	accB, _ := svc.OwnerAccountID(ctx, b.ID)

	opName, opTag := uniqAlliance(t)
	mine, err := svc.CreateAlliance(ctx, accA, opName, opTag, now)
	if err != nil {
		t.Fatalf("CreateAlliance: %v", err)
	}
	if err := svc.SetEntryMode(ctx, accA, "open"); err != nil {
		t.Fatalf("SetEntryMode: %v", err)
	}
	res, err := svc.JoinOrRequest(ctx, accB, mine.Alliance.ID, now)
	if err != nil || res != "joined" {
		t.Fatalf("entrada aberta deveria juntar direto: res=%q err=%v", res, err)
	}
}
