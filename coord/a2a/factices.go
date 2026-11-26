// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Serveurs d'agents A2A FACTICES (qualification sans clé, sans compte,
// 0 € : mission M-COORD-LLM-02, T5). Deux exécuteurs :
//
//   - ExecGeminiFactice : ADAPTATEUR GEMINI SIMULÉ. Traduit A2A vers une
//     réponse "API Gemini" déterministe : la charge COORD (critique de plan
//     ou réception) contient l'empreinte attendue ; l'agent répond par le
//     verdict JSON STRICT attendu par la machine normative (ancrage exact),
//     dans l'enveloppe d'artefact {jetons, texte}. Ce simulateur NE PRÉSUME
//     PAS que l'API Gemini est un serveur A2A : il matérialise l'adaptateur
//     agent qui traduit A2A vers l'API Gemini.
//   - ExecEchoFactice : agent NEUTRE (indépendant de Gemini) : même rail,
//     réponse en écho. Démontre la RÉUTILISABILITÉ du rail par tout autre
//     fournisseur (E8).
//
// Sécurité (E9) : tout appel SANS jeton valide est refusé (401) ; la carte
// publique A2A reste servie (découverte standard, aucune donnée sensible).
package a2a

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// jetonsFactices : usage déclaré par les agents factices (conversion
// déterministe, jamais facturée : loopback uniquement).
const jetonsFactices = 750

// ServeurFactice : agent A2A loopback avec jeton exigé.
type ServeurFactice struct {
	listener net.Listener
	url      string
	srv      *http.Server
	ferme    *atomic.Bool
}

// URL : URL de BASE de l'agent (résolution /.well-known/agent-card.json).
func (s *ServeurFactice) URL() string { return s.url }

// Fermer : arrêt RÉEL de l'agent : l'écoute est fermée ET tout appel
// ultérieur est refusé (les connexions HTTP persistantes ne servent plus :
// tasks/get échoue côté rail, la délégation non résolue exige le pilote).
func (s *ServeurFactice) Fermer() {
	if s.ferme != nil && !s.ferme.Swap(true) {
		s.srv.Close()
		s.listener.Close()
	}
}

// artefact : enveloppe d'artefact {jetons, texte} (contrat du rail).
func artefact(ec *a2asrv.ExecutorContext, texte string) *a2a.TaskArtifactUpdateEvent {
	payload := fmt.Sprintf(`{"jetons":%d,"texte":%s}`, jetonsFactices,
		mustJSON(texte))
	return a2a.NewArtifactEvent(ec, a2a.NewTextPart(payload))
}

// mustJSON : marshal JSON minimal d'une chaîne (fuite contrôlée impossible :
// textes factices uniquement).
func mustJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r', '\t':
			b.WriteString(" ")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// textDeCharge : texte de la charge A2A reçue.
func texteDeCharge(ec *a2asrv.ExecutorContext) string {
	if ec.Message == nil || len(ec.Message.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range ec.Message.Parts {
		b.WriteString(p.Text())
	}
	return b.String()
}

// empDuPrompt : empreinte 64 hex attendue, extraite de la charge COORD.
// Le gabarit transporté contient le marqueur standard `empreinte_attendue":"`
// (sans séquence d'échappement : le prompt orchestrateur n'échappe pas la
// charge). Absente => "".
func empDuPrompt(charge string) string {
	marque := `empreinte_attendue":"`
	i := strings.Index(charge, marque)
	if i < 0 {
		return ""
	}
	reste := charge[i+len(marque):]
	if len(reste) < 64 {
		return ""
	}
	suite := reste[:64]
	for _, c := range suite {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}
	return suite
}

// verdictDePrompt : produit le verdict JSON STRICT que la machine
// normative attend, ANCRÉ à l'empreinte extraite de la charge.
func verdictDePrompt(emp, verdict string, plan bool) string {
	if emp == "" {
		return "ERREUR_INTERNE_PAS_ANCRABLE"
	}
	if plan {
		return fmt.Sprintf(`{"verdict":%q,"points":[],"empreinte_attendue":%q}`, verdict, emp)
	}
	return fmt.Sprintf(`{"verdict":%q,"points":[],"empreinte_attendue":%q,`+
		`"verdict_id":"A2AFICT01","verdict_url":"https://example.invalid/a2a-fictice"}`,
		verdict, emp)
}

// ExecGeminiFactice : agent GEMINI SIMULÉ (traduit A2A vers la "réponse
// Gemini" attendue). Verdicts plan et réception déterministes :
// PLAN_ACCEPTE / ACCEPTEE (mode nominal). Le champ verdictReception de la
// carte partagée permet de piloter A_CORRIGER pour la démo du cycle de
// correction (C15).
type ExecGeminiFactice struct {
	Partage *PartageGemini
}

// PartageGemini : état partagé du factice Gemini (pilotable par les tests).
type PartageGemini struct {
	// ReponseRefus : si non vide, l'agent échoue la tâche (TACHE_NEGATIVE).
	ReponseRefus string
	// VerdictReception : verdict de réception injecté (ACCEPTEE par défaut;
	// A_CORRIGER pour la démo du cycle de correction).
	VerdictReception string
}

// Cancel : annulation CONFORME (piège SDK du POC : l'événement canceled est
// EXIGÉ, jamais une séquence vide).
func (ExecGeminiFactice) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

// Execute : critique de plan ou réception, verdict ancré, JSON strict.
func (e ExecGeminiFactice) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		charge := texteDeCharge(ec)
		if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
			return
		}
		emp := empDuPrompt(charge)
		if emp == "" {
			// Charge NON conforme au gabarit => ÉCHEC net, données opaques
			// (E7 injection : aucune instruction de la charge n'est exécutée).
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("CHARGE_NON_CONFORME"))), nil)
			return
		}
		var verdict string
		switch {
		case strings.Contains(charge, "CRITIQUE DE PLAN"):
			verdict = "PLAN_ACCEPTE"
			if !yield(artefact(ec, verdictDePrompt(emp, verdict, true)), nil) {
				return
			}
		case strings.Contains(charge, "RECEPTION INDEPENDANTE"):
			vr := "ACCEPTEE"
			if e.Partage != nil && e.Partage.VerdictReception != "" {
				vr = e.Partage.VerdictReception
			}
			if !yield(artefact(ec, verdictDePrompt(emp, vr, false)), nil) {
				return
			}
		default:
			// Exécution générique SIMULÉE (E7 : contenu opaque).
			s := "GEMINI_SIMULE : " + charge
			if len(s) > 200 {
				s = s[:200]
			}
			if !yield(artefact(ec, s), nil) {
				return
			}
		}
		if e.Partage != nil && e.Partage.ReponseRefus != "" {
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed,
				a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(e.Partage.ReponseRefus))), nil)
			return
		}
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
	}
}

// ExecEchoFactice : agent NEUTRE indépendant de Gemini (E8 : le même rail
// sert un second adaptateur simulé).
type ExecEchoFactice struct{}

func (ExecEchoFactice) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

func (ExecEchoFactice) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		charge := texteDeCharge(ec)
		if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
			return
		}
		if strings.Contains(charge, "PAUSE") {
			// Saisie attendue persistée, annulable (E2).
			yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateInputRequired, nil), nil)
			return
		}
		if !yield(artefact(ec, "ECHO:"+charge), nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
	}
}

// carteFactice : AgentCard standard A2A v1.0 d'un agent factice.
func carteFactice(nom, url string) *a2a.AgentCard {
	return &a2a.AgentCard{
		Name:        nom,
		Description: "Agent factice COORD (M-COORD-LLM-02, qualification SIMULEE, 0 EUR)",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(url, a2a.TransportProtocolJSONRPC),
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
	}
}

// DemarrerAgentFactice : écoute loopback, JSON-RPC A2A + carte standard ;
// JETON EXIGÉ sur tout appel (E9 : 401 sans jeton).
func DemarrerAgentFactice(nom string, exec a2asrv.AgentExecutor, jeton string) (*ServeurFactice, error) {
	return demarrer(nom, exec, jeton, nil)
}

// DemarrerAgentFacticeCarte : identique à DemarrerAgentFactice avec une
// carte d'agent fournie (M-COORD-LLM-04 #56 : cartes aux CAPACITÉS
// HONNÊTES — Skills déclarés par fournisseur, aucune capacité simulée).
// La carte DOIT référencer l'interface d'invocation effective : utiliser
// CarteAgent en base puis ajuster. Additif ; le comportement historique
// de DemarrerAgentFactice est inchangé.
func DemarrerAgentFacticeCarte(nom string, exec a2asrv.AgentExecutor, jeton string, carte *a2a.AgentCard) (*ServeurFactice, error) {
	if carte == nil {
		return nil, errors.New("factices : carte d'agent exigée (nil interdit)")
	}
	return demarrer(nom, exec, jeton, carte)
}

// demarrer : démarrage commun (loopback, jeton exigé, fermeture réelle).
func demarrer(nom string, exec a2asrv.AgentExecutor, jeton string, carte *a2a.AgentCard) (*ServeurFactice, error) {
	handler := a2asrv.NewHandler(exec)
	mux := http.NewServeMux()
	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(handler))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	urlInvoke := fmt.Sprintf("http://127.0.0.1:%d/invoke", port)
	urlBase := fmt.Sprintf("http://127.0.0.1:%d", port)

	if carte == nil {
		carte = carteFactice(nom, urlInvoke)
	}
	// La carte DOIT désigner l'URL d'invocation RÉELLE : sans interface
	// d'invocation déclarée, l'agent serait indésinvocable pour un client
	// A2A conforme (V1.0 : SupportedInterfaces obligatoire).
	if len(carte.SupportedInterfaces) == 0 {
		carte.SupportedInterfaces = []*a2a.AgentInterface{
			a2a.NewAgentInterface(urlInvoke, a2a.TransportProtocolJSONRPC),
		}
	}
	mux.Handle(a2asrv.WellKnownAgentCardPath,
		a2asrv.NewStaticAgentCardHandler(carte))

	// Middleware jeton : seuls les appels AUTHENTIFIÉS passent ; agent
	// FERMÉ : tout appel est refusé (arrêt réel simulé, E4).
	ferme := &atomic.Bool{}
	avecJeton := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ferme.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != a2asrv.WellKnownAgentCardPath {
			if r.Header.Get(EnteteJeton) != jeton {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("JETON_AGENT_ABSENT_OU_INCONNU"))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
	srv := &http.Server{Handler: avecJeton}
	go srv.Serve(ln)
	return &ServeurFactice{listener: ln, url: urlBase, srv: srv, ferme: ferme}, nil
}

// CarteAgent : AgentCard standard A2A v1.0, base déclarative pour
// DemarrerAgentFacticeCarte (l'URL d'invocation effective est réglée au
// démarrage). Les Skills (capacités HONNÊTES, M-COORD-LLM-04) restent à
// la charge de l'appelant : honnêteté = déclarer moins plutôt que plus.
func CarteAgent(nom, description string) *a2a.AgentCard {
	return &a2a.AgentCard{
		Name:                nom,
		Description:         description,
		SupportedInterfaces: []*a2a.AgentInterface{
			// URL réglée au démarrage (démarrage effectif ci-dessus).
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
	}
}

// portLibre : port libre loopback (helpers de tests).
func portLibre() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// JetonsFactices : usage déclaré par les agents factices (constante
// SIMULATION documentée : loopback uniquement, jamais facturée).
const JetonsFactices = jetonsFactices

// Executable : alias public du contrat d'exécuteur d'agent (a2asrv).
// Frontière du module : les consommateurs du rail (banc COORD, outils
// tiers) implémentent et passent des exécuteurs SANS importer le SDK
// a2a-go directement (M-COORD-RAIL-01 T2 : cohérent avec l'objectif que
// la dépendance SDK soit un détail d'implémentation du module).
type Executable = a2asrv.AgentExecutor

// CompteurEnvois : exécuteur DÉCORATEUR comptant les message/send reçus
// (observabilité des bancs d'essai : « l'agent n'a jamais exécuté deux
// fois »). N'ajoute AUCUNE sémantique : il délègue intégralement.
type CompteurEnvois struct {
	Inner Executable
	N     *atomic.Int32
}

func (c CompteurEnvois) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	c.N.Add(1)
	return c.Inner.Execute(ctx, ec)
}

func (c CompteurEnvois) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return c.Inner.Cancel(ctx, ec)
}
