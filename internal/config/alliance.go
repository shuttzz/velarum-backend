package config

// Alianças (SW3). Cf. design-aliancas. Valores tunáveis.
const (
	// AllianceCreateCost é o custo em MOEDA PREMIUM para fundar uma aliança.
	AllianceCreateCost = 300
	// AllianceDefaultCap é o teto inicial de membros (cresce por pesquisa no futuro).
	AllianceDefaultCap = 30

	AllianceNameMin = 3
	AllianceNameMax = 24
	AllianceTagMin  = 2
	AllianceTagMax  = 5
)
