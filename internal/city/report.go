package city

import (
	"context"
	"encoding/json"
	"time"

	"backend/internal/db"
	"backend/internal/domain/resource"
)

const reportTypeBattle = "battle"

// Report é a visão de domínio de um relatório (caixa de entrada do jogador). O payload é
// JSON cru tipado por `Type` (o frontend interpreta conforme o tipo).
type Report struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Read      bool            `json:"read"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

// battleReport é o payload de um relatório de batalha (resultado de uma marcha PvE).
type battleReport struct {
	ProvinceID      string           `json:"province_id"`
	ProvinceNameKey string           `json:"province_name_key"`
	AttackerWon     bool             `json:"attacker_won"`
	Sent            map[string]int   `json:"sent"`
	Losses          map[string]int   `json:"losses"`
	Survivors       map[string]int   `json:"survivors"`
	Reward          resource.Amounts `json:"reward"`
}

// ListReports devolve os relatórios do jogador (mais novos primeiro).
func (s *Service) ListReports(ctx context.Context, cityID string) ([]Report, error) {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return nil, err
	}
	cityRow, err := s.q.GetCity(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListPlayerReports(ctx, cityRow.PlayerID)
	if err != nil {
		return nil, err
	}
	out := make([]Report, 0, len(rows))
	for _, r := range rows {
		out = append(out, Report{
			ID: db.UUIDString(r.ID), Type: r.Type, Read: r.Read, CreatedAt: r.CreatedAt, Payload: json.RawMessage(r.Payload),
		})
	}
	return out, nil
}

// MarkReportsRead marca todos os relatórios do jogador como lidos.
func (s *Service) MarkReportsRead(ctx context.Context, cityID string) error {
	id, err := db.ParseUUID(cityID)
	if err != nil {
		return err
	}
	cityRow, err := s.q.GetCity(ctx, id)
	if err != nil {
		return err
	}
	return s.q.MarkAllReportsRead(ctx, cityRow.PlayerID)
}
