package aic

// transitions autorisées de la machine d'états des missions.
var allowed = map[string][]string{
	StateOpen:             {StateRunning, StateBlocked},
	StateRunning:          {StateReview, StateBlocked},
	StateReview:           {StateAwaitingApproval, StateCorrection, StateBlocked},
	StateCorrection:       {StateRunning, StateBlocked},
	StateAwaitingApproval: {StateAccepted, StateCorrection, StateBlocked},
	StateAccepted:         {},
	StateBlocked:          {},
}

// CanTransition vérifie qu'un passage d'état est autorisé.
func CanTransition(from, to string) bool {
	for _, t := range allowed[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Terminal indique si un état est final pour la mission.
func Terminal(s string) bool {
	return s == StateAccepted || s == StateBlocked
}
