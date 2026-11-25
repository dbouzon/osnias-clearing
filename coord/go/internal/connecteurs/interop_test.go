// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// MATRICE D'INTEROPÉRABILITÉ (M-COORD-RAIL-01, ordre T4) : les QUATRE
// fournisseurs déclarés de COORD (Mistral, OpenAI, Gemini, Anthropic)
// SetBranchent MIS EN ŒUVRE en factices loopback, et TOUS les couples
// ordonnés (exécutant, réceptionnaire) distincts — 4×3 = 12 couples —
// déroulent un cycle complet de la machine normative v1.5 : création
// (affectation EXPLICITE des rôles), exécution, soumission, réception
// ancrée, ACK. Aucune clé, aucun compte, 0 € : la qualification RÉELLE
// des API des fournisseurs N'EST PAS présupposée (recettes réelles
// distinctes, autorisation écrite du pilote). TI-07 (arrêt pilote,
// REPONSE_TARDIVE) est couvert par TestReponseTardiveArretPendantAppel.
package connecteurs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
	"coordengine/internal/spec"
)

// interopFournisseurs : les 4 profils déclarés (IDs de banc, exécutant
// jamais égal au réceptionnaire par construction de la matrice).
var interopFournisseurs = []struct {
	id     string
	modele string
	format string
	profil string
}{
	{"banc-anthropic", "claude-factice-matrix", FormatAnthropic, ""},
	{"banc-gemini", "gemini-factice-matrix", FormatOpenAI, ProfilGemini},
	{"banc-mistral", "mistral-factice-matrix", FormatOpenAI, ""},
	{"banc-openai", "openai-factice-matrix", FormatOpenAI, ""},
}

// interopCharge : corps de requête décodé par les factices (formats
// openai et anthropic partagent le champ messages).
type interopCharge struct {
	Messages []messageAPI `json:"messages"`
}

// interopEmpreinteDeCharge : empreinte 64 hex attendue dans la charge de
// réception (marqueur du gabarit, séquence échappée comme au gabarit).
func interopEmpreinteDeCharge(charge string) string {
	for _, marque := range []string{`empreinte_attendue\":\"`, `empreinte_attendue":"`} {
		i := strings.Index(charge, marque)
		if i < 0 {
			continue
		}
		reste := charge[i+len(marque):]
		if len(reste) < 64 {
			continue
		}
		suite := reste[:64]
		ok := true
		for _, c := range suite {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				ok = false
				break
			}
		}
		if ok {
			return suite
		}
	}
	return ""
}

// interopContenu : réponse de l'agent factice selon la charge reçue
// (réception : verdict JSON STRICT ancré ; sinon livrable simulé).
func interopContenu(t *testing.T, id, charge string) string {
	t.Helper()
	if strings.Contains(charge, "RECEPTION INDEPENDANTE") {
		emp := interopEmpreinteDeCharge(charge)
		if emp == "" {
			t.Fatalf("%s : charge de réception sans empreinte ancrable", id)
		}
		return fmt.Sprintf(`{"verdict":"ACCEPTEE","points":[],"empreinte_attendue":"%s",`+
			`"verdict_id":"V-INTEROP-%s","verdict_url":"https://example.invalid/interop/%s"}`,
			emp, id, id)
	}
	return "LIVRABLE-" + id
}

// interopServeur : factice loopback au format DU FOURNISSEUR déclaré
// (openai-compatible pour mistral/openai/gemini, Messages API pour
// anthropic). Le modèle déclaré est TOUJOURS écho (MODELE_DIVERGENT
// neutre par ailleurs), jetons déclarés pour la voûte.
func interopServeur(t *testing.T, id, modele, format string) *httptest.Server {
	t.Helper()
	appels := 0
	s := nouveauServeurFactice(t, func(w http.ResponseWriter, r *http.Request) {
		appels++
		var c interopCharge
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil || len(c.Messages) == 0 {
			t.Errorf("%s : requête factice non JSON : %v", id, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		charge := c.Messages[len(c.Messages)-1].Content
		contenu := interopContenu(t, id, charge)
		if format == FormatAnthropic {
			w.Write(reponseAnthropicTexte(contenu, modele, "end_turn", 240, 400))
			return
		}
		w.Write(interopReponseChat(contenu, modele, 640))
	})
	t.Cleanup(func() {
		if appels == 0 {
			t.Errorf("%s : le fournisseur factice n'a reçu AUCUN appel", id)
		}
	})
	return s
}

// interopReponseChat : réponse compatible OpenAI AVEC finish_reason EXPLICITE
// ("stop", requis par le profil gemini stop-only ; les profils mistral/
// openai ignorent ce champ supplémentaire).
func interopReponseChat(contenu, modele string, jetons int) []byte {
	b, _ := json.Marshal(map[string]any{
		"model": modele,
		"choices": []any{map[string]any{
			"message":       map[string]any{"content": contenu},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"total_tokens": jetons},
	})
	return b
}

// TestMatriceInteropDouzeCouples : TOUS les couples ordonnés distincts
// des 4 fournisseurs déroulent le cycle complet. Registre à 4 adaptateurs :
// SEULE l'affectation explicite exprime les couples (le tri historique
// en serait incapable — il ne connaît que ses deux premiers triés).
func TestMatriceInteropDouzeCouples(t *testing.T) {
	ctx := context.Background()
	serveurs := map[string]*httptest.Server{}
	cfg := &Config{Mode: ModeSimule, Fournisseurs: []ConfigFournisseur{}}
	for _, f := range interopFournisseurs {
		serveurs[f.id] = interopServeur(t, f.id, f.modele, f.format)
		cfg.Fournisseurs = append(cfg.Fournisseurs, ConfigFournisseur{
			ID: f.id, Endpoint: serveurs[f.id].URL, Modele: f.modele,
			Format: f.format, Profil: f.profil,
			TimeoutMS: 4000, TailleMaxReponse: 65536, RetraitsMax: 1,
			TarifEntreeCentimesParMillion: 100, TarifSortieCentimesParMillion: 300,
			TarifRequeteCentimes:        1,
			JetonsEntreeMaxParTentative: 20000, JetonsSortieMaxParTentative: 20000,
		})
	}
	reg := map[string]adaptateurs.Adaptateur{}
	for _, f := range interopFournisseurs {
		var a adaptateurs.Adaptateur
		var err error
		if f.format == FormatAnthropic {
			a, err = cfg.NouveauAnthropicPoint(f.id)
		} else {
			a, err = cfg.NouveauHTTPEndPoint(f.id)
		}
		if err != nil {
			t.Fatalf("connecteur %s : %v", f.id, err)
		}
		reg[f.id] = a
	}
	ref, err := missions.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orch, err := NouvelOrchestrateur(ref, reg)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string][]string{
		"EXECUTANT":      {"executer", "soumettre", "acker"},
		"RECEPTIONNAIRE": {"recevoir"},
		"PILOTE":         {"cloturer"},
	}
	plaf := budget.Plafonds{Appels: map[string]int{}, Jetons: map[string]int{}}
	for _, f := range interopFournisseurs {
		plaf.Appels[f.id] = 50
		plaf.Jetons[f.id] = 5000000
	}

	couples := 0
	for _, exec := range interopFournisseurs {
		for _, recep := range interopFournisseurs {
			if exec.id == recep.id {
				continue
			}
			couples++
			issue := fmt.Sprintf("banc-interop-%s-%s",
				strings.TrimPrefix(exec.id, "banc-"), strings.TrimPrefix(recep.id, "banc-"))
			if err := orch.NouvelleMissionExplicit(issue, "exec-1", "recep-1",
				exec.id, recep.id, roles, plaf); err != nil {
				t.Fatalf("couple %s : création : %v", issue, err)
			}
			livrable := "LIVRABLE INTEROP " + issue
			if _, err := orch.Executer(ctx, issue, "exec-1", "mandat interop "+issue); err != nil {
				t.Fatalf("couple %s : exécution : %v", issue, err)
			}
			if _, err := orch.Soumettre(ctx, issue, "exec-1", livrable); err != nil {
				t.Fatalf("couple %s : soumission : %v", issue, err)
			}
			v, _, err := orch.Recevoir(ctx, issue, "recep-1", livrable)
			if err != nil {
				t.Fatalf("couple %s : réception : %v", issue, err)
			}
			if v.Verdict != "ACCEPTEE" {
				t.Fatalf("couple %s : ACCEPTEE attendu : %+v", issue, v)
			}
			if err := orch.Acker(issue, "exec-1"); err != nil {
				t.Fatalf("couple %s : ACK : %v", issue, err)
			}
			enr, err := ref.Obtenir(issue)
			if err != nil {
				t.Fatal(err)
			}
			if enr.Mission.State != spec.EtatACKRecu {
				t.Fatalf("couple %s : ACK_RECU attendu : %s", issue, enr.Mission.State)
			}
			// Clôture : JAMAIS sans le pilote (N-14, vérifié au moins une
			// fois sur un couple de la matrice).
			if err := orch.Cloturer(issue); err == nil ||
				!strings.Contains(err.Error(), "CLOTURE_REFUSEE") {
				t.Fatalf("clôture hors pilote interdite : %v", err)
			}
		}
	}
	if couples != 12 {
		t.Fatalf("12 couples ordonnés attendus : %d", couples)
	}
}
