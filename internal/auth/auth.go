// Package auth implementa contas globais (registro/login) e sessões server-side.
//
// Modelo: conta global (email/username únicos) + sessão com token OPACO. O cookie httpOnly
// carrega só o token; no banco guardamos o seu sha256 (token_hash) — permite logout e
// revogação. Senha com bcrypt. Cf. internal/db (sqlc) para as queries.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"backend/internal/db"
)

const (
	// SessionCookieName é o nome do cookie httpOnly que carrega o token opaco de sessão.
	SessionCookieName = "vela_session"
	// SessionTTL é a validade de uma sessão (jogo de estratégia tem sessões longas).
	SessionTTL = 14 * 24 * time.Hour

	minPasswordLen = 8
	minUsernameLen = 3
	maxUsernameLen = 32
)

var (
	ErrInvalidInput       = errors.New("dados inválidos")
	ErrWeakPassword       = errors.New("senha muito curta (mínimo 8 caracteres)")
	ErrEmailTaken         = errors.New("email já cadastrado")
	ErrUsernameTaken      = errors.New("nome de usuário já em uso")
	ErrInvalidCredentials = errors.New("email ou senha inválidos")
	ErrNoSession          = errors.New("sessão ausente ou inválida")
)

// Account é a visão pública de uma conta (nunca expõe o hash de senha).
type Account struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Premium  int    `json:"premium"` // moeda premium (saldo da conta)
}

// Service expõe os casos de uso de autenticação.
type Service struct {
	pool         *pgxpool.Pool
	q            *db.Queries
	cookieSecure bool
}

// NewService cria o serviço. cookieSecure controla o atributo Secure do cookie
// (true em produção/HTTPS; false em dev sob http).
func NewService(pool *pgxpool.Pool, cookieSecure bool) *Service {
	return &Service{pool: pool, q: db.New(pool), cookieSecure: cookieSecure}
}

// Register cria uma conta global nova. Erros mapeados: ErrInvalidInput, ErrWeakPassword,
// ErrEmailTaken, ErrUsernameTaken.
func (s *Service) Register(ctx context.Context, username, email, password string) (Account, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if !validUsername(username) || !validEmail(email) {
		return Account{}, ErrInvalidInput
	}
	if len(password) < minPasswordLen {
		return Account{}, ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, fmt.Errorf("hash senha: %w", err)
	}
	row, err := s.q.CreateAccount(ctx, db.CreateAccountParams{
		Username: username, Email: email, PasswordHash: string(hash),
	})
	if err != nil {
		if name, ok := uniqueViolation(err); ok {
			switch name {
			case "uq_accounts_email":
				return Account{}, ErrEmailTaken
			case "uq_accounts_username":
				return Account{}, ErrUsernameTaken
			}
		}
		return Account{}, fmt.Errorf("criar conta: %w", err)
	}
	return toAccount(row), nil
}

// Login valida email+senha e cria uma sessão, retornando o token OPACO (vai no cookie),
// o instante de expiração e a conta. Falha → ErrInvalidCredentials (sem distinguir o motivo).
func (s *Service) Login(ctx context.Context, email, password string, now time.Time) (string, time.Time, Account, error) {
	row, err := s.q.GetAccountByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", time.Time{}, Account{}, ErrInvalidCredentials
		}
		return "", time.Time{}, Account{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)) != nil {
		return "", time.Time{}, Account{}, ErrInvalidCredentials
	}
	token, err := newToken()
	if err != nil {
		return "", time.Time{}, Account{}, err
	}
	expires := now.Add(SessionTTL)
	if _, err := s.q.CreateSession(ctx, db.CreateSessionParams{
		AccountID: row.ID, TokenHash: hashToken(token), ExpiresAt: expires,
	}); err != nil {
		return "", time.Time{}, Account{}, fmt.Errorf("criar sessão: %w", err)
	}
	_ = s.q.TouchAccountLogin(ctx, db.TouchAccountLoginParams{
		ID: row.ID, LastLoginAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	return token, expires, toAccount(row), nil
}

// Authenticate resolve o token opaco do cookie para o ID da conta. Sessão ausente/expirada
// → ErrNoSession (e remove a sessão expirada).
func (s *Service) Authenticate(ctx context.Context, token string, now time.Time) (string, error) {
	if token == "" {
		return "", ErrNoSession
	}
	sess, err := s.q.GetSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNoSession
		}
		return "", err
	}
	if !now.Before(sess.ExpiresAt) {
		_ = s.q.DeleteSession(ctx, hashToken(token))
		return "", ErrNoSession
	}
	return db.UUIDString(sess.AccountID), nil
}

// AccountByID busca a visão pública de uma conta pelo ID.
func (s *Service) AccountByID(ctx context.Context, id string) (Account, error) {
	uid, err := db.ParseUUID(id)
	if err != nil {
		return Account{}, err
	}
	row, err := s.q.GetAccountByID(ctx, uid)
	if err != nil {
		return Account{}, err
	}
	return toAccount(row), nil
}

// Logout invalida a sessão do token informado.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, hashToken(token))
}

// newToken gera um token opaco de 256 bits (base64url, sem padding).
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken devolve o sha256 hex do token — é o que persistimos (nunca o token cru).
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func toAccount(a db.Account) Account {
	return Account{ID: db.UUIDString(a.ID), Username: a.Username, Email: a.Email, Premium: int(a.Premium)}
}

func validUsername(u string) bool {
	n := len(u)
	return n >= minUsernameLen && n <= maxUsernameLen && !strings.ContainsAny(u, " @")
}

func validEmail(e string) bool {
	at := strings.IndexByte(e, '@')
	if at <= 0 || strings.ContainsAny(e, " ") {
		return false
	}
	domain := e[at+1:]
	dot := strings.LastIndexByte(domain, '.')
	return dot > 0 && dot < len(domain)-1
}

// uniqueViolation extrai o nome da constraint de um erro de violação UNIQUE do Postgres.
func uniqueViolation(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}
