// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// TI-06b (M-COORD-RAIL-01, ordre T4) : interruption RÉELLE du processus
// pendant un appel en vol. Le binaire de test est RELANCÉ en processus
// enfant : il effectue une délégation (EN_COURS persisté au registre
// durable) puis s'endort ; le parent le tue BRUTALEMENT (SIGKILL /
// TerminateProcess : AUCUN nettoyage, aucun finalizer, crash réel) et
// ROUVRE le registre depuis un nouveau Rail. Failles impossibles à ce
// niveau : persistance atomique du registre, tâche incertaine
// identifiable, doublon refusé SANS réseau, résolution PILOTE exclusive.
package a2a

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	probeMarqueurSucces = "PROBE_DELEGE"
	probeCle            = "cle-ti06b"
)

// TestModuleProbeSousProcessus : mode ENFANT — exécuté uniquement via
// l'environnement COORD_A2A_PROBE=1 posé par le parent. Le binaire de
// test se réexécute lui-même : la délégation est RÉELLE (agent du parent
// atteint en loopback), le registre persisté, puis le processus dort
// jusqu'à être tué.
func TestModuleProbeSousProcessus(t *testing.T) {
	if os.Getenv("COORD_A2A_PROBE") != "1" {
		t.Skip("mode parent : le probe n'est actif que via COORD_A2A_PROBE=1")
	}
	reg := os.Getenv("COORD_A2A_PROBE_REGISTRE")
	rail, err := NouveauRail(context.Background(),
		os.Getenv("COORD_A2A_PROBE_URL"), os.Getenv("COORD_A2A_PROBE_JETON"), reg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe : rail : "+err.Error())
		os.Exit(3)
	}
	if _, _, err := rail.Deleguer(context.Background(), "executer", "PAUSE", probeCle); err != nil {
		fmt.Fprintln(os.Stderr, "probe : delegation : "+err.Error())
		os.Exit(4)
	}
	// marque + flux non bufferisés : le parent lit puis tue.
	fmt.Println(probeMarqueurSucces)
	os.Stdout.Sync()
	time.Sleep(30 * time.Second) // tué bien avant
}

// TestModuleInterruptionReelleDuProcessus : mode PARENT — tue l'enfant
// par SIGKILL pendant la vol, puis vérifie la COHÉRENCE du registre
// durable et le comportement fail-closed de la reprise.
func TestModuleInterruptionReelleDuProcessus(t *testing.T) {
	if os.Getenv("COORD_A2A_PROBE") == "1" {
		return // enfant : ne pas exécuter le parent dans le même binaire
	}
	var n atomic.Int32
	srv, err := DemarrerAgentFactice("agent-ti06b",
		CompteurEnvois{Inner: ExecEchoFactice{}, N: &n}, jetonModule)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Fermer()
	reg := filepath.Join(t.TempDir(), "rail-ti06b.json")

	cmd := exec.Command(os.Args[0], "-test.run=TestModuleProbeSousProcessus", "-test.timeout=60s")
	cmd.Env = append(os.Environ(),
		"COORD_A2A_PROBE=1",
		"COORD_A2A_PROBE_URL="+srv.URL(),
		"COORD_A2A_PROBE_JETON="+jetonModule,
		"COORD_A2A_PROBE_REGISTRE="+reg,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("lancement du processus enfant : %v", err)
	}
	sc := bufio.NewScanner(stdout)
	luOk := false
	for sc.Scan() {
		if strings.Contains(sc.Text(), probeMarqueurSucces) {
			luOk = true
			break
		}
	}
	if !luOk {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("l'enfant n'a pas signalé sa délégation (sortie : %q)", sc.Err())
	}
	// == Interruption RÉELLE : SIGKILL, aucun nettoyage possible. ==
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill : %v", err)
	}
	_ = cmd.Wait()

	// Registre rouvert par un NOUVEAU rail (nouvel état mémoire : équivalent
	// strict du redémarrage du processus après crash).
	rail2, err := NouveauRail(context.Background(), srv.URL(), jetonModule, reg)
	if err != nil {
		t.Fatalf("rouverture du registre après crash : %v", err)
	}
	e, ok := rail2.ResultatParCle(probeCle)
	if !ok {
		t.Fatal("la tâche du crash doit exister au registre durable")
	}
	if e.Resolu {
		t.Fatalf("la tâche interrompue ne doit PAS être résolue : %+v", e)
	}
	if len(rail2.NonResolues()) != 1 {
		t.Fatalf("une tâche incertaine attendue : %+v", rail2.NonResolues())
	}
	// Doublon refusé SANS re-réseau (l'agent n'a envoyé qu'UN message/send).
	if _, doublon, errD := rail2.Deleguer(context.Background(), "executer", "PAUSE", probeCle); !doublon || !errors.Is(errD, ErrDoublon) {
		t.Fatalf("doublon refusé après crash attendu : %v", errD)
	}
	if n.Load() != 1 {
		t.Fatalf("l'agent n'a jamais exécuté deux fois : %d", n.Load())
	}
	// Résolution PILOTE EXCLUSIVE de la tâche incertaine.
	if err := rail2.ResoudrePilote(probeCle, "RESULTAT-APRES-CRASH-PILOTE", 111); err != nil {
		t.Fatalf("résolution pilote : %v", err)
	}
	e, ok = rail2.ResultatParCle(probeCle)
	if !ok || !e.Resolu || !e.Manuel || e.Texte != "RESULTAT-APRES-CRASH-PILOTE" {
		t.Fatalf("résolution manuelle attendue : %+v", e)
	}
}
