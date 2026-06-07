package city

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"backend/internal/config"
	"backend/internal/db"
)

// SW3 — ALIANÇAS (núcleo de membros). Papéis owner|leader|officer|member; entrada open|approval;
// criar custa moeda premium (config.AllianceCreateCost). Cf. design-aliancas. Operações por JOGADOR
// (resolvido da conta no mundo padrão), não por cidade.

// Erros de aliança (mapeáveis para 4xx).
var (
	ErrNoPlayerYet         = errors.New("entre no mundo primeiro")
	ErrAllianceNotFound    = errors.New("aliança não encontrada")
	ErrAlreadyInAlliance   = errors.New("você já está numa aliança")
	ErrNotInAlliance       = errors.New("você não está numa aliança")
	ErrAllianceNameTaken   = errors.New("nome de aliança já em uso")
	ErrAllianceTagTaken    = errors.New("tag de aliança já em uso")
	ErrBadAllianceName     = errors.New("nome/tag inválidos")
	ErrInsufficientPremium = errors.New("moeda premium insuficiente")
	ErrAllianceFull        = errors.New("aliança lotada")
	ErrAllianceForbidden   = errors.New("sem permissão na aliança")
	ErrNoJoinRequest       = errors.New("pedido não encontrado")
)

// Alliance é a visão de domínio de uma aliança.
type Alliance struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Tag       string `json:"tag"`
	EntryMode string `json:"entry_mode"`
	MemberCap int    `json:"member_cap"`
	Members   int    `json:"members"`
}

// AllianceMember é um membro no roster.
type AllianceMember struct {
	PlayerID string    `json:"player_id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// JoinRequest é um pedido pendente de entrada.
type JoinRequest struct {
	ID        string    `json:"id"`
	PlayerID  string    `json:"player_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// MyAlliance é a aliança do jogador + seu papel + roster (+ pedidos se for oficial+).
type MyAlliance struct {
	Alliance Alliance         `json:"alliance"`
	MyRole   string           `json:"my_role"`
	Members  []AllianceMember `json:"members"`
	Requests []JoinRequest    `json:"requests"`
}

func roleRank(role string) int {
	switch role {
	case "owner":
		return 3
	case "leader":
		return 2
	case "officer":
		return 1
	default:
		return 0
	}
}

// CreateAlliance funda uma aliança (custa premium). O criador vira owner.
func (s *Service) CreateAlliance(ctx context.Context, accountID, name, tag string, now time.Time) (MyAlliance, error) {
	name = strings.TrimSpace(name)
	tag = strings.ToUpper(strings.TrimSpace(tag))
	if len(name) < config.AllianceNameMin || len(name) > config.AllianceNameMax || len(tag) < config.AllianceTagMin || len(tag) > config.AllianceTagMax {
		return MyAlliance{}, ErrBadAllianceName
	}
	accUUID, err := db.ParseUUID(accountID)
	if err != nil {
		return MyAlliance{}, err
	}
	worldUUID, _ := db.ParseUUID(config.DefaultWorldID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MyAlliance{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	player, err := q.GetPlayerByAccountAndWorld(ctx, db.GetPlayerByAccountAndWorldParams{WorldID: worldUUID, AccountID: accUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return MyAlliance{}, ErrNoPlayerYet
	} else if err != nil {
		return MyAlliance{}, err
	}
	if _, err := q.GetMembershipByPlayer(ctx, player.ID); err == nil {
		return MyAlliance{}, ErrAlreadyInAlliance
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return MyAlliance{}, err
	}
	// Cobra a moeda premium (atômico). 0 linhas → sem saldo.
	n, err := q.SpendAccountPremium(ctx, db.SpendAccountPremiumParams{ID: accUUID, Premium: config.AllianceCreateCost})
	if err != nil {
		return MyAlliance{}, err
	}
	if n == 0 {
		return MyAlliance{}, ErrInsufficientPremium
	}
	a, err := q.InsertAlliance(ctx, db.InsertAllianceParams{WorldID: worldUUID, Name: name, Tag: tag, OwnerPlayerID: player.ID, EntryMode: "approval"})
	if err != nil {
		return MyAlliance{}, allianceUniqueErr(err)
	}
	if err := q.InsertAllianceMember(ctx, db.InsertAllianceMemberParams{AllianceID: a.ID, PlayerID: player.ID, Role: "owner"}); err != nil {
		return MyAlliance{}, err
	}
	_ = q.DeleteJoinRequestsByPlayer(ctx, player.ID) // pendências antigas deste jogador caem
	if err := tx.Commit(ctx); err != nil {
		return MyAlliance{}, fmt.Errorf("commit: %w", err)
	}
	return s.MyAlliance(ctx, accountID)
}

// ListAlliances lista as alianças do mundo (para navegar/entrar).
func (s *Service) ListAlliances(ctx context.Context) ([]Alliance, error) {
	worldUUID, _ := db.ParseUUID(config.DefaultWorldID)
	rows, err := s.q.ListAlliances(ctx, worldUUID)
	if err != nil {
		return nil, err
	}
	out := make([]Alliance, 0, len(rows))
	for _, r := range rows {
		out = append(out, Alliance{ID: db.UUIDString(r.ID), Name: r.Name, Tag: r.Tag, EntryMode: r.EntryMode, MemberCap: int(r.MemberCap), Members: int(r.Members)})
	}
	return out, nil
}

// MyAlliance devolve a aliança do jogador (ou erro se não está em nenhuma).
func (s *Service) MyAlliance(ctx context.Context, accountID string) (MyAlliance, error) {
	player, err := s.playerForAccount(ctx, accountID)
	if err != nil {
		return MyAlliance{}, err
	}
	mem, err := s.q.GetMembershipByPlayer(ctx, player.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MyAlliance{}, ErrNotInAlliance
	} else if err != nil {
		return MyAlliance{}, err
	}
	a, err := s.q.GetAlliance(ctx, mem.AllianceID)
	if err != nil {
		return MyAlliance{}, err
	}
	memberRows, err := s.q.ListAllianceMembers(ctx, a.ID)
	if err != nil {
		return MyAlliance{}, err
	}
	out := MyAlliance{
		Alliance: Alliance{ID: db.UUIDString(a.ID), Name: a.Name, Tag: a.Tag, EntryMode: a.EntryMode, MemberCap: int(a.MemberCap), Members: len(memberRows)},
		MyRole:   mem.Role,
		Members:  make([]AllianceMember, 0, len(memberRows)),
		Requests: []JoinRequest{},
	}
	for _, m := range memberRows {
		out.Members = append(out.Members, AllianceMember{PlayerID: db.UUIDString(m.PlayerID), Username: m.Username, Role: m.Role, JoinedAt: m.JoinedAt})
	}
	if roleRank(mem.Role) >= roleRank("officer") {
		reqs, err := s.q.ListJoinRequests(ctx, a.ID)
		if err != nil {
			return MyAlliance{}, err
		}
		for _, r := range reqs {
			out.Requests = append(out.Requests, JoinRequest{ID: db.UUIDString(r.ID), PlayerID: db.UUIDString(r.PlayerID), Username: r.Username, CreatedAt: r.CreatedAt})
		}
	}
	return out, nil
}

// JoinOrRequest: entra direto (aberta) ou cria pedido (aprovação).
func (s *Service) JoinOrRequest(ctx context.Context, accountID, allianceID string, now time.Time) (string, error) {
	aid, err := db.ParseUUID(allianceID)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	player, err := s.playerForAccountTx(ctx, q, accountID)
	if err != nil {
		return "", err
	}
	if _, err := q.GetMembershipByPlayer(ctx, player.ID); err == nil {
		return "", ErrAlreadyInAlliance
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	a, err := q.GetAllianceForUpdate(ctx, aid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAllianceNotFound
	} else if err != nil {
		return "", err
	}
	if a.EntryMode == "open" {
		cnt, err := q.CountAllianceMembers(ctx, aid)
		if err != nil {
			return "", err
		}
		if int(cnt) >= int(a.MemberCap) {
			return "", ErrAllianceFull
		}
		if err := q.InsertAllianceMember(ctx, db.InsertAllianceMemberParams{AllianceID: aid, PlayerID: player.ID, Role: "member"}); err != nil {
			return "", err
		}
		_ = q.DeleteJoinRequestsByPlayer(ctx, player.ID)
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return "joined", nil
	}
	// aprovação → cria pedido
	if err := q.InsertJoinRequest(ctx, db.InsertJoinRequestParams{AllianceID: aid, PlayerID: player.ID}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return "requested", nil
}

// ApproveRequest (oficial+) aceita um pedido (vira membro). RejectRequest recusa.
func (s *Service) ApproveRequest(ctx context.Context, accountID, requestID string, approve bool, now time.Time) error {
	rid, err := db.ParseUUID(requestID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	req, err := q.GetJoinRequest(ctx, rid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoJoinRequest
	} else if err != nil {
		return err
	}
	actor, err := s.requireRole(ctx, q, accountID, req.AllianceID, roleRank("officer"))
	if err != nil {
		return err
	}
	_ = actor
	if approve {
		a, err := q.GetAllianceForUpdate(ctx, req.AllianceID)
		if err != nil {
			return err
		}
		cnt, err := q.CountAllianceMembers(ctx, req.AllianceID)
		if err != nil {
			return err
		}
		if int(cnt) >= int(a.MemberCap) {
			return ErrAllianceFull
		}
		// Só adiciona se o solicitante ainda não entrou em outra aliança nesse meio-tempo.
		if _, err := q.GetMembershipByPlayer(ctx, req.PlayerID); err == nil {
			_ = q.DeleteJoinRequest(ctx, rid)
			return tx.Commit(ctx)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := q.InsertAllianceMember(ctx, db.InsertAllianceMemberParams{AllianceID: req.AllianceID, PlayerID: req.PlayerID, Role: "member"}); err != nil {
			return err
		}
		_ = q.DeleteJoinRequestsByPlayer(ctx, req.PlayerID)
	} else {
		_ = q.DeleteJoinRequest(ctx, rid)
	}
	return tx.Commit(ctx)
}

// Leave: o jogador sai da aliança (o dono precisa dissolver/transferir primeiro).
func (s *Service) Leave(ctx context.Context, accountID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	player, err := s.playerForAccountTx(ctx, q, accountID)
	if err != nil {
		return err
	}
	mem, err := q.GetMembershipByPlayer(ctx, player.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotInAlliance
	} else if err != nil {
		return err
	}
	if mem.Role == "owner" {
		return ErrAllianceForbidden // dono precisa dissolver (ou transferir, futuro)
	}
	if err := q.DeleteAllianceMember(ctx, db.DeleteAllianceMemberParams{AllianceID: mem.AllianceID, PlayerID: player.ID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Kick (oficial+) expulsa um membro de papel INFERIOR.
func (s *Service) Kick(ctx context.Context, accountID, targetPlayerID string) error {
	tpid, err := db.ParseUUID(targetPlayerID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	actor, err := s.membershipForAccount(ctx, q, accountID)
	if err != nil {
		return err
	}
	if roleRank(actor.Role) < roleRank("officer") {
		return ErrAllianceForbidden
	}
	target, err := q.GetAllianceMemberForUpdate(ctx, db.GetAllianceMemberForUpdateParams{AllianceID: actor.AllianceID, PlayerID: tpid})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAllianceForbidden
	} else if err != nil {
		return err
	}
	if roleRank(target.Role) >= roleRank(actor.Role) {
		return ErrAllianceForbidden // só expulsa quem é de papel inferior
	}
	if err := q.DeleteAllianceMember(ctx, db.DeleteAllianceMemberParams{AllianceID: actor.AllianceID, PlayerID: tpid}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetEntryMode (líder+) alterna entre 'open' e 'approval'.
func (s *Service) SetEntryMode(ctx context.Context, accountID, mode string) error {
	if mode != "open" && mode != "approval" {
		return ErrBadAllianceName
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	actor, err := s.membershipForAccount(ctx, q, accountID)
	if err != nil {
		return err
	}
	if roleRank(actor.Role) < roleRank("leader") {
		return ErrAllianceForbidden
	}
	if err := q.UpdateAllianceEntryMode(ctx, db.UpdateAllianceEntryModeParams{ID: actor.AllianceID, EntryMode: mode}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetMemberRole (dono define qualquer papel abaixo de owner incl. leader; líder define até officer).
func (s *Service) SetMemberRole(ctx context.Context, accountID, targetPlayerID, newRole string) error {
	if newRole != "member" && newRole != "officer" && newRole != "leader" {
		return ErrBadAllianceName
	}
	tpid, err := db.ParseUUID(targetPlayerID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	actor, err := s.membershipForAccount(ctx, q, accountID)
	if err != nil {
		return err
	}
	// owner pode definir member/officer/leader; leader pode definir member/officer.
	maxAssignable := roleRank(actor.Role) - 1
	if actor.Role != "owner" && roleRank(actor.Role) < roleRank("leader") {
		return ErrAllianceForbidden
	}
	if roleRank(newRole) > maxAssignable {
		return ErrAllianceForbidden
	}
	target, err := q.GetAllianceMemberForUpdate(ctx, db.GetAllianceMemberForUpdateParams{AllianceID: actor.AllianceID, PlayerID: tpid})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAllianceForbidden
	} else if err != nil {
		return err
	}
	if target.Role == "owner" || roleRank(target.Role) >= roleRank(actor.Role) {
		return ErrAllianceForbidden
	}
	if err := q.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{AllianceID: actor.AllianceID, PlayerID: tpid, Role: newRole}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Disband (só dono) dissolve a aliança (cascade nos membros/pedidos).
func (s *Service) Disband(ctx context.Context, accountID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	actor, err := s.membershipForAccount(ctx, q, accountID)
	if err != nil {
		return err
	}
	if actor.Role != "owner" {
		return ErrAllianceForbidden
	}
	if err := q.DeleteAlliance(ctx, actor.AllianceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- helpers ---

func (s *Service) playerForAccount(ctx context.Context, accountID string) (db.Player, error) {
	accUUID, err := db.ParseUUID(accountID)
	if err != nil {
		return db.Player{}, err
	}
	worldUUID, _ := db.ParseUUID(config.DefaultWorldID)
	p, err := s.q.GetPlayerByAccountAndWorld(ctx, db.GetPlayerByAccountAndWorldParams{WorldID: worldUUID, AccountID: accUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Player{}, ErrNoPlayerYet
	}
	return p, err
}

func (s *Service) playerForAccountTx(ctx context.Context, q *db.Queries, accountID string) (db.Player, error) {
	accUUID, err := db.ParseUUID(accountID)
	if err != nil {
		return db.Player{}, err
	}
	worldUUID, _ := db.ParseUUID(config.DefaultWorldID)
	p, err := q.GetPlayerByAccountAndWorld(ctx, db.GetPlayerByAccountAndWorldParams{WorldID: worldUUID, AccountID: accUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Player{}, ErrNoPlayerYet
	}
	return p, err
}

func (s *Service) membershipForAccount(ctx context.Context, q *db.Queries, accountID string) (db.AllianceMember, error) {
	player, err := s.playerForAccountTx(ctx, q, accountID)
	if err != nil {
		return db.AllianceMember{}, err
	}
	mem, err := q.GetMembershipByPlayer(ctx, player.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AllianceMember{}, ErrNotInAlliance
	}
	return mem, err
}

// requireRole valida que o ator é membro da aliança e tem papel >= minRank.
func (s *Service) requireRole(ctx context.Context, q *db.Queries, accountID string, allianceID pgtype.UUID, minRank int) (db.AllianceMember, error) {
	actor, err := s.membershipForAccount(ctx, q, accountID)
	if err != nil {
		return db.AllianceMember{}, err
	}
	if !sameUUID(actor.AllianceID, allianceID) || roleRank(actor.Role) < minRank {
		return db.AllianceMember{}, ErrAllianceForbidden
	}
	return actor, nil
}

// allianceUniqueErr traduz violação de UNIQUE de nome/tag em erro de negócio.
func allianceUniqueErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if strings.Contains(pgErr.ConstraintName, "tag") {
			return ErrAllianceTagTaken
		}
		return ErrAllianceNameTaken
	}
	return err
}

// sameAlliance: true se os dois jogadores estão na MESMA aliança (ambos com membership e mesmo id).
func sameAlliance(ctx context.Context, q *db.Queries, p1, p2 pgtype.UUID) (bool, error) {
	m1, err := q.GetMembershipByPlayer(ctx, p1)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	m2, err := q.GetMembershipByPlayer(ctx, p2)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return sameUUID(m1.AllianceID, m2.AllianceID), nil
}

// notifyAlliance insere um relatório (payload) para todos os OUTROS membros da aliança do jogador
// `victim` (se ele tiver aliança). Usado p/ avisar a aliança quando um membro é atacado/espionado.
func notifyAlliance(ctx context.Context, q *db.Queries, worldID, victim pgtype.UUID, reportType string, payload []byte) error {
	m, err := q.GetMembershipByPlayer(ctx, victim)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // vítima sem aliança → ninguém a avisar
	} else if err != nil {
		return err
	}
	members, err := q.ListAllianceMembers(ctx, m.AllianceID)
	if err != nil {
		return err
	}
	for _, mem := range members {
		if sameUUID(mem.PlayerID, victim) {
			continue // a própria vítima já recebe o alerta direto
		}
		if _, err := q.InsertReport(ctx, db.InsertReportParams{WorldID: worldID, PlayerID: mem.PlayerID, Type: reportType, Payload: payload}); err != nil {
			return err
		}
	}
	return nil
}

// TransferOwnership: o DONO passa a propriedade da aliança para outro MEMBRO; o antigo dono vira líder.
func (s *Service) TransferOwnership(ctx context.Context, accountID, targetPlayerID string) error {
	targetUUID, err := db.ParseUUID(targetPlayerID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	caller, err := s.membershipForAccount(ctx, q, accountID)
	if err != nil {
		return err
	}
	if caller.Role != "owner" {
		return ErrAllianceForbidden
	}
	if sameUUID(caller.PlayerID, targetUUID) {
		return ErrAllianceForbidden // já é o dono
	}
	target, err := q.GetAllianceMemberForUpdate(ctx, db.GetAllianceMemberForUpdateParams{AllianceID: caller.AllianceID, PlayerID: targetUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAllianceForbidden // alvo não é membro desta aliança
	} else if err != nil {
		return err
	}
	if err := q.UpdateAllianceOwner(ctx, db.UpdateAllianceOwnerParams{ID: caller.AllianceID, OwnerPlayerID: target.PlayerID}); err != nil {
		return err
	}
	if err := q.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{AllianceID: caller.AllianceID, PlayerID: target.PlayerID, Role: "owner"}); err != nil {
		return err
	}
	if err := q.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{AllianceID: caller.AllianceID, PlayerID: caller.PlayerID, Role: "leader"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
