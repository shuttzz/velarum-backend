package db

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// UUIDString formata um pgtype.UUID na forma canônica (8-4-4-4-12). Vazio se inválido.
func UUIDString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ParseUUID converte uma string em pgtype.UUID.
func ParseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("uuid inválido %q: %w", s, err)
	}
	return u, nil
}
