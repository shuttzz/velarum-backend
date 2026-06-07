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

// ScoutSpeedBonusPct: bônus de VELOCIDADE de marcha do batedor pelo nível da Toca dos Batedores.
// Pequeno DE PROPÓSITO (pacing) — nível 1 = 0% (base); cada nível acima soma 5%. O gate do nº de
// níveis por era (Era 1 = 3) e o ganho de +batedor/marcha vêm no épico dos batedores (a definir).
func ScoutSpeedBonusPct(scoutHouseLevel int) int {
	if scoutHouseLevel <= 1 {
		return 0
	}
	return (scoutHouseLevel - 1) * 5
}

// ScoutMarchSeconds aplica o bônus de velocidade da Toca ao tempo de marcha do batedor.
func ScoutMarchSeconds(scoutHouseLevel, ax, ay, bx, by int) int {
	base := MarchSecondsBetween(ax, ay, bx, by)
	bonus := ScoutSpeedBonusPct(scoutHouseLevel)
	if bonus == 0 {
		return base
	}
	return int(float64(base) * 100.0 / float64(100+bonus))
}
