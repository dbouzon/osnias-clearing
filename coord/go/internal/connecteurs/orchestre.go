// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

package connecteurs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"coordengine/internal/adaptateurs"
	"coordengine/internal/arret"
	"coordengine/internal/budget"
	"coordengine/internal/missions"
	"coordengine/internal/policy"
	"coordengine/internal/spec"
)

// ActeurPilote : l'identité locale du pilote humain. Aucun appel adaptateur
// n'est jamais qualifié par elle.
const ActeurPilote = "pilote-local"

// Types de journal propres à ce package, DISTINCTS des ECART du moteur (N-20).
const (
	TypeAppelReel     = "APPEL_REEL"
	TypeAppelReserve  = "APPEL_RESERVE"
	TypeSurplus       = "APPEL_SURPLUS"
	TypeAppelEchoue   = "APPEL_ECHOUE"
	TypeReponseTard   = "REPONSE_TARDIVE"
	TypeCritiquePlan  = "CRITIQUE_PLAN"
	TypeReception     = "RECEPTION"
	TypeResolution    = "RESOLUTION_APPEL"
	TypeClotureRefuse = "CLOTURE_REFUSEE"
	TypeCommitRefuse  = "COMMIT_REFUSE"
)

// Erreurs structurées (fail closed, explicites).
type ErrAppelEnCours struct{ Issue string }

func (e ErrAppelEnCours) Error() string {
	return "orchestrateur : APPEL_EN_COURS_NON_RESOLU sur " + e.Issue +
		" : un appel réel antérieur n'est pas résolu (décision pilote requise)"
}

var errReponseTardive = errors.New("orchestrateur : REPONSE_TARDIVE : arrêt pilote survenu pendant l'appel, aucune transition appliquée")

// Orchestrateur : conduite minimale d'une mission réelle inter-IA :
// plan/critique, exécution, soumission, réception, correction, ACK.
// Il possède le registre des adaptateurs (réels ou simulés) et passe
// TOUJOURS par la machine normative pour changer l'état ; aucun texte de
// modèle ne vaut autorisation, aucune clôture automatique n'existe ici.
//
// Invariant d'appel hors verrou (mission #29, étape 4) : l'appel réseau
// n'est JAMAIS exécuté sous le verrou de mission. Chaque aller-retour
// facturable suit RÉSERVER (TX1, verrou court) -> APPELER (hors verrou,
// annulable par contexte) -> COMMITTER (TX2, verrou court). L'arrêt humain
// reste donc possible pendant l'appel ; une réponse arrivée après un arrêt
// est journalisée REPONSE_TARDIVE et n'applique AUCUNE transition.
type Orchestrateur struct {
	ref *missions.Referentiel
	reg map[string]adaptateurs.Adaptateur
}

// NouvelOrchestrateur vérifie le référentiel (R2/S5) et installe le registre.
func NouvelOrchestrateur(ref *missions.Referentiel, reg map[string]adaptateurs.Adaptateur) (*Orchestrateur, error) {
	if ref == nil {
		return nil, errors.New("orchestrateur : référentiel requis")
	}
	if len(reg) == 0 {
		return nil, errors.New("orchestrateur : registre vide (N-01, fail closed)")
	}
	if err := ref.EstConforme(); err != nil {
		return nil, err
	}
	return &Orchestrateur{ref: ref, reg: reg}, nil
}

// AdaptateurOf : l'adaptateur qualifiant un identifiant (fail closed).
func (o *Orchestrateur) AdaptateurOf(id string) (adaptateurs.Adaptateur, bool) {
	a, ok := o.reg[id]
	return a, ok
}

// NouvelleMission crée la mission normative (habilitations fail-closed).
// Les adaptateurs du registre sont triés : le premier qualifie l'exécutant,
// le deuxième le réceptionnaire ; la configuration des rôles gouverne.
func (o *Orchestrateur) NouvelleMission(issue, executantID, recepteurID string,
	roles map[string][]string, plafonds budget.Plafonds) error {
	ids := make([]string, 0, len(o.reg))
	for id := range o.reg {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > 2 {
		ids = ids[:2]
	}
	quaExec, quaRecep := ids[0], ids[len(ids)-1]
	identites := []policy.Identity{
		{ID: executantID, Role: "EXECUTANT", QualifiedBy: quaExec},
		{ID: recepteurID, Role: "RECEPTIONNAIRE", QualifiedBy: quaRecep},
		{ID: ActeurPilote, Role: "PILOTE", QualifiedBy: quaExec},
	}
	if _, err := o.ref.Creer(issue, ids, roles, identites, plafonds); err != nil {
		return err
	}
	return o.ref.Journaliser(issue, map[string]any{
		"type": "CREATION_ORCHESTREE",
		"detail": "mission conduite par connecteurs réels : " +
			"exécutant qualifié par " + quaExec + ", réceptionnaire par " + quaRecep,
	})
}

// ---- enregistrement d'appel en cours (anti-doublons après interruption) ----

type appelEnCours struct {
	Issue      string `json:"issue"`
	Action     string `json:"action"`
	Empreinte  string `json:"empreinte_charge"`
	CleIdemp   string `json:"cle_idempotence"`
	Etat       string `json:"etat"`
	InstantUTC string `json:"instant_utc"`
}

const (
	appelEnCoursEtatEnCours  = "EN_COURS"
	appelEnCoursEtatTermine  = "TERMINE"
	appelEnCoursEtatOrphelin = "ORPHELIN"
)

func cheminAppel(racine, issue string) string {
	return filepath.Join(filepath.Dir(missions.JournalPath(racine, issue)), "appel_en_cours.json")
}

// appelEnCoursNonResolu : fail closed (un fichier illisible est non résolu).
func (o *Orchestrateur) appelEnCoursNonResolu(issue string) (*appelEnCours, error) {
	raw, err := os.ReadFile(cheminAppel(o.ref.Racine(), issue))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.New("orchestrateur : enregistrement d'appel illisible")
	}
	var a appelEnCours
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, errors.New("orchestrateur : enregistrement d'appel illisible")
	}
	if a.Etat == appelEnCoursEtatEnCours || a.Etat == appelEnCoursEtatOrphelin {
		return &a, nil
	}
	return nil, nil
}

func (o *Orchestrateur) appelEnCoursEcrit(a *appelEnCours) error {
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(cheminAppel(o.ref.Racine(), a.Issue)), "appel_en_cours.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, cheminAppel(o.ref.Racine(), a.Issue))
}

// ResoudreAppelEnCours : décision EXPLICITE du pilote sur un appel laissé
// non résolu (crash pendant un appel) : acte gouverné, journalisé, jamais
// automatique. resolution vaut "RECU" (la réponse est arrivée) ou "PERDU".
func (o *Orchestrateur) ResoudreAppelEnCours(issue, resolution, motif, par string) error {
	a, err := o.appelEnCoursNonResolu(issue)
	if err != nil {
		return err
	}
	if a == nil {
		return errors.New("orchestrateur : aucun appel en cours à résoudre sur " + issue)
	}
	a.Etat = "RESOLU_" + resolution
	if err := o.appelEnCoursEcrit(a); err != nil {
		return err
	}
	return o.ref.Journaliser(issue, map[string]any{
		"type":   TypeResolution,
		"detail": "appel en cours résolu (" + resolution + ") : " + motif,
		"par":    par,
	})
}

func empreinteCharge(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func rev40(s string) string {
	h := sha256.Sum256([]byte("revision|" + s))
	return hex.EncodeToString(h[:])[:40]
}

func resoudreActeur(enr *missions.Enregistrement, acteur string) (policy.Identity, bool) {
	for _, id := range enr.Identites {
		if id.ID != acteur {
			continue
		}
		if acteur == ActeurPilote && id.Role == "PILOTE" {
			return id, true
		}
		if acteur != ActeurPilote && (id.Role == "EXECUTANT" || id.Role == "RECEPTIONNAIRE") {
			return id, true
		}
	}
	return policy.Identity{}, false
}

// ---- séquence RÉSERVER -> APPELER -> COMMITTER ----

type preparation struct {
	identite    policy.Identity
	adaptateur  string
	estimation  int
	estCentimes int // réservation monétaire durable de la tentative (réel uniquement)
	cleIdemp    string
	charge      string
	action      string
}

// preparer : TX1. Vérifie absence d'appel non résolu, arrêt, reprise,
// acteur habilité, validation normative sur COPIE (C2) quand la cible est
// connue, RÉSERVE le budget de façon DURABLE (R3 ; la réservation est
// consommée même en cas d'échec : l'incertitude est payée, fail closed) :
// jetons ET, pour un adaptateur réel, enveloppe MONÉTAIRE conservatrice en
// centimes opposée au plafond de la mission (réserve 7154948522), et
// inscrit l'appel en cours (anti-doublons après interruption).
func (o *Orchestrateur) preparer(issue, cible, action, acteur, charge string) (*preparation, error) {
	var p *preparation
	err := o.ref.Transaction(issue, func(ctx *missions.ContexteMission) error {
		p = nil
		a, errNa := o.appelEnCoursNonResolu(issue)
		if errNa != nil {
			return errNa
		}
		if a != nil {
			return ErrAppelEnCours{Issue: issue}
		}
		enr := ctx.Enr
		if arret.Actif(&enr.Arret) {
			return errors.New("orchestrateur : transition refusée : arrêt demandé par le pilote (" +
				enr.Arret.Motif + ")")
		}
		if enr.RepriseAttendue {
			return errors.New("orchestrateur : reprise normative requise après levée d'arrêt (N-16/N-17)")
		}
		id, ok := resoudreActeur(enr, acteur)
		if !ok {
			return errors.New("orchestrateur : acteur inconnu ou non habilité : " + acteur)
		}
		if cible == "" {
			// Pas de cible d'état (critique de plan) : seule l'habilitation
			// de l'action est vérifiée sur une copie, rien d'autre ne change.
			actionAPI := actionVersActionSpec(action)
			if !policy.Authorized(id, actionAPI, enr.Adaptateurs, enr.Roles) {
				return errors.New("orchestrateur : identité non habilitée pour l'action " + actionAPI)
			}
		} else {
			copie, errC := missions.Copie(enr)
			if errC != nil {
				return errC
			}
			var donnees map[string]string
			if cible == spec.EtatExecution {
				donnees = map[string]string{"mandat": charge}
			} else if cible == spec.EtatDecisionAttendue {
				donnees = map[string]string{"revision": rev40(charge), "empreinte_preuves": empreinteCharge(charge)}
			}
			if okT, motif := copie.Mission.Transition(cible, id, enr.Adaptateurs, enr.Roles, donnees); !okT {
				return errors.New("orchestrateur : validation normative refusée avant appel : " + motif)
			}
		}
		estimation := adaptateurs.JetonsEstimes(actionVersActionEstimation(action))
		if okB, motifB := enr.Budget.VerifierAvantAppel(id.QualifiedBy, estimation); !okB {
			return errors.New("orchestrateur : BUDGET_DEPASSE : " + motifB)
		}
		enr.Budget.Consommer(id.QualifiedBy, estimation)
		// Voûte monétaire (réserve 7154948522) : pour un adaptateur RÉEL,
		// une estimation CONSERVATRICE en centimes est réservée DURABLEMENT
		// AVANT tout appel : refus si aucun plafond n'est armé, si un tarif
		// ou une borne est inconnu, ou si le plafond monétaire de la mission
		// serait dépassé. Les adaptateurs loopback/simulés restent 0 €.
		estCentimes := 0
		if a, okA := o.reg[id.QualifiedBy]; okA {
			if h, okH := a.(ConnecteurReel); okH && !h.SimuleLocal() {
				if enr.Budget.Plafonds.Centimes <= 0 {
					return errors.New("orchestrateur : APPEL_REEL_REFUSE : aucun plafond " +
						"monetaire arme pour la mission (fail closed, reserve 7154948522)")
				}
				var errT error
				estCentimes, errT = h.EstimationCentimesSur()
				if errT != nil {
					return errors.New("orchestrateur : APPEL_REEL_REFUSE : " + errT.Error())
				}
				if okB, motifB := enr.Budget.ReserverCentimes(id.QualifiedBy, estCentimes); !okB {
					return errors.New("orchestrateur : BUDGET_DEPASSE : " + motifB)
				}
			}
		}
		p = &preparation{
			identite:    id,
			adaptateur:  id.QualifiedBy,
			estimation:  estimation,
			estCentimes: estCentimes,
			cleIdemp: empreinteCharge(issue + "|" + action + "|" + charge + "|" +
				time.Now().UTC().Format(time.RFC3339Nano)),
			charge: charge,
			action: action,
		}
		return o.appelEnCoursEcrit(&appelEnCours{
			Issue: issue, Action: action, Empreinte: empreinteCharge(charge),
			CleIdemp: p.cleIdemp, Etat: appelEnCoursEtatEnCours,
			InstantUTC: time.Now().UTC().Format(time.RFC3339),
		})
	}, nil)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("orchestrateur : préparation incohérente (défaut interne)")
	}
	o.ref.Journaliser(issue, map[string]any{
		"type": TypeAppelReserve, "action": p.action, "adaptateur": p.adaptateur,
		"jetons_reserves": p.estimation, "cle_idempotence": p.cleIdemp,
		"centimes_reserves": p.estCentimes,
		"detail": "centimes_reserves = estimation conservatrice LOCALE declaree " +
			"(tarifs + bornes + retraits), pas une garantie fournisseur",
	})
	return p, nil
}

// appelerReel : appel réseau HORS verrou, annulable, idempotence persistée.
func (o *Orchestrateur) appelerReel(ctx context.Context, p *preparation) (adaptateurs.Resultat, error) {
	a, ok := o.reg[p.adaptateur]
	if !ok {
		return adaptateurs.Resultat{}, errors.New("orchestrateur : adaptateur non enregistré : " + p.adaptateur)
	}
	h, okH := a.(ConnecteurReel)
	if !okH {
		// Adaptateur simulé : l'anti-doublon et la réservation s'appliquent
		// quand même ; l'appel reste local, sans réseau (0 €).
		var res adaptateurs.Resultat
		var err error
		if p.action == "executer" {
			res, err = a.Executer(p.charge)
		} else {
			res, err = a.Soumettre(p.charge)
		}
		return res, err
	}
	var res adaptateurs.Resultat
	var err error
	switch p.action {
	case "executer":
		res, err = h.ExecuterContexte(ctx, "executer", p.charge, p.cleIdemp)
	default:
		res, err = h.ExecuterContexte(ctx, p.action, p.charge, p.cleIdemp)
	}
	return res, err
}

// committer : TX2. Relit l'état persisté, refuse la transition si un arrêt
// est survenu pendant l'appel (réponse tardive : état inchangé), applique
// la transition normative, paie l'éventuel surplus de jetons réels,
// enregistre l'éventuel SURCOÛT monétaire réel (une réservation n'est
// jamais libérée : conservateur par construction), marque l'appel TERMINE
// et journalise. donnees supplémentaires fournies quand
// la réponse les contient (verdict de réception).
func (o *Orchestrateur) committer(issue, cible, action string, p *preparation,
	res adaptateurs.Resultat, donneesExtra map[string]string, sansTransition bool) error {

	var motifRefus string
	echec := o.ref.Transaction(issue, func(c *missions.ContexteMission) error {
		enr := c.Enr
		if arret.Actif(&enr.Arret) {
			_ = o.appelEnCoursEcrit(&appelEnCours{
				Issue: issue, Action: action, Empreinte: empreinteCharge(p.charge),
				CleIdemp: p.cleIdemp, Etat: appelEnCoursEtatOrphelin,
				InstantUTC: time.Now().UTC().Format(time.RFC3339),
			})
			return errReponseTardive
		}
		if surplus := res.Jetons - p.estimation; surplus > 0 {
			enr.Budget.AjusterJetons(p.adaptateur, surplus)
		}
		// Voûte monétaire (réserve 7154948522) : un ÉVENTUEL surcoût réel
		// constaté est ENREGISTRÉ (jamais libéré) ; le coût réel ne peut
		// jamais libérer de réservation (conservateur par construction).
		if p.estCentimes > 0 {
			if a, okA := o.reg[p.adaptateur]; okA {
				if h, okH := a.(ConnecteurReel); okH {
					if coutReel, errC := h.CoutReelCentimes(res.Jetons); errC == nil &&
						coutReel > p.estCentimes {
						enr.Budget.AjusterCentimes(p.adaptateur, coutReel-p.estCentimes)
					}
				}
			}
		}
		if !sansTransition {
			donnees := map[string]string{}
			for k, v := range donneesExtra {
				donnees[k] = v
			}
			switch cible {
			case spec.EtatExecution:
				donnees["mandat"] = p.charge
			case spec.EtatDecisionAttendue:
				donnees["revision"] = rev40(p.charge)
				donnees["empreinte_preuves"] = empreinteCharge(p.charge)
			case spec.EtatACKRecu:
				if n := len(enr.Mission.Decisions); n > 0 {
					donnees["empreinte_courante"] = enr.Mission.Decisions[n-1].Empreinte
					donnees["revision_courante"] = enr.Mission.Decisions[n-1].Revision
				}
			}
			okT, motif := enr.Mission.Transition(cible, p.identite, enr.Adaptateurs, enr.Roles, donnees)
			if !okT {
				motifRefus = motif
				return nil // le refus est explicite mais la comptabilité reste sauvegardée
			}
		}
		return o.appelEnCoursEcrit(&appelEnCours{
			Issue: issue, Action: action, Empreinte: empreinteCharge(p.charge),
			CleIdemp: p.cleIdemp, Etat: appelEnCoursEtatTermine,
			InstantUTC: time.Now().UTC().Format(time.RFC3339),
		})
	}, nil)
	if echec != nil {
		if errors.Is(echec, errReponseTardive) {
			o.ref.Journaliser(issue, map[string]any{
				"type": TypeReponseTard, "action": action,
				"detail":       "réponse arrivée pendant/après un arrêt pilote : aucune transition appliquée, aucune fraude d'état",
				"jetons_reels": res.Jetons,
			})
		}
		return echec
	}
	if motifRefus != "" {
		// Transition refusée au commit : comptabilité consommée (fail
		// closed), état inchangé, appel marqué TERMINE côté comptabilité.
		_ = o.appelEnCoursEcrit(&appelEnCours{
			Issue: issue, Action: action, Empreinte: empreinteCharge(p.charge),
			CleIdemp: p.cleIdemp, Etat: appelEnCoursEtatOrphelin,
			InstantUTC: time.Now().UTC().Format(time.RFC3339),
		})
		o.ref.Journaliser(issue, map[string]any{
			"type": TypeCommitRefuse, "action": action,
			"detail": "transition refusée au commit : " + motifRefus + " (aucune fraude d'état)",
		})
		return fmt.Errorf("orchestrateur : COMMIT_REFUSE : %s (réservation conservée, état inchangé)", motifRefus)
	}
	o.ref.Journaliser(issue, map[string]any{
		"type": TypeAppelReel, "action": action, "adaptateur": p.adaptateur,
		"jetons": res.Jetons, "empreinte_sortie": empreinteCharge(res.Sortie),
	})
	if surplus := res.Jetons - p.estimation; surplus > 0 {
		o.ref.Journaliser(issue, map[string]any{
			"type":       TypeSurplus,
			"detail":     fmt.Sprintf("jetons réels supérieurs à la réservation : +%d", surplus),
			"adaptateur": p.adaptateur,
		})
	}
	return nil
}

func actionVersActionSpec(action string) string {
	switch action {
	case "executer":
		return spec.ActionExecuter
	case "soumettre":
		return spec.ActionSoumettre
	case "recevoir", "critiquer":
		return spec.ActionRecevoir
	case "acker":
		return spec.ActionAcker
	}
	return action
}

func actionVersActionEstimation(action string) string {
	if action == "executer" {
		return "executer"
	}
	return "soumettre"
}

// ---- orchestration du cycle mission #29 ----

// Executer : transition PLANIFIEE -> EXECUTION (ou EN_CORRECTION -> EXECUTION)
// avec appel réel de l'adaptateur executant, hors verrou.
func (o *Orchestrateur) Executer(ctx context.Context, issue, executantID, mandat string) (adaptateurs.Resultat, error) {
	p, err := o.preparer(issue, spec.EtatExecution, "executer", executantID, mandat)
	if err != nil {
		return adaptateurs.Resultat{}, err
	}
	res, err := o.appelerReel(ctx, p)
	if err != nil {
		o.echecAppel(issue, p, err)
		return adaptateurs.Resultat{}, err
	}
	if err := o.committer(issue, spec.EtatExecution, "executer", p, res, nil, false); err != nil {
		return res, err
	}
	return res, nil
}

// echecAppel : un appel échoué garde la réservation (fail closed, incertitude
// payée), journalise l'échec et marque l'appel résolu par décision... NON :
// l'échec est EXPLICITE (aucune réponse ambiguë), l'appel est clos ECHOUÉ et
// peut être retenté par le pilote (chaque tentative est comptée).
func (o *Orchestrateur) echecAppel(issue string, p *preparation, err error) {
	_ = o.appelEnCoursEcrit(&appelEnCours{
		Issue: issue, Action: p.action, Empreinte: empreinteCharge(p.charge),
		CleIdemp: p.cleIdemp, Etat: "RESOLU_ECHOUE",
		InstantUTC: time.Now().UTC().Format(time.RFC3339),
	})
	o.ref.Journaliser(issue, map[string]any{
		"type": TypeAppelEchoue, "action": p.action, "adaptateur": p.adaptateur,
		"detail": "échec explicite de l'appel (réservation conservée, fail closed) : " + err.Error(),
	})
}

// Soumettre : transition EXECUTION -> DECISION_ATTENDUE avec appel réel de
// l'adaptateur executant sur le livrable (l'ancrage révision/empreinte est
// calculé côté orchestrateur depuis le livrable, jamais depuis le modèle).
func (o *Orchestrateur) Soumettre(ctx context.Context, issue, executantID, livrable string) (adaptateurs.Resultat, error) {
	p, err := o.preparer(issue, spec.EtatDecisionAttendue, "soumettre", executantID, livrable)
	if err != nil {
		return adaptateurs.Resultat{}, err
	}
	res, err := o.appelerReel(ctx, p)
	if err != nil {
		o.echecAppel(issue, p, err)
		return adaptateurs.Resultat{}, err
	}
	if err := o.committer(issue, spec.EtatDecisionAttendue, "soumettre", p, res, nil, false); err != nil {
		return res, err
	}
	return res, nil
}

// VerdictPlan : verdict structuré de critique de plan (ancré au plan).
type VerdictPlan struct {
	Verdict           string   `json:"verdict"`
	Points            []string `json:"points"`
	EmpreinteAttendue string   `json:"empreinte_attendue"`
}

// CritiquerPlan : critique du plan par l'adaptateur réceptionnaire AVANT
// exécution (échange plan/critique, mission #29 étape de cycle). Aucune
// transition : la mission reste PLANIFIEE ; le verdict est structuré, ancré
// à l'empreinte du plan, et validé (un verdict non conforme est rejeté,
// fail closed, il ne vaut jamais acceptation).
func (o *Orchestrateur) CritiquerPlan(ctx context.Context, issue, recepteurID, plan string) (VerdictPlan, error) {
	var out VerdictPlan
	emp := empreinteCharge(plan)
	charge := "CRITIQUE DE PLAN (projet COORD, cycle Mistral exécutant / OpenAI réceptionnaire).\n" +
		"Tu es le réceptionnaire indépendant. Réponds STRICTEMENT en JSON, sans texte autour :\n" +
		"{\"verdict\":\"PLAN_ACCEPTE|A_CORRIGER|REFUSEE\",\"points\":[\"...\"],\"empreinte_attendue\":\"" + emp + "\"}\n" +
		"Les points sont tes objections ou réserves, en phrases courtes. empreinte_attendue vaut\n" +
		"exactement " + emp + " (ancrage du plan critiqué). Aucune autre clé.\n\n--- PLAN ---\n" + plan
	p, err := o.preparer(issue, "", "critiquer", recepteurID, charge)
	if err != nil {
		return out, err
	}
	res, err := o.appelerReel(ctx, p)
	if err != nil {
		o.echecAppel(issue, p, err)
		return out, err
	}
	if err := o.committer(issue, "", "critiquer", p, res, nil, true); err != nil {
		return out, err
	}
	v, err := VerdictPlanDeSortie(res.Sortie, emp)
	if err != nil {
		o.ref.Journaliser(issue, map[string]any{
			"type": TypeCritiquePlan, "detail": "verdict NON CONFORME rejeté (fail closed) : " + err.Error(),
		})
		return out, err
	}
	o.ref.Journaliser(issue, map[string]any{
		"type": TypeCritiquePlan, "verdict": v.Verdict,
		"empreinte_plan": v.EmpreinteAttendue, "points": len(v.Points),
	})
	return v, nil
}

// VerdictPlanDeSortie : validation du verdict structuré (fail closed).
func VerdictPlanDeSortie(sortie, empreinteAttendue string) (VerdictPlan, error) {
	var v VerdictPlan
	s := strings.TrimSpace(sortie)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if !strings.HasPrefix(strings.TrimSpace(s), "{") {
		return v, errors.New("verdict non JSON")
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return v, errors.New("verdict JSON invalide")
	}
	switch v.Verdict {
	case "PLAN_ACCEPTE", "A_CORRIGER", "REFUSEE":
	default:
		return v, errors.New("verdict hors énumération (PLAN_ACCEPTE|A_CORRIGER|REFUSEE)")
	}
	if v.EmpreinteAttendue != empreinteAttendue {
		return v, errors.New("verdict non ancré : empreinte divergente")
	}
	return v, nil
}

// VerdictReception : verdict structuré du réceptionnaire (ancré à la
// soumission en attente) : réception indépendante, mission #29.
type VerdictReception struct {
	Verdict           string   `json:"verdict"`
	Points            []string `json:"points"`
	EmpreinteAttendue string   `json:"empreinte_attendue"`
	VerdictID         string   `json:"verdict_id"`
	VerdictURL        string   `json:"verdict_url"`
}

// CibleDeVerdict : verdict -> cible normative (fail closed).
func CibleDeVerdict(verdict string) (string, bool) {
	switch verdict {
	case "ACCEPTEE":
		return spec.EtatAcceptee, true
	case "A_CORRIGER":
		return spec.EtatEnCorrection, true
	case "REFUSEE":
		return spec.EtatRefusee, true
	}
	return "", false
}

// Recevoir : réception indépendante par appel réel du réceptionnaire, puis
// transition normative avec un verdict structuré validé et ancré. Le
// contenu du modèle n'est JAMAIS cru : seules les valeurs conformes à la
// soumission en attente (empreinte) et à l'énumération passent.
func (o *Orchestrateur) Recevoir(ctx context.Context, issue, recepteurID, livrable string) (VerdictReception, adaptateurs.Resultat, error) {
	var out VerdictReception
	enr, err := o.ref.Obtenir(issue)
	if err != nil {
		return out, adaptateurs.Resultat{}, err
	}
	if enr.Mission.State != spec.EtatDecisionAttendue || enr.Mission.EnAttente == nil {
		return out, adaptateurs.Resultat{}, errors.New("orchestrateur : aucune soumission en attente (état " + enr.Mission.State + ")")
	}
	emp := enr.Mission.EnAttente.Empreinte
	if empreinteCharge(livrable) != emp {
		return out, adaptateurs.Resultat{}, errors.New("orchestrateur : livrable non conforme : " +
			"son empreinte diverge de la soumission en attente (fail closed, aucun appel)")
	}
	charge := "RECEPTION INDEPENDANTE (projet COORD). Réponds STRICTEMENT en JSON, sans texte autour :\n" +
		"{\"verdict\":\"ACCEPTEE|A_CORRIGER|REFUSEE\",\"points\":[\"...\"],\"empreinte_attendue\":\"" + emp + "\",\n" +
		"\"verdict_id\":\"<identifiant court de ton verdict>\",\"verdict_url\":\"<URL ou référence publique de ton verdict>\"}\n" +
		"empreinte_attendue vaut exactement " + emp + " (ancrage de la soumission reçue). Aucune autre clé.\n" +
		"Critique factuelle : conformité aux critères annoncés, défauts bloquants, réserves non bloquantes.\n" +
		"Tu n'as AUCUN pouvoir de clôture : ton verdict nourrit la machine normative qui décide.\n\n" +
		"--- SOUMISSION (empreinte " + emp + ") ---\n" + livrable
	p, err := o.preparer(issue, "", "recevoir", recepteurID, charge)
	if err != nil {
		return out, adaptateurs.Resultat{}, err
	}
	res, err := o.appelerReel(ctx, p)
	if err != nil {
		o.echecAppel(issue, p, err)
		return out, adaptateurs.Resultat{}, err
	}
	v, err := VerdictReceptionDeSortie(res.Sortie, emp)
	if err != nil {
		// verdict non conforme : la réception ÉCHOUE explicitement, la
		// soumission reste en attente (aucune fraude), le coût est payé.
		o.echecAppel(issue, p, err)
		return out, res, err
	}
	cible, ok := CibleDeVerdict(v.Verdict)
	if !ok {
		o.echecAppel(issue, p, errors.New("verdict hors énumération"))
		return out, res, errors.New("orchestrateur : verdict hors énumération de réception")
	}
	donnees := map[string]string{
		"verdict":    v.Verdict,
		"verdict_id": v.VerdictID,
	}
	if v.VerdictURL == "" {
		v.VerdictURL = "urn:coord:verdict:" + issue + "/" + v.VerdictID
	}
	donnees["verdict_url"] = v.VerdictURL
	err = o.committer(issue, cible, "recevoir", p, res, donnees, false)
	if err != nil {
		return v, res, err
	}
	o.ref.Journaliser(issue, map[string]any{
		"type": TypeReception, "verdict": v.Verdict, "cible": cible,
		"verdict_id": v.VerdictID, "empreinte": emp, "points": len(v.Points),
	})
	return v, res, nil
}

// VerdictReceptionDeSortie : validation fail-closed du verdict de réception.
func VerdictReceptionDeSortie(sortie, empreinteAttendue string) (VerdictReception, error) {
	var v VerdictReception
	s := strings.TrimSpace(sortie)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if !strings.HasPrefix(strings.TrimSpace(s), "{") {
		return v, errors.New("verdict non JSON")
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return v, errors.New("verdict JSON invalide")
	}
	if _, ok := CibleDeVerdict(v.Verdict); !ok {
		return v, errors.New("verdict hors énumération (ACCEPTEE|A_CORRIGER|REFUSEE)")
	}
	if v.EmpreinteAttendue != empreinteAttendue {
		return v, errors.New("verdict non ancré : empreinte divergente")
	}
	if strings.TrimSpace(v.VerdictID) == "" {
		return v, errors.New("verdict sans référence (verdict_id obligatoire)")
	}
	return v, nil
}

// Acker : ACK de l'exécutant sur la dernière décision recevable. Aucun appel
// réseau (le protocole COORD ne facture pas l'ACK ; 0 €), transition
// normative ACCEPTEE -> ACK_RECU ancrée à la décision.
func (o *Orchestrateur) Acker(issue, executantID string) error {
	echec := o.ref.Transaction(issue, func(c *missions.ContexteMission) error {
		enr := c.Enr
		if arret.Actif(&enr.Arret) {
			return errors.New("orchestrateur : ACK refusé : arrêt pilote actif")
		}
		if enr.RepriseAttendue {
			return errors.New("orchestrateur : reprise normative requise (N-16/N-17)")
		}
		id, ok := resoudreActeur(enr, executantID)
		if !ok {
			return errors.New("orchestrateur : acteur inconnu : " + executantID)
		}
		var donnees map[string]string
		if n := len(enr.Mission.Decisions); n > 0 {
			donnees = map[string]string{
				"empreinte_courante": enr.Mission.Decisions[n-1].Empreinte,
				"revision_courante":  enr.Mission.Decisions[n-1].Revision,
			}
		}
		okT, motif := enr.Mission.Transition(spec.EtatACKRecu, id, enr.Adaptateurs, enr.Roles, donnees)
		if !okT {
			return errors.New("orchestrateur : ACK refusé : " + motif)
		}
		return nil
	}, nil)
	if echec != nil {
		return echec
	}
	return o.ref.Journaliser(issue, map[string]any{
		"type": "ACK", "detail": "ACK de l'exécutant " + executantID + " sur la décision reçue",
	})
}

// Cloturer : REFUSÉ par construction. Pas de clôture automatique (mission
// #29, spec 6.6) : la clôture relève du pilote humain uniquement.
func (o *Orchestrateur) Cloturer(issue string) error {
	_ = o.ref.Journaliser(issue, map[string]any{
		"type":   TypeClotureRefuse,
		"detail": "clôture réservée au pilote : refus de l'automatisme (aucun texte de modèle ne vaut autorisation)",
	})
	return errors.New("orchestrateur : CLOTURE_REFUSEE : la clôture est réservée au pilote humain")
}
