package config

import "backend/internal/domain/resource"

// Espionagem (SW3). Batedor = unidade NÃO-combatente, treinada na Toca dos Batedores, enviada em
// missões de scout (lane separada) que revelam intel do alvo. Valores dev-scale (tunáveis).
const (
	// ScoutHouseKey é o edifício que habilita a espionagem (treinar/enviar batedores).
	ScoutHouseKey = "toca_dos_batedores"
	// ScoutTrainSeconds é o tempo de treino POR batedor.
	ScoutTrainSeconds = 20
)

// ScoutCost é o custo POR batedor treinado.
var ScoutCost = resource.Amounts{Matter: 30, Energy: 20}
