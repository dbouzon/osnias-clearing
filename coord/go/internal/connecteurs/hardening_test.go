// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"coordengine/internal/adaptateurs"

	"coordengine/internal/missions"
)

// pointReelBlanc : connecteur white-box déclaré RÉEL pointant vers un serveur
// factice loopback (AUCUNE clé, AUCUN réseau sortant, 0 €). Réservé aux
// contrôles transversaux de la couche commune (hardening M-COORD-HARDENING-01).
func pointReelBlanc(endpoint string, jetonsEntreeMax int) *HTTPEndPoint {
	return &HTTPEndPoint{
		id: "mistral", endpoint: endpoint, modele: "test-modele",
		client:    &http.Client{Timeout: 5 * time.Second},
		tailleMax: 65536, retraits: 0, simuleLoc: false,
		jetonsEntreeMax: jetonsEntreeMax,
		// Voûte monétaire armée (tarifs factices, centimes entiers) : l'appel
		// atteint le réseau (échec transport voulu), il n'est PAS refusé en
		// TX1 par TARIF_INCONNU.
		tarifEntreeMillion: 100, tarifSortieMillion: 200, tarifRequete: 1,
		jetonsSortieMax: 10000,
	}
}

// REPRO P1 (reproduction AVANT correction, garde-fou permanent) : la borne
// d'émission divise la charge par 4 (« 4 octets par jeton »). En UTF-8
// multi-octets (accents/CJK), le ratio réel descend vers 2 voire 1 octet par
// jeton : une charge de jetonsEntreeMax*3 octets CJK représente PLUS que la
// borne de jetons déclarée, et le code incriminé l'ÉMET quand même. Sur le
// code corrigé (diviseur conservateur 2, estimation MAJORÉE, PAS une garantie
// fournisseur), la même charge est refusée AVANT tout octet émis.
func TestReproP1BorneOctetsSousEstimeJetons(t *testing.T) {
	appels := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels++
		w.Write(reponseChat("LIVRABLE FACTICE", "test-modele", 1500))
	}))
	t.Cleanup(s.Close)
	h := pointReelBlanc(s.URL, 500)    // borne déclarée : 500 jetons d'entrée
	charge := strings.Repeat("世", 500) // 1500 octets UTF-8 ≈ 500..1000 jetons réels
	_, err := h.ExecuterContexte(context.Background(), "executer", charge, "cle-repro-p1")
	if err == nil || !strings.Contains(err.Error(), "CHARGE_HORS_BORNE") {
		t.Fatalf("REPRO P1 : une charge de %d octets CJK au-delà de la borne "+
			"de 500 jetons doit être refusée AVANT émission (CHARGE_HORS_BORNE) ; "+
			"ici l'appel est parti (%d requêtes reçues) : %+v", len(charge), appels, err)
	}
	if appels != 0 {
		t.Fatalf("REPRO P1 : AUCUN octet ne doit être émis après refus ; %d requêtes reçues", appels)
	}
}

// TestP1ChargeDansBorneAdmise : non-régression — une charge latine dont
// l'estimation conservatrice reste sous la borne est admise normalement.
func TestP1ChargeDansBorneAdmise(t *testing.T) {
	appels := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appels++
		w.Write(reponseChat("LIVRABLE FACTICE", "test-modele", 1500))
	}))
	t.Cleanup(s.Close)
	h := pointReelBlanc(s.URL, 20000)
	charge := strings.Repeat("a", 800) // corps (gabarit+charge) très sous la borne 40 000 octets
	r, err := h.ExecuterContexte(context.Background(), "executer", charge, "cle-p1-ok")
	if err != nil || appels != 1 {
		t.Fatalf("P1 : charge légitime admise attendue ; err=%+v appels=%d", err, appels)
	}
	if r.Jetons != 1500 {
		t.Fatalf("P1 : jetons factices attendus 1500, obtenu %d", r.Jetons)
	}
}

// TestP3UsageAbsentVsNul : le refus JETONS_INCONNUS (code public inchangé)
// distingue désormais diagnostiquement l'usage ABSENT du corps de l'usage
// PRÉSENT mais nul/invalide. Aucune des deux familles n'est admise
// (fail closed inchangé), aucun secret ne traverse les diagnostics.
func TestP3UsageAbsentVsNul(t *testing.T) {
	avecUsage := func(u map[string]any) []byte {
		b, _ := json.Marshal(map[string]any{
			"model":   "test-modele",
			"choices": []any{map[string]any{"message": map[string]any{"content": "SORTIE"}}},
			"usage":   u,
		})
		return b
	}
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Idempotency-Key") {
		case "cle-sans-usage":
			b, _ := json.Marshal(map[string]any{
				"model":   "test-modele",
				"choices": []any{map[string]any{"message": map[string]any{"content": "SORTIE"}}},
			})
			w.Write(b)
		case "cle-usage-nul":
			w.Write(avecUsage(map[string]any{"total_tokens": 0}))
		case "cle-usage-invalide":
			w.Write(avecUsage(map[string]any{"total_tokens": -7}))
		default:
			w.Write(reponseChat("LIVRABLE", "test-modele", 1500))
		}
	})
	h := pointReelBlanc(s.URL, 0) // borne émission neutralisée : on teste l'usage
	cas := []struct{ cle, attendu string }{
		{"cle-sans-usage", "usage absent"},
		{"cle-usage-nul", "nul/invalide"},
		{"cle-usage-invalide", "nul/invalide"},
	}
	for _, c := range cas {
		_, err := h.ExecuterContexte(context.Background(), "executer", "charge", c.cle)
		if err == nil || !strings.Contains(err.Error(), "JETONS_INCONNUS") {
			t.Fatalf("P3 (%s) : refus JETONS_INCONNUS attendu, obtenu %+v", c.cle, err)
		}
		if !strings.Contains(err.Error(), c.attendu) {
			t.Fatalf("P3 (%s) : diagnostic « %s » attendu dans : %+v", c.cle, c.attendu, err)
		}
	}
}

// TestP2ExpurgationPreserveEmpreintes : PREUVE — les empreintes SHA-256
// (hex 64) et SHA Git (hex 40), sans préfixe sk-/AIza, traversent expurger
// INTACTES ; les séquences sk- et AIza sont remplacées. Une clé partielle
// ou trop courte (<8) n'est pas remplacée : elle n'est pas la clé (comporte-
// ment documenté, aucune divulgation nouvelle).
func TestP2ExpurgationPreserveEmpreintes(t *testing.T) {
	b := baseHTTP{cle: "sk-cleactive1234567890"}
	sha256 := "59bd797dee176010c0ec05d5f563de68655a744f34ff8373aa5fa6bd15a4748b"
	gitSha := "406bd6edc5b8b61eafdf5a4ac890799239b01d92"
	e := b.expurger("sha256=" + sha256 + " git=" + gitSha +
		" cle1=sk-facticeabcdef12 cle2=AIzaSyD-1234567890abcdefghijklmnopqrsXYZAB" +
		" fragment=sk-cleac")
	for _, want := range []string{sha256, gitSha} {
		if !strings.Contains(e, want) {
			t.Fatalf("P2 : l'empreinte %s doit rester intacte ; obtenu %s", want, e)
		}
	}
	if strings.Contains(e, "sk-facticeabcdef12") || strings.Contains(e, "AIzaSyD-1234567890abcdefghijklmnopqrsXYZAB") {
		t.Fatalf("P2 : les séquences de clé doivent être expurgées ; obtenu %s", e)
	}
	if !strings.Contains(e, "sk-cleac") {
		t.Fatal("P2 : une clé partielle/tronquée (documentée, non-authentifiante) n'est pas remplacée")
	}
	b2 := baseHTTP{cle: "sk-cour"} // < 8 caractères : pas de remplacement littéral
	if got := b2.expurger("x sk-cour y"); !strings.Contains(got, "sk-cour") {
		t.Fatalf("P2 : comportement clé trop courte documenté ; obtenu %s", got)
	}
}

// TestP4AppelEnCoursPersisteAvantReseau : PREUVE — l'appel réel en échec de
// transport a bien persisté son marqueur d'idempotence AVANT le réseau
// (appel_en_cours.json présent, aucune réservation libérée) ; le deuxième
// essai est refusé jusqu'à résolution pilote (SEC-01, déjà verrouillé par
// A15 : ce test ne fait que prouver la persistance durable du marqueur).
func TestP4AppelEnCoursPersisteAvantReseau(t *testing.T) {
	mort := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(499)
	}))
	mort.Close() // serveur fermé : l'appel échoue au transport, fait exprès
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("référentiel : %v", err)
	}
	orch, err := NouvelOrchestrateur(ref, map[string]adaptateurs.Adaptateur{
		"mistral": pointReelBlanc(mort.URL, 10000),
	})
	if err != nil {
		t.Fatalf("orchestrateur : %v", err)
	}
	issue := "harden-p4"
	p := plafondsTest()
	p.Centimes = 100000
	if err := orch.NouvelleMission(issue, "mistral-exec", "openai-recep", rolesTest(), p); err != nil {
		t.Fatalf("création : %v", err)
	}
	if _, err := orch.Executer(context.Background(), issue, "mistral-exec", "mandat"); err == nil {
		t.Fatal("P4 : l'appel doit échouer (serveur fermé : échec transport attendu)")
	}
	if _, err := os.Stat(cheminAppel(ref.Racine(), issue)); err != nil {
		t.Fatalf("P4 : appel_en_cours.json doit persister après l'échec transport : %v", err)
	}
}
