// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Recette SANS MOTEUR du module rail A2A autonome (M-COORD-RAIL-01, T2) :
// le module ne dépend de RIEN de coordengine ; ces tests couvrent sa
// frontière publique (rail + agents factices loopback), 0 €. La recette
// d'intégration orchestrateur (E1..E10, démo E2E) vit côté batterie dans
// coordengine/internal/a2a.
package a2a

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

const jetonModule = "jeton-module-test-01"

// moduleTestDemarre : agent factice + rail sur registre temporaire.
func moduleTestDemarre(t *testing.T, exec a2asrv.AgentExecutor) (*Rail, *ServeurFactice, func()) {
	t.Helper()
	srv, err := DemarrerAgentFactice("agent-module", exec, jetonModule)
	if err != nil {
		t.Fatal(err)
	}
	reg := filepath.Join(t.TempDir(), "rail-registre.json")
	rail, err := NouveauRail(context.Background(), srv.URL(), jetonModule, reg)
	if err != nil {
		srv.Fermer()
		t.Fatal(err)
	}
	return rail, srv, srv.Fermer
}

// M1 : délégation nominale — tâche durable exigée, registre persistant,
// résultat capturé à l'envoi, enveloppe {jetons, texte} strictement validée.
func TestModuleDelegationNominale(t *testing.T) {
	rail, _, fermer := moduleTestDemarre(t, ExecEchoFactice{})
	defer fermer()
	e, _, err := rail.Deleguer(context.Background(), "executer", "ECHO:CHARGE", "cle-m1")
	if err != nil {
		t.Fatal(err)
	}
	if e.Etat != EtatAchevee || !e.Resolu {
		t.Fatalf("tâche achevée résolue attendue : %+v", e)
	}
	if e.Jetons != JetonsFactices || !strings.HasPrefix(e.Texte, "ECHO:") {
		t.Fatalf("artefact echo attendu : %+v", e)
	}
	raw, err := os.ReadFile(rail.Chemin())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"resolu":true`) {
		t.Fatalf("registre durable attendu : %s", raw)
	}
}

// M2 : annulation des deux côtés — saisie attendue exigée pour annuler,
// état canceled EXIGÉ, aucun résultat durable d'une tâche annulée.
func TestModuleAnnulation(t *testing.T) {
	rail, _, fermer := moduleTestDemarre(t, ExecEchoFactice{})
	defer fermer()
	e, _, err := rail.Deleguer(context.Background(), "executer", "PAUSE", "cle-m2")
	if err != nil {
		t.Fatal(err)
	}
	if e.Etat != EtatSaisieAttendue {
		t.Fatalf("input-required attendu : %s", e.Etat)
	}
	if _, ok := rail.ResultatDurable(e.Tache); ok {
		t.Fatal("résultat durable inattendu pour une tâche non résolue")
	}
	etat, err := rail.Annuler(context.Background(), e.Tache)
	if err != nil || etat != EtatAnnulee {
		t.Fatalf("annulation conforme attendue : %v / %s", err, etat)
	}
	if _, ok := rail.ResultatDurable(e.Tache); ok {
		t.Fatal("un résultat durable d'une tâche annulée doit être refusé")
	}
}

// M3 : doublon — une clé ne délègue JAMAIS deux fois, aucun réseau au
// second appel.
func TestModuleDoublonRefuse(t *testing.T) {
	rail, _, fermer := moduleTestDemarre(t, ExecEchoFactice{})
	if _, _, err := rail.Deleguer(context.Background(), "executer", "ECHO:X", "cle-m3"); err != nil {
		fermer()
		t.Fatal(err)
	}
	fermer() // agent TUÉ : un second appel réseau échouerait forcément.
	e2, doublon, err := rail.Deleguer(context.Background(), "executer", "ECHO:X", "cle-m3")
	if !doublon || !errors.Is(err, ErrDoublon) {
		t.Fatalf("doublon refusé attendu : %v", err)
	}
	if e2 == nil || e2.Tache == "" {
		t.Fatalf("l'enregistrement initial doit être retourné : %+v", e2)
	}
}

// M4 : mapping fermé — 6 états connus, tout état inconnu REFUSÉ.
func TestModuleMappingFerme(t *testing.T) {
	if _, err := EtatDepuisA2A("TASK_STATE_IMPOSSIBLE"); err == nil {
		t.Fatal("état inconnu refusé attendu")
	}
}

// M5 : sécurité — jeton exigé (401), carte publique servie, loopback
// imposé, jeton absent refusé, registre 0600 sans secret.
func TestModuleSecurite(t *testing.T) {
	srv, err := DemarrerAgentFactice("agent-m5", ExecEchoFactice{}, jetonModule)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Fermer()
	rep, err := http.Post(srv.URL()+"/invoke", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if rep.StatusCode != http.StatusUnauthorized {
		t.Fatalf("401 attendu sans jeton : %d", rep.StatusCode)
	}
	rep3, err := http.Get(srv.URL() + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatal(err)
	}
	if rep3.StatusCode != http.StatusOK {
		t.Fatalf("carte A2A servie attendue : %d", rep3.StatusCode)
	}
	// Loopback IMPOSÉ : agent distant refusé au chargement.
	if _, err := NouveauRail(context.Background(), "https://agent.example.com", "j",
		t.TempDir()+"/r.json"); err == nil ||
		!strings.Contains(err.Error(), "APPEL_REEL_NON_AUTORISE") {
		t.Fatalf("loopback imposé attendu : %v", err)
	}
	// Jeton absent : refus.
	if _, err := NouveauRail(context.Background(), srv.URL(), "  ", t.TempDir()+"/r.json"); err == nil {
		t.Fatal("jeton absent refusé attendu")
	}
	reg := filepath.Join(t.TempDir(), "rail-m5.json")
	rail, err := NouveauRail(context.Background(), srv.URL(), jetonModule, reg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := rail.Deleguer(context.Background(), "executer", "ECHO:P", "cle-m5"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(rail.Chemin())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("registre 0600 attendu : %v", info.Mode().Perm())
	}
	raw, _ := os.ReadFile(rail.Chemin())
	if strings.Contains(string(raw), jetonModule) {
		t.Fatal("le jeton ne doit JAMAIS figurer dans le registre")
	}
}

// M6 : reprise après arrêt réel — registre rechargé par un nouveau rail,
// résultat durable conservé, agent jamais exécuté deux fois, résolution
// pilote exclusive des tâches incertaines.
func TestModuleRepriseEtResolutionPilote(t *testing.T) {
	var n atomic.Int32
	exec := &CompteurModule{inner: ExecEchoFactice{}, n: &n}
	srv, err := DemarrerAgentFactice("agent-m6", exec, jetonModule)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Fermer()
	reg := filepath.Join(t.TempDir(), "rail-m6.json")
	rail1, err := NouveauRail(context.Background(), srv.URL(), jetonModule, reg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := rail1.Deleguer(context.Background(), "executer", "ECHO:R", "cle-m6"); err != nil {
		t.Fatal(err)
	}
	// == Arrêt réel du processus client : seul le registre durable subsiste. ==
	rail2, err := NouveauRail(context.Background(), srv.URL(), jetonModule, reg)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := rail2.ResultatParCle("cle-m6"); !ok || !e.Resolu || !strings.HasPrefix(e.Texte, "ECHO:") {
		t.Fatalf("résultat durable conservé attendu : %+v", e)
	}
	if _, doublon, errD := rail2.Deleguer(context.Background(), "executer", "ECHO:R", "cle-m6"); !doublon || !errors.Is(errD, ErrDoublon) {
		t.Fatalf("doublon refusé après reprise attendu : %v", errD)
	}
	if n.Load() != 1 {
		t.Fatalf("l'agent n'a JAMAIS exécuté deux fois : %d", n.Load())
	}
	// Tâche incertaine : résolution PILOTE EXCLUSIVE, clé inconnue refusée.
	if err := rail2.ResoudrePilote("cle-inconnue", "x", 1); err == nil {
		t.Fatal("clé inconnue refusée attendue")
	}
}

// CompteurModule : exécuteur comptant les messages/send (doublons).
type CompteurModule struct {
	inner a2asrv.AgentExecutor
	n     *atomic.Int32
}

func (c *CompteurModule) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	c.n.Add(1)
	return c.inner.Execute(ctx, ec)
}

func (c *CompteurModule) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return c.inner.Cancel(ctx, ec)
}
