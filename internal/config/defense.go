package config

import "time"

// Defesa de cidade no PvP (saque). Estruturas defensivas escaláveis e RE-SKINNÁVEIS por era — o
// conceito atravessa do medieval ao espacial (Muralha → bastião → escudo de energia → escudo
// planetário; Torre do Vigia → ... → defesa orbital). Aqui só os números/efeito; o nome/arte por era.
const (
	WallKey  = "muralha"
	TowerKey = "torre_do_vigia"
)

// NewbieShieldDuration: proteção de uma conta NOVA contra saque. Cai ao 1º ataque do jogador.
const NewbieShieldDuration = 4 * 24 * time.Hour

// WallHP é o HP de robustez que a Muralha no nível dado soma ao defensor (absorve dano, reduz baixas
// da guarnição; não é destruída no saque comum).
func WallHP(level int) int {
	if level <= 0 {
		return 0
	}
	return 200 + 150*(level-1)
}

// TowerAttack é o ataque defensivo que a Torre do Vigia no nível dado soma ao defensor (a cidade revida).
func TowerAttack(level int) int {
	if level <= 0 {
		return 0
	}
	return 30 + 20*(level-1)
}
