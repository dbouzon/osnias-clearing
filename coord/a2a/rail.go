// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package a2a : rail A2A DIRECT générique et réutilisable de COORD
// (mission M-COORD-LLM-02, issue #48, plan v2 PLAN_ACCEPTE).
//
// INVARIANTS (T3) :
//   - COORD reste l'autorité des transitions et du verdict : ce rail ne
//     crée AUCUNE mission, ne délivre AUCUN verdict, ne modifie AUCUN état
//     mission ; il est un CONNECTEUR au sens adaptateurs.Adaptateur.
//     Pas de second orchestrateur (E10).
//   - indépendance fournisseur : rien dans ce fichier ne mentionne Gemini ;
//     un adaptateur agent traduit A2A vers l'API du fournisseur.
//   - identifiant de tâche DURABLE exigé : un message/send qui ne produit
//     pas un Task A2A (identifiant, suivable par tasks/get) est REFUSÉ.
//   - persistance : chaque délégation est inscrite dans un registre local
//     atomique (tmp + fsync + rename) ; après un arrêt réel du processus,
//     le rail retrouve ses tâches (E5) et les tâches INCERTAINES restent
//     non résolues jusqu'à décision du PILOTE EXCLUSIVEMENT (Q4, SEC-01) :
//     aucun retry fournisseur automatique.
//   - déduplication : une même clé d'idempotence ne délègue JAMAIS deux
//     fois (E3) ; le second appel est rejeté en doublon, aucun second
//     débit budgétaire n'est possible.
//   - authentification et isolation : jeton LOCAL distinct du token
//     console pilote, exigé ; loopback IMPOSÉ dans cette tranche (tout
//     autre hôte refusé au chargement, fail closed) ; HTTPS distant
//     documenté mais non activé.
package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

// EnteteJeton : jeton d'agent du rail, DISTINCT du token console pilote.
const EnteteJeton = "X-Coord-Rail-Jeton"

// États de tâche du rail (mapping fermé des états A2A v1.0 ; tout état
// inconnu est REFUSÉ, fail closed).
const (
	EtatSoumise        = "submitted"
	EtatEnCours        = "working"
	EtatSaisieAttendue = "input-required"
	EtatAchevee        = "completed"
	EtatAnnulee        = "canceled"
	EtatEchouee        = "failed"
)

// Erreurs structurées du rail.
var (
	// ErrDoublon : la clé d'idempotence a déjà délégué une tâche ; le
	// second appel est REFUSÉ (jamais deux débits budgétaires, E3).
	ErrDoublon = errors.New("a2a : DOUBLON_REFUSE : tâche déjà déléguée sous cette clé " +
		"(aucune nouvelle délégation, fail closed)")
	// ErrTerminalSansResultat : tâche terminée sans résultat exploitable.
	ErrTerminalSansResultat = errors.New("a2a : TACHE_TERMINALE_SANS_RESULTAT (fail closed)")
)

// Enregistrement : état durable d'UNE délégation (registre persistant).
type Enregistrement struct {
	Cle    string `json:"cle"`    // clé d'idempotence COORD
	Action string `json:"action"` // executer | soumettre | critiquer | recevoir
	Tache  string `json:"tache"`  // ID de tâche A2A durable
	Etat   string `json:"etat"`   // état courant constaté
	Texte  string `json:"texte"`  // résultat exploitable (si résolu)
	Jetons int    `json:"jetons"` // jetons déclarés par l'agent (0 = inconnu)
	Resolu bool   `json:"resolu"` // résultat collecté et exploitable
	Manuel bool   `json:"manuel"` // résolu PAR LE PILOTE (résolution manuelle)
}

// Rail : client A2A direct vers UN agent, avec registre durable.
type Rail struct {
	client *a2aclient.Client
	jeton  string
	chemin string
	mu     sync.Mutex
	reg    map[string]*Enregistrement
}

// transportJeton : injecte le jeton du rail dans chaque requête A2A.
type transportJeton struct {
	base  http.RoundTripper
	jeton string
}

func (t *transportJeton) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.jeton != "" {
		r.Header.Set(EnteteJeton, t.jeton)
	}
	return t.base.RoundTrip(r)
}

// hostEstLoopback : banc d'essai local uniquement (serveurs factices, 0 €).
func hostEstLoopback(h string) bool {
	host, _, err := net.SplitHostPort(h)
	if err != nil {
		host = h
	}
	return host == "127.0.0.1" || host == "localhost" || host == "::1" ||
		strings.HasPrefix(host, "127.")
}

// NouveauRail résout l'AgentCard (découverte /.well-known/agent-card.json),
// construit le client JSON-RPC et charge le registre durable. Fail closed :
// seuls les agents LOOPBACK sont acceptés dans cette tranche (aucun serveur
// public, aucun fournisseur réel sans autorisation distincte).
func NouveauRail(ctx context.Context, baseURL, jeton, cheminRegistre string) (*Rail, error) {
	u, err := url.Parse(strings.TrimSuffix(baseURL, "/"))
	if err != nil || u.Scheme == "" {
		return nil, errors.New("a2a : URL d'agent invalide (fail closed)")
	}
	if !hostEstLoopback(u.Host) {
		return nil, errors.New("a2a : APPEL_REEL_NON_AUTORISE : rail A2A limité aux agents " +
			"loopback dans cette tranche (fail closed, aucun serveur public)")
	}
	if strings.TrimSpace(jeton) == "" {
		return nil, errors.New("a2a : jeton d'agent requis (distinct du token console, fail closed)")
	}
	if cheminRegistre == "" {
		return nil, errors.New("a2a : chemin de registre durable requis (fail closed)")
	}
	carte, err := agentcard.DefaultResolver.Resolve(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("a2a : AgentCard illisible (%s) : %w", u.String(), err)
	}
	httpClient := &http.Client{Transport: &transportJeton{base: http.DefaultTransport, jeton: jeton}}
	client, err := a2aclient.NewFromCard(ctx, carte, a2aclient.WithJSONRPCTransport(httpClient))
	if err != nil {
		return nil, fmt.Errorf("a2a : client A2A impossible : %w", err)
	}
	r := &Rail{client: client, jeton: jeton, chemin: cheminRegistre, reg: map[string]*Enregistrement{}}
	if err := r.charger(); err != nil {
		return nil, err
	}
	return r, nil
}

// charger : relit le registre persistant (reprise après arrêt réel, E5).
func (r *Rail) charger() error {
	raw, err := os.ReadFile(r.chemin)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("a2a : registre illisible : %w", err)
	}
	var reg map[string]*Enregistrement
	if err := json.Unmarshal(raw, &reg); err != nil {
		return fmt.Errorf("a2a : registre non JSON : %w", err)
	}
	r.reg = reg
	return nil
}

// persister : écriture atomique durable (tmp + fsync + rename).
func (r *Rail) persister() error {
	raw, err := json.Marshal(r.reg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.chemin), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.chemin), ".rail-*")
	if err != nil {
		return err
	}
	nom := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(nom)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(nom)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(nom)
		return err
	}
	if err := os.Chmod(nom, 0o600); err != nil {
		os.Remove(nom)
		return err
	}
	return os.Rename(nom, r.chemin)
}

// etatDepuisA2A : mapping FERMé des états (tout autre état = refus).
func etatDepuisA2A(e a2a.TaskState) (string, error) {
	switch e {
	case a2a.TaskStateSubmitted:
		return EtatSoumise, nil
	case a2a.TaskStateWorking:
		return EtatEnCours, nil
	case a2a.TaskStateInputRequired:
		return EtatSaisieAttendue, nil
	case a2a.TaskStateCompleted:
		return EtatAchevee, nil
	case a2a.TaskStateCanceled:
		return EtatAnnulee, nil
	case a2a.TaskStateFailed:
		return EtatEchouee, nil
	}
	return "", fmt.Errorf("a2a : ETAT_INCONNU %q (fail closed)", string(e))
}

// enveloppe : contrat d'artefact des agents du rail : {"jetons": N,
// "texte": "..."} (JSON strict ; jetons <= 0 = JETONS_INCONNUS fail closed).
type enveloppe struct {
	Jetons int    `json:"jetons"`
	Texte  string `json:"texte"`
}

// extraire : texte + jetons déclarés d'une tâche A2A (fail closed).
func extraire(t *a2a.Task) (string, int, error) {
	if len(t.Artifacts) == 0 || len(t.Artifacts[0].Parts) == 0 {
		return "", 0, ErrTerminalSansResultat
	}
	var env enveloppe
	if err := json.Unmarshal([]byte(t.Artifacts[0].Parts[0].Text()), &env); err != nil {
		return "", 0, fmt.Errorf("a2a : ARTEFACT_NON_CONFORME (contrat {jetons, texte}, fail closed) : %w", err)
	}
	if env.Jetons <= 0 {
		return "", 0, errors.New("a2a : JETONS_INCONNUS : usage non déclaré par l'agent (fail closed)")
	}
	if strings.TrimSpace(env.Texte) == "" {
		return "", 0, errors.New("a2a : REPONSE_VIDE (fail closed)")
	}
	return env.Texte, env.Jetons, nil
}

// Deleguer : UNE délégation par clé (E3 anti-doublon). Retourne
// l'enregistrement durable et EXIGE un identifiant de tâche A2A. Un second
// appel avec la même clé retourne ErrDoublon, SANS nouveau réseau.
func (r *Rail) Deleguer(ctx context.Context, action, charge, cle string) (*Enregistrement, bool, error) {
	if action != "executer" && action != "soumettre" && action != "critiquer" && action != "recevoir" {
		return nil, false, errors.New("a2a : action inconnue " + action)
	}
	r.mu.Lock()
	if ex, ok := r.reg[cle]; ok {
		r.mu.Unlock()
		return ex, true, ErrDoublon
	}
	r.mu.Unlock()

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(charge))
	res, err := r.client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		return nil, false, fmt.Errorf("a2a : message/send échoué : %w", err)
	}
	tache, ok := res.(*a2a.Task)
	if !ok || tache == nil || string(tache.ID) == "" {
		// Identifiant durable EXIGÉ (plan v2 T3) : une réponse sans tâche
		// suivie ne peut être ni annulée ni retrouvée : refus intégral.
		return nil, false, errors.New("a2a : TACHE_NON_DURABLE : l'agent n'a pas retourné de " +
			"tâche A2A suivable (identifiant exigé, fail closed)")
	}
	etat, err := etatDepuisA2A(tache.Status.State)
	if err != nil {
		return nil, false, err
	}
	e := &Enregistrement{Cle: cle, Action: action, Tache: string(tache.ID), Etat: etat}
	r.mu.Lock()
	r.reg[cle] = e
	err = r.persisterSousVerrou()
	r.mu.Unlock()
	if err != nil {
		return nil, false, err
	}
	// Tâche ACHIEVÉE d'emblée : le résultat est capturé depuis le message/send
	// LUI-MÊME (l'artefact est garanti dans le résultat de envoi ; tasks/get
	// NE GARANTIT PAS de retourner les artefacts). Capture + persistance
	// immédiates : que le processus s'arrête ou non, le résultat est acquis.
	if etat == EtatAchevee {
		if texte, jetons, err := extraire(tache); err == nil {
			r.mu.Lock()
			e.Etat = EtatAchevee
			e.Texte = texte
			e.Jetons = jetons
			e.Resolu = true
			err = r.persisterSousVerrou()
			r.mu.Unlock()
			if err != nil {
				return nil, false, err
			}
		} else {
			// Réponse refusée (artefact non conforme, jetons inconnus,
			// sortie vide) : la délégation reste NON résolue, décision
			// pilote requise (fail closed, jamais réparé).
			return nil, false, err
		}
	}
	return e, false, nil
}

// persisterSousVerrou : persister() suppose le verrou déjà pris.
func (r *Rail) persisterSousVerrou() error { return r.persister() }

// Suivre : état COURANT d'une tâche (tasks/get), fail closed sur l'inconnu.
func (r *Rail) Suivre(ctx context.Context, idTache string) (string, error) {
	t, err := r.client.GetTask(ctx, &a2a.GetTaskRequest{ID: a2a.TaskID(idTache)})
	if err != nil {
		return "", fmt.Errorf("a2a : tasks/get échoué (%s) : %w", idTache, err)
	}
	if t == nil || string(t.ID) == "" {
		return "", fmt.Errorf("a2a : TACHE_PERDUE %s (fail closed)", idTache)
	}
	return etatDepuisA2A(t.Status.State)
}

// ResultatDurable : enregistrement résolu d'une tâche, lu du REGISTRE
// DURABLE (aucun réseau : le résultat a été capturé à la délégation).
// Absent => la tâche n'a pas (encore) de résultat exploitable.
func (r *Rail) ResultatDurable(idTache string) (*Enregistrement, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.reg {
		if e.Tache == idTache && e.Resolu {
			copie := *e
			return &copie, true
		}
	}
	return nil, false
}

// ResultatParCle : enregistrement résolu par clé d'idempotence.
func (r *Rail) ResultatParCle(cle string) (*Enregistrement, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.reg[cle]
	return e, ok
}

// Annuler : demande d'annulation A2A (tasks/cancel). L'état ANNULÉE
// retourné par l'agent est EXIGÉ (piège SDK documenté par le POC : la
// séquence d'annulation DOIT produire un événement canceled).
func (r *Rail) Annuler(ctx context.Context, idTache string) (string, error) {
	t, err := r.client.CancelTask(ctx, &a2a.CancelTaskRequest{ID: a2a.TaskID(idTache)})
	if err != nil {
		return "", fmt.Errorf("a2a : tasks/cancel échoué (%s) : %w", idTache, err)
	}
	etat, err := etatDepuisA2A(t.Status.State)
	if err != nil {
		return "", err
	}
	if etat != EtatAnnulee {
		return "", fmt.Errorf("a2a : ANNULATION_NON_CONFORME : état %q après tasks/cancel "+
			"(canceled exigé, fail closed)", etat)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.reg {
		if e.Tache == idTache && !e.Resolu {
			e.Etat = EtatAnnulee
			if err := r.persister(); err != nil {
				return "", err
			}
			return etat, nil
		}
	}
	return etat, nil
}

// NonResolues : délégations NON résolues (après arrêt réel du processus ou
// agent injoignable). Une tâche incertaine exige une décision du PILOTE :
// AUCUNE résolution automatique n'existe (Q4, SEC-01).
func (r *Rail) NonResolues() []Enregistrement {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Enregistrement
	for _, e := range r.reg {
		if !e.Resolu {
			out = append(out, *e)
		}
	}
	return out
}

// ResoudrePilote : résolution MANUELLE par le pilote (marquée manuel=true).
// Seul point de résolution d'une tâche incertaine ; jamais appelé par le
// rail lui-même.
func (r *Rail) ResoudrePilote(cle, texte string, jetons int) error {
	if jetons <= 0 {
		return errors.New("a2a : résolution pilote exige des jetons déclarés > 0 (fail closed)")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.reg[cle]
	if !ok {
		return errors.New("a2a : résolution pilote : clé de délégation inconnue " + cle)
	}
	if e.Resolu {
		return errors.New("a2a : résolution pilote : tâche déjà résolue " + cle)
	}
	e.Resolu = true
	e.Manuel = true
	e.Texte = texte
	e.Jetons = jetons
	e.Etat = EtatAchevee
	return r.persister()
}

// Chemin : chemin du registre durable (lecture seule, pour diagnostics et
// tests embarqués : aucune écriture hors persister).
func (r *Rail) Chemin() string { return r.chemin }

// EtatDepuisA2A : mapping FERMÉ des états A2A exposé (états connus du rail ;
// tout état inconnu est REFUSÉ, fail closed). La vérification « état inconnu
// refusé » EST la frontière de ce mapping : aucun état ne sera jamais deviné.
func EtatDepuisA2A(e a2a.TaskState) (string, error) { return etatDepuisA2A(e) }
