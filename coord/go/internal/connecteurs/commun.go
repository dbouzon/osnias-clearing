// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Briques de sécurité COMMUNES aux connecteurs LLM HTTP réels (mission
// M-COORD-LLM-01, issue #40, décision D1 du plan v2 PLAN_ACCEPTE) :
// HTTPEndPoint (format {model, messages} : Mistral, OpenAI) et
// AnthropicPoint (format Messages API) partagent la MÊME boucle d'appel
// contrôlée : bornage à l'émission, retraits bornés avec backoff, plafond
// de réponse, délai de contexte, expurgation des secrets, refus des usages
// inconnus, voûte monétaire. L'extraction est purement mécanique et prouvée
// par le rejeu intégral des batteries existantes (tests connecteurs,
// reproductions monétaires, harnais SEC-01, race CI) : aucune règle de
// sécurité n'est dupliquée ni modifiée.
package connecteurs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"coordengine/internal/adaptateurs"
)

// secretRE : séquences de clé à expurger de toute sortie. Familles
// couvertes : préfixe sk- (OpenAI et compatibles) et clés Google AI Studio
// (AIza suivi de 35 caractères) — correction M-COORD-RAIL-01 T2 validée par
// le pilote (constat Gemini confirmé : la clé AIza n'était pas expurgée).
var secretRE = regexp.MustCompile(`(sk-[A-Za-z0-9_\-]{8,}|AIza[0-9A-Za-z_\-]{35})`)

// Format d'API déclarés (décision D4 du plan v2) : champ additif "format"
// de la configuration, rétrocompatible ("openai" si absent).
const (
	FormatOpenAI    = "openai"    // corps {model, messages} (Mistral, OpenAI)
	FormatAnthropic = "anthropic" // Messages API (POST /v1/messages)
)

// ConnecteurReel : contrat commun des connecteurs HTTP réels, utilisé par
// l'orchestrateur (voûte monétaire, appel annulable, idempotence). Aucun
// privilège fournisseur : tout connecteur déclaré entre par cette interface.
type ConnecteurReel interface {
	ID() string
	ExecuterContexte(ctx context.Context, action, charge, cleIdempotence string) (adaptateurs.Resultat, error)
	EstimationCentimesSur() (int, error)
	CoutReelCentimes(jetonsTotal int) (int, error)
	// SimuleLocal : endpoint loopback, banc d'essai factice uniquement (0 €).
	SimuleLocal() bool
}

// baseHTTP : champs et contrôles transversaux partagés par les connecteurs
// HTTP réels. Jamais instanciée seule : embarquée par HTTPEndPoint et
// AnthropicPoint, qui ne diffèrent QUE par le format de requête/réponse.
type baseHTTP struct {
	id        string
	endpoint  string
	cle       string
	modele    string
	client    *http.Client
	tailleMax int64
	retraits  int
	simuleLoc bool // endpoint loopback : banc d'essai local uniquement
	// Tarification et bornes déclarées (réserve 7154948522). Zéro = inconnu :
	// l'orchestrateur refuse l'appel d'un connecteur réel (fail closed).
	tarifEntreeMillion int
	tarifSortieMillion int
	tarifRequete       int
	jetonsEntreeMax    int
	jetonsSortieMax    int
}

// ID retourne l'identifiant opaque du fournisseur (aucun privilège).
func (b *baseHTTP) ID() string { return b.id }

// SimuleLocal indique un endpoint loopback (serveurs factices, 0 €).
func (b *baseHTTP) SimuleLocal() bool { return b.simuleLoc }

// cleIdempotence : clé SHA-256 persistée par l'orchestrateur
// (prévention des doublons après interruption).
func (b *baseHTTP) cleIdempotence(action, charge string) string {
	s := sha256.New()
	s.Write([]byte(b.id + "|" + action + "|" + charge))
	return hex.EncodeToString(s.Sum(nil))
}

// expurger retire toute séquence de clé du texte qui sortira du connecteur
// (erreurs et sorties) : la clé n'atteint jamais un journal ni l'UI.
func (b *baseHTTP) expurger(s string) string {
	if b.cle != "" && len(b.cle) >= 8 {
		s = strings.ReplaceAll(s, b.cle, "[EXPURGE]")
	}
	return secretRE.ReplaceAllString(s, "[EXPURGE]")
}

// analyseReponse : hook de format. Retourne le modèle répondant, la sortie
// concaténée et le total de jetons facturés ; toute anomalie de format
// devient une erreur fail closed (jamais de réparation).
type analyseReponse func(lu []byte) (modeleRecu, sortie string, jetons int, err error)

// appel : boucle d'appel COMMUNE (retraits bornés 0..5, backoff plafonné à
// 5 s, annulation par contexte, plafond de réponse, bornage de la requête
// À L'ÉMISSION, rejet/expurgation des secrets, contrôles finaux). Les
// formats ne fournissent que le corps, les en-têtes et l'analyse de
// réponse. Cette méthode ne touche JAMAIS l'état COORD.
func (b *baseHTTP) appel(ctx context.Context, action, charge, cleIdempotence string,
	corps []byte, poserEntetes func(http.Header), analyser analyseReponse) (adaptateurs.Resultat, error) {

	if action != "executer" && action != "soumettre" && action != "critiquer" && action != "recevoir" {
		return adaptateurs.Resultat{}, errors.New("connecteurs : action inconnue " + action)
	}
	// Bornage à L'ÉMISSION (réserve 7154948522) : la requête elle-même est
	// refusée AVANT tout octet émis si sa charge dépasse la borne d'entrée
	// déclarée (heuristique conservatrice : au plus 4 octets UTF-8 par jeton
	// d'entrée). Fail closed, défense en profondeur côté connecteur.
	if !b.simuleLoc && b.jetonsEntreeMax > 0 && len(corps) > b.jetonsEntreeMax*4 {
		return adaptateurs.Resultat{}, errors.New("connecteurs : CHARGE_HORS_BORNE : " +
			"la requête dépasse jetons_entree_max_par_tentative (heuristique 4 octets par " +
			"jeton, fail closed, AUCUN octet émis)")
	}

	refusAnalyse := func(err error) (adaptateurs.Resultat, error) {
		return adaptateurs.Resultat{}, errors.New("connecteurs : " + err.Error())
	}

	var derniere string
	for essai := 0; essai <= b.retraits; essai++ {
		if essai > 0 {
			pause := time.Duration(300*(1<<(essai-1))) * time.Millisecond
			if pause > 5*time.Second {
				pause = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				return adaptateurs.Resultat{}, fmt.Errorf("connecteurs : appel annulé (%d essais) : %w", essai, ctx.Err())
			case <-time.After(pause):
			}
		}
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(corps))
		if err != nil {
			return adaptateurs.Resultat{}, errors.New("connecteurs : requête impossible pour " + b.id)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", cleIdempotence)
		if poserEntetes != nil {
			poserEntetes(r.Header)
		}
		rep, err := b.client.Do(r)
		if err != nil {
			if ctx.Err() != nil {
				return adaptateurs.Resultat{}, fmt.Errorf("connecteurs : appel annulé : %w", ctx.Err())
			}
			derniere = "transport : " + b.expurger(err.Error())
			continue
		}
		lu, err := io.ReadAll(io.LimitReader(rep.Body, b.tailleMax+1))
		_ = rep.Body.Close()
		if err != nil {
			derniere = "lecture de la réponse : " + b.expurger(err.Error())
			continue
		}
		if int64(len(lu)) > b.tailleMax {
			return adaptateurs.Resultat{}, errors.New("connecteurs : REPONSE_TROP_GRANDE : " +
				"la réponse dépasse taille_max_reponse (fail closed)")
		}
		if rep.StatusCode == http.StatusTooManyRequests ||
			rep.StatusCode >= 500 {
			derniere = fmt.Sprintf("HTTP %d", rep.StatusCode)
			continue
		}
		if rep.StatusCode != http.StatusOK {
			return adaptateurs.Resultat{}, errors.New("connecteurs : HTTP " +
				fmt.Sprint(rep.StatusCode) + " (non retentable) : " +
				b.expurger(string(lu[:min64(int64(len(lu)), 256)])))
		}
		modeleRecu, sortie, jetons, err := analyser(lu)
		if err != nil {
			return refusAnalyse(err)
		}
		if b.modele != "" && modeleRecu != "" && modeleRecu != b.modele {
			return adaptateurs.Resultat{}, errors.New("connecteurs : MODELE_DIVERGENT : identité " +
				"répondante (" + modeleRecu + ") distincte du modèle déclaré (" + b.modele + "), fail closed")
		}
		if strings.TrimSpace(sortie) == "" {
			return adaptateurs.Resultat{}, errors.New("connecteurs : REPONSE_VIDE (fail closed)")
		}
		if jetons <= 0 {
			return adaptateurs.Resultat{}, errors.New("connecteurs : JETONS_INCONNUS : " +
				"usage absent de la réponse, budget non garanti (fail closed)")
		}
		if b.cle != "" && strings.Contains(sortie, b.cle) {
			return adaptateurs.Resultat{}, errors.New("connecteurs : SORTIE_COMPROMISE : " +
				"la réponse contient la clé, elle est rejetée intégralement (fail closed)")
		}
		sortie = b.expurger(sortie)
		return adaptateurs.Resultat{Jetons: jetons, Sortie: sortie, Simule: false}, nil
	}
	return adaptateurs.Resultat{}, errors.New("connecteurs : APPEL_ECHOUE : " + b.id + " : " + derniere)
}

// adopter : triple verrou + clés, partagé par tous les constructeurs de
// connecteurs réels (mission #29, étape 4/5). Identique pour tout format.
func (b *baseHTTP) adopter(c *Config, f ConfigFournisseur) error {
	u, err := parseEndpoint(f.Endpoint)
	if err != nil {
		return errors.New("connecteurs : endpoint invalide pour " + f.ID)
	}
	if schemeNonSecurise(u) && !b.simuleLoc {
		return errors.New("connecteurs : endpoint " + f.ID +
			" non sécurisé (https exigé hors loopback, fail closed)")
	}
	if !b.simuleLoc {
		if c.Mode != ModeReel {
			return errors.New("connecteurs : APPEL_REEL_NON_AUTORISE : mode REEL requis pour " + f.ID)
		}
		if os.Getenv(EnvModeReel) != "1" {
			return errors.New("connecteurs : APPEL_REEL_NON_AUTORISE : " +
				EnvModeReel + "=1 requis (aucun appel facturable par défaut)")
		}
		if c.Autorisation == nil || !contient(c.Autorisation.Fournisseurs, f.ID) {
			return errors.New("connecteurs : APPEL_REEL_NON_AUTORISE : fournisseur " + f.ID +
				" absent de l'autorisation du pilote")
		}
		cle := os.Getenv(f.CleEnv)
		if strings.TrimSpace(cle) == "" {
			return errors.New("connecteurs : APPEL_REEL_NON_AUTORISE : clé absente (" +
				f.CleEnv + " non renseignée, fail closed)")
		}
		b.cle = cle
	} else if f.CleEnv != "" {
		// Loopback : clé optionnelle, pour les tests de fuite de secret.
		b.cle = os.Getenv(f.CleEnv)
	}
	return nil
}

// ---- tarif monétaire conservateur (réserve 7154948522) ----

// coutJetons : coût en centimes de `jetons` au tarif déclaré (centimes par
// million de jetons), arrondi VERS LE HAUT en arithmétique ENTIÈRE : aucun
// flottant, aucun sous-comptage, unités explicites (centimes d'euro).
func coutJetons(jetons, tarifCentimesParMillion int) int {
	return (jetons*tarifCentimesParMillion + 999_999) / 1_000_000
}

// EnveloppeCentimes : formule conservatrice PARTAGÉE (mission
// M-COORD-LLM-02 : le rail A2A l'applique comme les connecteurs HTTP,
// sans dupliquer la règle monétaire). Centimes d'euro ENTIERS, arrondi
// vers le haut ; tarifs/bornes <= 0 => TARIF_INCONNU (fail closed).
func EnveloppeCentimes(tarifEntree, tarifSortie, tarifRequete, jetonsEntreeMax, jetonsSortieMax, retraits int) (int, error) {
	if tarifEntree <= 0 || tarifSortie <= 0 || tarifRequete <= 0 ||
		jetonsEntreeMax <= 0 || jetonsSortieMax <= 0 {
		return 0, errors.New("TARIF_INCONNU : tarifs (entree/sortie/requete) ou bornes absents " +
			": aucun plafond monetaire opposable (fail closed)")
	}
	parTentative := coutJetons(jetonsEntreeMax, tarifEntree) +
		coutJetons(jetonsSortieMax, tarifSortie) + tarifRequete
	return (1 + retraits) * parTentative, nil
}

// CoutConservateurJetons : coût À POSTERIORI d'un usage TOTAL, au tarif le
// plus élevé des deux déclarés (on ne sous-compte jamais), + requête.
// Centimes d'euro ENTIERS, aucun flottant. PARTAGÉE (M-COORD-LLM-02).
func CoutConservateurJetons(tarifEntree, tarifSortie, tarifRequete, jetonsTotal int) (int, error) {
	if tarifEntree <= 0 || tarifSortie <= 0 || tarifRequete <= 0 {
		return 0, errors.New("TARIF_INCONNU : tarification absente (entree/sortie/requete)")
	}
	m := tarifEntree
	if tarifSortie > m {
		m = tarifSortie
	}
	return coutJetons(jetonsTotal, m) + tarifRequete, nil
}

// EstimationCentimesSur : enveloppe monétaire CONSERVATRICE d'une tentative
// potentiellement facturable, en centimes d'euro entiers, calculée AVANT
// tout appel. Chaque retrait possible (429/5xx, jusqu'à retraits_max) peut
// être intégralement facturé par le fournisseur : l'enveloppe compte donc
// (1 + retraits) tentatives intégralement. Tarif ou borne inconnu (<= 0) :
// erreur TARIF_INCONNU, l'appel est refusé par l'orchestrateur (fail
// closed). Cette enveloppe est une ESTIMATION LOCALE fondée sur les tarifs
// et bornes DÉCLARÉS par le pilote : ce n'est PAS une garantie du montant
// réellement facturé par le fournisseur.
func (b *baseHTTP) EstimationCentimesSur() (int, error) {
	return EnveloppeCentimes(b.tarifEntreeMillion, b.tarifSortieMillion, b.tarifRequete,
		b.jetonsEntreeMax, b.jetonsSortieMax, b.retraits)
}

// CoutReelCentimes : coût conservateur calculé À POSTERIORI depuis l'usage
// TOTAL réellement signalé par la réponse : la répartition entrée/sortie
// n'étant pas disponible, le tarif le plus élevé des deux est appliqué (on
// ne sous-compte jamais). Ne sert qu'à enregistrer un ÉVENTUEL surcoût
// dépassant la réservation : le coût constaté ne libère JAMAIS de centimes
// réservés (conservateur par construction, côté orchestrateur).
func (b *baseHTTP) CoutReelCentimes(jetonsTotal int) (int, error) {
	return CoutConservateurJetons(b.tarifEntreeMillion, b.tarifSortieMillion, b.tarifRequete, jetonsTotal)
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func contient(liste []string, v string) bool {
	for _, x := range liste {
		if x == v {
			return true
		}
	}
	return false
}

// parseEndpoint : URL d'endpoint exploitable (fail closed).
func parseEndpoint(endpoint string) (*url.URL, error) {
	return url.Parse(endpoint)
}

// hostIsLoopback : banc d'essai local uniquement (serveurs factices, 0 €).
func hostIsLoopback(hostname string) bool {
	return hostname == "127.0.0.1" || hostname == "localhost" ||
		hostname == "::1" || strings.HasPrefix(hostname, "127.")
}

// schemeNonSecurise : http hors loopback interdit (fail closed).
func schemeNonSecurise(u *url.URL) bool {
	return u.Scheme != "https"
}

// clientHTTP : client HTTP borné par le timeout déclaré (millisecondes).
func clientHTTP(f ConfigFournisseur) *http.Client {
	return &http.Client{Timeout: time.Duration(f.TimeoutMS) * time.Millisecond}
}
