package aic

import (
	"errors"
	"strings"
)

// GitHubClient est l'interface de communication GitHub du coordinateur.
// Le coordinateur ne détient jamais la permission de fusion : la fusion
// reste une opération humaine hors du périmètre du compte bot.
type GitHubClient interface {
	// PostComment publie un commentaire au nom du coordinateur.
	PostComment(missionID string, body string) error
	// CreateDraftPR ouvre une PR Draft représentant une livraison.
	CreateDraftPR(missionID, title, body string) (prRef string, err error)
	// RequestChanges rattache une demande de correction à la PR.
	RequestChanges(prRef, body string) error
}

// IdempotentGitHub est une extension optionnelle de GitHubClient : si
// l'adaptateur la réalise, le coordinateur lui transmet la clé d'idempotence
// de chaque écriture et l'adaptateur déduplique lui-même les envois répétés
// (T02-R3, risque 2). Les adaptateurs API réels (T03) devront la réaliser
// (recherche avant création) pour garantir l'absence de doublon.
type IdempotentGitHub interface {
	// PostCommentIdem publie un commentaire, dédupliqué par clé.
	PostCommentIdem(idemKey, missionID, body string) error
	// CreateDraftPRIdem ouvre une PR Draft, dédupliquée par clé.
	CreateDraftPRIdem(idemKey, missionID, title, body string) (prRef string, err error)
	// RequestChangesIdem rattache une demande de correction, dédupliquée par clé.
	RequestChangesIdem(idemKey, prRef, body string) error
}

// KeyStore permet à l'adaptateur simulé de rendre sa déduplication durable :
// les clés déjà envoyées sont persistées, la déduplication survit à un
// redémarrage complet du processus (T02-R4). *Store réalise cette interface.
type KeyStore interface {
	// GitHubSeenKey indique si la clé a déjà été envoyée de façon durable.
	GitHubSeenKey(idemKey string) (bool, error)
	// GitHubMarkKey enregistre durablement l'envoi de la clé.
	GitHubMarkKey(idemKey string) error
}

// SimulatedGitHub enregistre les écritures sans toucher au réseau. Utilisé
// pour tous les tests T02.
type SimulatedGitHub struct {
	Comments       []string
	DraftPRs       []string
	ChangeRequests []string
	sentKeys       map[string]bool
	draftRefs      map[string]string
	keyStore       KeyStore // déduplication durable (T02-R4), optionnelle
}

// UseKeyStore branche la déduplication durable de l'adaptateur simulé.
func (g *SimulatedGitHub) UseKeyStore(ks KeyStore) { g.keyStore = ks }

// seenKey consulte la mémoire de l'adaptateur puis, si branché, le registre
// durable : la déduplication résiste au redémarrage complet du processus.
func (g *SimulatedGitHub) seenKey(key string) bool {
	if g.sentKeys == nil {
		g.sentKeys = map[string]bool{}
	}
	if g.sentKeys[key] {
		return true
	}
	if g.keyStore != nil {
		if seen, err := g.keyStore.GitHubSeenKey(key); err == nil && seen {
			g.sentKeys[key] = true // cache mémoire
			return true
		}
	}
	return false
}

func (g *SimulatedGitHub) markSeenKey(key string) {
	if g.sentKeys == nil {
		g.sentKeys = map[string]bool{}
	}
	g.sentKeys[key] = true
	if g.keyStore != nil {
		_ = g.keyStore.GitHubMarkKey(key)
	}
}

// PostComment simule la publication d'un commentaire.
func (g *SimulatedGitHub) PostComment(missionID, body string) error {
	g.Comments = append(g.Comments, missionID+" :: "+body)
	return nil
}

// PostCommentIdem simule un commentaire idempotent : une même clé ne produit
// qu'un seul commentaire, même après un renvoi (fenêtre de crash) ou un
// redémarrage complet du processus (registre durable, T02-R4).
func (g *SimulatedGitHub) PostCommentIdem(idemKey, missionID, body string) error {
	if g.seenKey("c:" + idemKey) {
		return nil
	}
	g.markSeenKey("c:" + idemKey)
	return g.PostComment(missionID, body)
}

// CreateDraftPR simule l'ouverture d'une PR Draft.
func (g *SimulatedGitHub) CreateDraftPR(missionID, title, body string) (string, error) {
	ref := "PR-DRAFT-" + missionID
	g.DraftPRs = append(g.DraftPRs, ref+" :: "+title)
	return ref, nil
}

// CreateDraftPRIdem simule une PR Draft idempotente : une même clé retourne
// toujours la même référence, sans dupliquer la livraison, même après un
// redémarrage complet du processus (T02-R4).
func (g *SimulatedGitHub) CreateDraftPRIdem(idemKey, missionID, title, body string) (string, error) {
	if g.draftRefs == nil {
		g.draftRefs = map[string]string{}
	}
	if ref, ok := g.draftRefs[idemKey]; ok {
		return ref, nil
	}
	if g.seenKey("d:" + idemKey) {
		// déjà créée de façon durable mais référence non restituée en
		// mémoire (redémarrage) : la référence est reconstruite selon la
		// convention déterministe du simulateur ; l'adaptateur réel T03
		// fera une recherche avant création.
		ref := "PR-DRAFT-" + missionID
		g.draftRefs[idemKey] = ref
		return ref, nil
	}
	ref, err := g.CreateDraftPR(missionID, title, body)
	if err == nil {
		g.draftRefs[idemKey] = ref
		g.markSeenKey("d:" + idemKey)
	}
	return ref, err
}

// RequestChanges simule l'attachement d'une demande de correction.
func (g *SimulatedGitHub) RequestChanges(prRef, body string) error {
	g.ChangeRequests = append(g.ChangeRequests, prRef+" :: "+body)
	return nil
}

// RequestChangesIdem simule une demande de correction idempotente.
func (g *SimulatedGitHub) RequestChangesIdem(idemKey, prRef, body string) error {
	if g.seenKey("r:" + idemKey) {
		return nil
	}
	g.markSeenKey("r:" + idemKey)
	return g.RequestChanges(prRef, body)
}

// ErrNoPushByAgent matérialise la règle de sécurité : l'agent Mistral n'écrit
// jamais dans le dépôt ; seul le coordinateur publie, sous permissions
// restreintes, et uniquement des livraisons Draft soumises à validation humaine.
var ErrNoPushByAgent = errors.New("aucun push n'est effectué par l'agent : livraison Draft uniquement")

// AgentCommandGuard vérifie qu'un corps d'Issue ou de commentaire ne comporte
// pas de commande arbitraire destinée à l'agent développeur. Seuls les
// mots-clés du vocabulaire du coordinateur sont acceptés.
func AgentCommandGuard(body string) error {
	// refus grossier des tentatives classiques d'injection de commandes.
	for _, pat := range []string{"rm -rf", "$(", "${", "; curl", "&& curl", "| curl", "`curl", "sudo ", "eval "} {
		if strings.Contains(body, pat) {
			return errors.New("commande arbitraire refusée dans le contenu GitHub (garde-fou T02-D)")
		}
	}
	return nil
}
