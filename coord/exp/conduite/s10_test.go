// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package conduite

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestS10Gouvernance execute le scenario S10 (gouvernance COORD) et
// verifie les criteres pre-declares du plan v2 accepte :
//
//  1. le Pilot reel et le serveur MCP #37 sont mobilises INCHANGE (aucune
//     modification du moteur) ;
//  2. la mission experimentale derivee de COORD est visible en lecture ;
//  3. aucun ecart n est releve dans les tests negatifs anti-mandat ;
//  4. la gouvernance est identique par mode (verdict conserve) ;
//  5. aucune credential COORD n atteint les agents, aucun canal de
//     mutation n existe ni cote HTTP (401/403), ni cote MCP (2 outils).
func TestS10Gouvernance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	res, err := LigneS10(ctx)
	if err != nil {
		t.Fatalf("S10 : %v", err)
	}
	if res.Ligne.Verdict != "conservée" && !strings.Contains(res.Ligne.Verdict, "conserv") {
		t.Errorf("S10 : verdict %s, attendu conservee", res.Ligne.Verdict)
	}
	if res.Ligne.A.EtatFinal != res.Ligne.B.EtatFinal {
		t.Errorf("S10 : etats finaux differents A=%s B=%s", res.Ligne.A.EtatFinal, res.Ligne.B.EtatFinal)
	}
	for _, n := range res.Negatifs {
		if strings.Contains(n, "defaut") || strings.Contains(n, "FUITE") ||
			strings.Contains(n, "inattendu") || strings.Contains(n, "ACCEPTEE") {
			t.Errorf("S10 : ecart de gouvernance : %s", n)
		}
	}
	if len(res.Invariants) < 8 {
		t.Errorf("S10 : preuves insuffisantes (%d invariants)", len(res.Invariants))
	}
	if !strings.Contains(MarkdownS10(res), "[REFUS ATTENDU]") {
		t.Error("S10 : le rapport ne rend pas les refus attendus")
	}
}
