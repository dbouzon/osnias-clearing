// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Tests du gateway coord-front (mission M-COORD-FRONT-01) : session et
// authentification, scénario démo COMPLET (création → dialogue → ordre
// → devis → REFUS puis ACCORD → validation → publication →
// transmission → mission fille), isolation inter-comptes, proxy réel
// avec injection du token pilote (jamais renvoyé au navigateur),
// adaptateur Drive factice, et parité i18n des 5 langues du front.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func configTest(comptes ...compteConf) config {
	return config{SecretSession: "secret-de-test-0123456789", Comptes: comptes}
}

func gatewayTest(t *testing.T, cfg config) (*gateway, *httptest.Server) {
	t.Helper()
	g := newGateway(cfg, ".")
	srv := httptest.NewServer(g.routes())
	t.Cleanup(srv.Close)
	return g, srv
}

type client struct {
	base  string
	token string // valeur du cookie de session
}

func (c *client) req(t *testing.T, methode, chemin string, corps any) (int, map[string]any, string) {
	t.Helper()
	var body io.Reader
	if corps != nil {
		b, err := json.Marshal(corps)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(methode, c.base+chemin, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.AddCookie(&http.Cookie{Name: cookieSession, Value: c.token})
	}
	rep, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	raw, _ := io.ReadAll(rep.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return rep.StatusCode, m, string(raw)
}

func connecter(t *testing.T, srv *httptest.Server, nom string) *client {
	t.Helper()
	c := &client{base: srv.URL}
	rep, err := http.Post(srv.URL+"/api/connexion",
		"application/json", strings.NewReader(`{"compte":"`+nom+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	raw, _ := io.ReadAll(rep.Body)
	if rep.StatusCode != 200 {
		t.Fatalf("connexion %s : statut %d %s", nom, rep.StatusCode, raw)
	}
	for _, ck := range rep.Cookies() {
		if ck.Name == cookieSession {
			c.token = ck.Value
		}
	}
	if c.token == "" {
		t.Fatal("connexion : aucun cookie de session reçu")
	}
	if strings.Contains(string(raw), "jeton-") {
		t.Fatal("fuite de token pilote dans /api/connexion")
	}
	return c
}

func TestConnexionEtAuthentification(t *testing.T) {
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
	))
	anonyme := &client{base: srv.URL}
	if s, _, _ := anonyme.req(t, "GET", "/api/projets", nil); s != 401 {
		t.Fatalf("sans session, /api/projets doit répondre 401, reçu %d", s)
	}
	if s, _, _ := anonyme.req(t, "GET", "/api/session", nil); s != 401 {
		t.Fatalf("sans session, /api/session doit répondre 401, reçu %d", s)
	}
	if s, _, _ := anonyme.req(t, "POST", "/api/connexion", map[string]any{"compte": "inconnu"}); s != 401 {
		t.Fatalf("compte inconnu doit répondre 401, reçu %d", s)
	}
	c := connecter(t, srv, "alice")
	if s, m, _ := c.req(t, "GET", "/api/session", nil); s != 200 || m["demo"] != true {
		t.Fatalf("session alice attendue {demo:true}, reçu %d %v", s, m)
	}
}

func TestScenarioDemoComplet(t *testing.T) {
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
	))
	c := connecter(t, srv, "alice")

	// Garde-fous création : intention vide refusée, doublon refusé.
	if s, m, _ := c.req(t, "POST", "/api/projets", map[string]any{"id": "x", "intention": " "}); s != 400 || m["code"] != "INTENTION_VIDE" {
		t.Fatalf("intention vide : attendu 400 INTENTION_VIDE, reçu %d %v", s, m)
	}
	s, p, _ := c.req(t, "POST", "/api/projets", map[string]any{"id": "etude-marche", "intention": "Étudier le marché agroalimentaire français"})
	if s != 201 || p["etat"] != etatIntention {
		t.Fatalf("création : attendu 201 INTENTION, reçu %d %v", s, p)
	}
	if s, m, _ := c.req(t, "POST", "/api/projets", map[string]any{"id": "etude-marche", "intention": "bis"}); s != 409 || m["code"] != "PROJET_EXISTANT" {
		t.Fatalf("doublon : attendu 409 PROJET_EXISTANT, reçu %d %v", s, m)
	}

	// Binôme identique refusé, dialogue ouvert avec binôme distinct.
	if s, m, _ := c.req(t, "POST", "/api/projets/etude-marche/dialogue",
		map[string]any{"directeur": "dir", "executant": "dir"}); s != 400 || m["code"] != "BINOME_IDENTIQUE" {
		t.Fatalf("binôme identique : attendu 400, reçu %d %v", s, m)
	}
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/dialogue",
		map[string]any{"directeur": "openai-dir", "executant": "openai-exec"}); s != 200 || p["etat"] != etatDialogue {
		t.Fatalf("dialogue : attendu 200 DIALOGUE, reçu %d %v", s, p)
	}

	// Trois tours : QUESTION, CLARIFICATION, PROPOSITION_ORDRE.
	s, rep, _ := c.req(t, "POST", "/api/projets/etude-marche/tour", map[string]any{"entree": "Un rapport sur le marché."})
	r := rep["reponse"].(map[string]any)
	if s != 200 || r["action"] != "QUESTION" {
		t.Fatalf("tour 1 : attendu QUESTION, reçu %d %v", s, r)
	}
	_, rep, _ = c.req(t, "POST", "/api/projets/etude-marche/tour", map[string]any{"entree": "Le secteur agroalimentaire."})
	r = rep["reponse"].(map[string]any)
	if r["action"] != "CLARIFICATION" {
		t.Fatalf("tour 2 : attendu CLARIFICATION, reçu %v", r)
	}
	_, rep, _ = c.req(t, "POST", "/api/projets/etude-marche/tour", map[string]any{"entree": "Un rapport markdown de 5 pages."})
	r = rep["reponse"].(map[string]any)
	p = rep["projet"].(map[string]any)
	if r["action"] != "PROPOSITION_ORDRE" || p["etat"] != etatOrdrePropose {
		t.Fatalf("tour 3 : attendu PROPOSITION_ORDRE + ORDRE_PROPOSE, reçu %v %v", r, p["etat"])
	}

	// Retour au dialogue avec motif, re-proposition (script déterministe).
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/retour-dialogue",
		map[string]any{"motif": "périmètre à affiner"}); s != 200 || p["etat"] != etatDialogue {
		t.Fatalf("retour dialogue : attendu DIALOGUE, reçu %d %v", s, p["etat"])
	}
	_, rep, _ = c.req(t, "POST", "/api/projets/etude-marche/tour", map[string]any{"entree": "avec une annexe chiffres"})
	_, rep, _ = c.req(t, "POST", "/api/projets/etude-marche/tour", map[string]any{"entree": "c'est bon"})
	_, rep, _ = c.req(t, "POST", "/api/projets/etude-marche/tour", map[string]any{"entree": "proposez"})
	p = rep["projet"].(map[string]any)
	if p["etat"] != etatOrdrePropose {
		t.Fatalf("re-proposition : attendu ORDRE_PROPOSE, reçu %v", p["etat"])
	}

	// Qualification : version v1 figée + empreinte.
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/qualifier", nil); s != 200 || p["etat"] != etatOrdreQualifie {
		t.Fatalf("qualification : attendu ORDRE_QUALIFIE, reçu %d %v", s, p["etat"])
	}
	if p["version"] != float64(1) || p["empreinte_ordre"] == "" {
		t.Fatalf("qualification : version/empreinte manquantes : %v %v", p["version"], p["empreinte_ordre"])
	}

	// Devis dérivé ; validation AVANT accord refusée par le front, mais
	// le moteur démo, comme le moteur réel, exige le jeton explicite.
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/devis", nil); s != 200 || p["devis"] == nil {
		t.Fatalf("devis : attendu 200 avec devis, reçu %d", s)
	}
	if s, m, _ := c.req(t, "POST", "/api/projets/etude-marche/valider", map[string]any{"jeton_action": ""}); s != 400 || m["code"] != "JETON_PILOTE_ABSENT" {
		t.Fatalf("validation sans jeton : attendu 400 JETON_PILOTE_ABSENT, reçu %d %v", s, m)
	}

	// REFUS explicite : la mission reste non exécutée.
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/accord-devis", map[string]any{"decision": "REFUS"}); s != 200 {
		t.Fatalf("refus devis : statut %d", s)
	}
	if p["accord_devis"] != false || p["etat"] != etatOrdreQualifie {
		t.Fatalf("refus devis : état inattendu %v %v", p["accord_devis"], p["etat"])
	}

	// ACCORD explicite, puis exécution pilote complète.
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/accord-devis", map[string]any{"decision": "ACCORD"}); s != 200 || p["accord_devis"] != true {
		t.Fatalf("accord devis : attendu accord_devis=true, reçu %d %v", s, p["accord_devis"])
	}
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/valider", map[string]any{"jeton_action": "pilote-demo"}); s != 200 || p["etat"] != etatValide {
		t.Fatalf("validation : attendu VALIDE, reçu %d %v", s, p["etat"])
	}
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/publier", nil); s != 200 || p["etat"] != etatPublie {
		t.Fatalf("publication : attendu PUBLIE, reçu %d %v", s, p["etat"])
	}
	if s, p, _ = c.req(t, "POST", "/api/projets/etude-marche/transmettre", nil); s != 200 || p["etat"] != etatTransmis {
		t.Fatalf("transmission : attendu TRANSMIS, reçu %d %v", s, p["etat"])
	}
	s, m, _ := c.req(t, "POST", "/api/projets/etude-marche/transmettre", nil)
	if s != 409 || m["code"] != "OPERATION_DEJA_EFFECTUEE" {
		t.Fatalf("double transmission : attendu 409 OPERATION_DEJA_EFFECTUEE, reçu %d %v", s, m)
	}

	// Mission fille lisible uniquement après transmission.
	if s, m, _ = c.req(t, "GET", "/api/projets/etude-marche/fille", nil); s != 200 || m["issue"] != "etude-marche-EXEC" {
		t.Fatalf("mission fille : attendu etude-marche-EXEC, reçu %d %v", s, m)
	}

	// Journal en ndjson : au moins une entrée par étape clé.
	req, _ := http.NewRequest("GET", srv.URL+"/api/projets/etude-marche/journal", nil)
	req.AddCookie(&http.Cookie{Name: cookieSession, Value: c.token})
	repHTTP, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer repHTTP.Body.Close()
	raw, _ := io.ReadAll(repHTTP.Body)
	for _, attendu := range []string{"DECLARATION_INTENTION", "PROPOSITION_ORDRE", "QUALIFICATION", "ACCORD_DEVIS", "TRANSMISSION"} {
		if !strings.Contains(string(raw), attendu) {
			t.Fatalf("journal : entrée %s absente", attendu)
		}
	}

	// Projet inconnu : 404 PROJET_INCONNU (forme du moteur réel).
	if s, m, _ := c.req(t, "GET", "/api/projets/absent", nil); s != 404 || m["code"] != "PROJET_INCONNU" {
		t.Fatalf("projet inconnu : attendu 404 PROJET_INCONNU, reçu %d %v", s, m)
	}
}

func TestIsolationInterComptes(t *testing.T) {
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
		compteConf{Nom: "bob", Demo: true, DossierDrive: t.TempDir()},
	))
	a := connecter(t, srv, "alice")
	b := connecter(t, srv, "bob")
	if s, _, _ := a.req(t, "POST", "/api/projets", map[string]any{"id": "mission-a", "intention": "intention A"}); s != 201 {
		t.Fatalf("création alice : statut %d", s)
	}
	// bob NE DOIT PAS voir le projet d'alice (moteur démo par compte).
	if s, m, _ := b.req(t, "GET", "/api/projets/mission-a", nil); s != 404 || m["code"] != "PROJET_INCONNU" {
		t.Fatalf("isolation rompue : bob voit mission-a (%d %v)", s, m)
	}
	s, m, _ := b.req(t, "GET", "/api/projets", nil)
	if s != 200 {
		t.Fatalf("listage bob : statut %d", s)
	}
	if liste, _ := m["projets"].([]any); len(liste) != 0 {
		t.Fatalf("isolation : la liste de bob n'est pas vide : %v", m["projets"])
	}
	// Session forgée (cookie signé invalide) : refus.
	faux := &client{base: srv.URL, token: "alice.faux"}
	if s, _, _ := faux.req(t, "GET", "/api/projets", nil); s != 401 {
		t.Fatalf("cookie forgé : attendu 401, reçu %d", s)
	}
}

func TestProxyReelInjectionToken(t *testing.T) {
	// Instance factice : exige le token pilote, l'écho permet de
	// vérifier le chemin et le corps transmis tels quels.
	const tokenPilote = "jeton-super-secret-pilote"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Coord-Pilot-Token") != tokenPilote {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"AUTH_REFUSEE","motif":"token absent ou invalide"}`))
			return
		}
		if r.URL.Path != "/projets/mission-x" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"mission-x","etat":"DIALOGUE","echo":true}`))
	}))
	defer backend.Close()
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "carole", URL: backend.URL, Token: tokenPilote},
	))
	c := connecter(t, srv, "carole")
	s, m, raw := c.req(t, "GET", "/api/projets/mission-x", nil)
	if s != 200 || m["etat"] != "DIALOGUE" {
		t.Fatalf("proxy réel : attendu 200 DIALOGUE, reçu %d %s", s, raw)
	}
	if strings.Contains(raw, tokenPilote) {
		t.Fatal("FUITE : le token pilote est revenu au navigateur")
	}
	// Sans token dans le config (impossible au démarrage, simulé
	// directement) : la couche session répond 401 AVANT le proxy.
	anonyme := &client{base: srv.URL}
	if s, _, _ = anonyme.req(t, "GET", "/api/projets/mission-x", nil); s != 401 {
		t.Fatalf("proxy sans session : attendu 401, reçu %d", s)
	}
}

func TestDriveFactice(t *testing.T) {
	racine := t.TempDir()
	d := nouveauDossierFactice(racine)
	if err := d.Ecrire("coord", "note.txt", []byte("bonjour")); err != nil {
		t.Fatal(err)
	}
	noms, err := d.Lister("coord")
	if err != nil || !reflect.DeepEqual(noms, []string{"note.txt"}) {
		t.Fatalf("lister : %v %v", noms, err)
	}
	contenu, err := d.Lire("coord", "note.txt")
	if err != nil || string(contenu) != "bonjour" {
		t.Fatalf("lire : %q %v", contenu, err)
	}
	// Fail closed : évasion hors racine refusée.
	if err := d.Ecrire("../../etc", "evil", nil); err == nil {
		t.Fatal("évasion hors racine non refusée")
	}
	// Dossier inexistant : liste vide, pas d'erreur.
	vide, err := d.Lister("inexistant")
	if err != nil || len(vide) != 0 {
		t.Fatalf("dossier inexistant : %v %v", vide, err)
	}
	// Via l'API : session requise, liste lisible.
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: racine},
	))
	c := connecter(t, srv, "alice")
	s, m, _ := c.req(t, "GET", "/api/drive/?dossier=coord", nil)
	if s != 200 || m["documents"].([]any)[0] != "note.txt" {
		t.Fatalf("api drive : attendu note.txt, reçu %d %v", s, m)
	}
	anonyme := &client{base: srv.URL}
	if s, _, _ = anonyme.req(t, "GET", "/api/drive/?dossier=coord", nil); s != 401 {
		t.Fatalf("api drive sans session : attendu 401, reçu %d", s)
	}
}

// TestLanguesParite : les 5 fichiers de traduction du front doivent
// porter EXACTEMENT le même jeu de clés (fr, en, de, es, zh-Hans).
// Le format est contrôlé : CoordLangues["xx"] = { JSON strict } ;
func TestLanguesParite(t *testing.T) {
	_, fichier, _, _ := runtime.Caller(0)
	racineFront := filepath.Join(filepath.Dir(fichier), "..", "..", "..", "front", "js", "langues")
	attendu := []string{"fr", "en", "de", "es", "zh"}
	cles := map[string]map[string]bool{}
	noms, err := os.ReadDir(racineFront)
	if err != nil {
		t.Skip("dossier langues absent du bac à sable de test Go")
	}
	for _, e := range noms {
		nom := strings.TrimSuffix(e.Name(), ".js")
		if !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		found := false
		for _, l := range attendu {
			if l == nom {
				found = true
			}
		}
		if !found {
			t.Fatalf("langue inattendue : %s", e.Name())
		}
		raw, err := os.ReadFile(filepath.Join(racineFront, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		texte := string(raw)
		/* Les lignes de commentaire d'en-tête (//) sont ignorées ;
		 * l'affectation doit suivre : CoordLangues["xx"] = { JSON }; */
		lignes := strings.Split(texte, "\n")
		code := ""
		for _, l := range lignes {
			if strings.HasPrefix(strings.TrimSpace(l), "//") {
				continue
			}
			code += l + "\n"
		}
		code = strings.TrimSpace(code)
		prefixe := `CoordLangues["` + nom + `"] = `
		if !strings.HasPrefix(code, prefixe) || !strings.HasSuffix(code, ";") {
			t.Fatalf("%s : en-tête ou point-virgule final manquant", e.Name())
		}
		corps := strings.TrimSpace(strings.TrimSuffix(code, ";"))
		corps = strings.TrimSpace(strings.TrimPrefix(corps, prefixe))
		var m map[string]any
		if err := json.Unmarshal([]byte(corps), &m); err != nil {
			t.Fatalf("%s : le corps doit être du JSON strict : %v", e.Name(), err)
		}
		cles[nom] = map[string]bool{}
		for k := range m {
			cles[nom][k] = true
		}
	}
	if len(cles) != len(attendu) {
		t.Fatalf("langues manquantes : %v", cles)
	}
	reference := cles["fr"]
	for _, l := range attendu[1:] {
		manquantes, enTrop := []string{}, []string{}
		for k := range reference {
			if !cles[l][k] {
				manquantes = append(manquantes, k)
			}
		}
		for k := range cles[l] {
			if !reference[k] {
				enTrop = append(enTrop, k)
			}
		}
		sort.Strings(manquantes)
		sort.Strings(enTrop)
		if len(manquantes) > 0 || len(enTrop) > 0 {
			t.Fatalf("%s : clés manquantes %v, clés en trop %v", l, manquantes, enTrop)
		}
	}
}

/* ================== M-COORD-DRIVE-01 (plan v2 §2) ================== */

// scenarioJusquaTransmission : parcours démo complet jusqu'à l'état
// TRANSMIS (cadrage optionnel pour l'export du dossier de sortie).
func scenarioJusquaTransmission(t *testing.T, c *client, id, dossierSortie string) {
	t.Helper()
	if s, _, _ := c.req(t, "POST", "/api/projets", map[string]any{
		"id": id, "intention": "Étudier le marché agroalimentaire",
	}); s != 201 {
		t.Fatalf("création : statut %d", s)
	}
	if dossierSortie != "" {
		if s, _, _ := c.req(t, "POST", "/api/projets/"+id+"/cadrage", map[string]any{
			"dossier_sortie": dossierSortie,
		}); s != 200 {
			t.Fatalf("cadrage : statut %d", s)
		}
	}
	for _, action := range []struct {
		chemin string
		corps  any
	}{
		{"/dialogue", map[string]any{"directeur": "openai-dir", "executant": "openai-exec"}},
		{"/tour", map[string]any{"entree": "Un rapport sur le marché."}},
		{"/tour", map[string]any{"entree": "Le secteur agroalimentaire."}},
		{"/tour", map[string]any{"entree": "Un rapport markdown de 5 pages."}},
		{"/qualifier", nil},
		{"/devis", nil},
		{"/accord-devis", map[string]any{"decision": "ACCORD"}},
		{"/valider", map[string]any{"jeton_action": "pilote-demo"}},
		{"/publier", nil},
	} {
		if s, _, raw := c.req(t, "POST", "/api/projets/"+id+action.chemin, action.corps); s != 200 {
			t.Fatalf("étape %s : statut %d %s", action.chemin, s, raw)
		}
	}
}

func TestDriveEcritureAPI(t *testing.T) {
	racine := t.TempDir()
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: racine, AutoriserEcritureDrive: true},
	))
	c := connecter(t, srv, "alice")
	// 200 : écrit puis lisible via l'API de lecture.
	if s, m, _ := c.req(t, "POST", "/api/drive/document", map[string]any{
		"dossier": "entrees", "nom": "note.txt", "contenu": "pièce fournie",
	}); s != 200 || m["ecrit"] != true {
		t.Fatalf("écriture : attendu 200 ecrit, reçu %d %v", s, m)
	}
	if s, m, _ := c.req(t, "GET", "/api/drive/?dossier=entrees", nil); s != 200 ||
		m["documents"].([]any)[0] != "note.txt" {
		t.Fatalf("lecture après écriture : %d %v", s, m)
	}
	// 401 : session obligatoire.
	anonyme := &client{base: srv.URL}
	if s, _, _ := anonyme.req(t, "POST", "/api/drive/document", map[string]any{
		"nom": "x", "contenu": "y",
	}); s != 401 {
		t.Fatalf("écriture sans session : attendu 401, reçu %d", s)
	}
	// 400 : fail-closed (nom chemin, dossier en remontée, corps trop grand).
	for _, corps := range []map[string]any{
		{"dossier": "a", "nom": "../evil", "contenu": "x"},
		{"dossier": "..", "nom": "ok.txt", "contenu": "x"},
		{"dossier": "a:b", "nom": "ok.txt", "contenu": "x"},
		{"dossier": "", "nom": "gros.txt", "contenu": strings.Repeat("x", 1<<20+1)},
	} {
		if s, m, _ := c.req(t, "POST", "/api/drive/document", corps); s != 400 {
			t.Fatalf("entrée interdite %v : attendu 400, reçu %d %v", corps["nom"], s, m)
		}
	}
	// 403 : opt-in requis (autoriser_ecriture_drive absent).
	_, srv2 := gatewayTest(t, configTest(
		compteConf{Nom: "bob", Demo: true, DossierDrive: t.TempDir()},
	))
	b := connecter(t, srv2, "bob")
	if s, m, _ := b.req(t, "POST", "/api/drive/document", map[string]any{
		"nom": "note.txt", "contenu": "x",
	}); s != 403 || m["code"] != "DRIVE_REFUS" {
		t.Fatalf("écriture non autorisée : attendu 403 DRIVE_REFUS, reçu %d %v", s, m)
	}
	// 404 : aucun Drive configuré pour ce compte.
	_, srv3 := gatewayTest(t, configTest(compteConf{Nom: "carole", Demo: true}))
	ca := connecter(t, srv3, "carole")
	if s, _, _ := ca.req(t, "POST", "/api/drive/document", map[string]any{
		"nom": "note.txt", "contenu": "x",
	}); s != 404 {
		t.Fatalf("écriture sans Drive : attendu 404, reçu %d", s)
	}
}

func TestRoutageDriveConfig(t *testing.T) {
	g, _ := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, RemoteRclone: "recette", DossierDrive: "coord"},
		compteConf{Nom: "bob", Demo: true, DossierDrive: t.TempDir()},
		compteConf{Nom: "carole", Demo: true},
	))
	if _, ok := g.drives["alice"].(*dossierRclone); !ok {
		t.Fatalf("alice : attendu *dossierRclone, reçu %T", g.drives["alice"])
	}
	if _, ok := g.drives["bob"].(*dossierFactice); !ok {
		t.Fatalf("bob : attendu *dossierFactice, reçu %T", g.drives["bob"])
	}
	if g.drives["carole"] != nil {
		t.Fatalf("carole : aucun Drive attendu, reçu %T", g.drives["carole"])
	}
}

func TestExportOptionADossierSortie(t *testing.T) {
	racine := t.TempDir()
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: racine, ExportSortie: true},
	))
	c := connecter(t, srv, "alice")
	scenarioJusquaTransmission(t, c, "etude-marche", "sortie/2026")
	// Transmission 200 AUGMENTÉE du champ export_sortie, sans jamais
	// faire échouer la transmission.
	s, p, _ := c.req(t, "POST", "/api/projets/etude-marche/transmettre", nil)
	if s != 200 || p["etat"] != etatTransmis {
		t.Fatalf("transmission : attendu 200 TRANSMIS, reçu %d %v", s, p["etat"])
	}
	exp, ok := p["export_sortie"].(map[string]any)
	if !ok || exp["etat"] != "ok" {
		t.Fatalf("export_sortie : attendu ok, reçu %v", p["export_sortie"])
	}
	if exp["dossier"] != "sortie/2026" {
		t.Fatalf("export_sortie.dossier : reçu %v", exp["dossier"])
	}
	fichiers, _ := exp["fichiers"].([]any)
	if len(fichiers) != 3 {
		t.Fatalf("export_sortie.fichiers : attendu 3 fichiers, reçu %v", fichiers)
	}
	// Les trois fichiers existent SOUS la racine du compte, avec le
	// contenu attendu (ordre, devis, journal ndjson).
	chemin := filepath.Join(racine, "sortie", "2026")
	for _, nom := range []string{"ordre.json", "devis.json", "journal.ndjson"} {
		if _, err := os.Stat(filepath.Join(chemin, nom)); err != nil {
			t.Fatalf("fichier exporté absent : %s (%v)", nom, err)
		}
	}
	ordre, _ := os.ReadFile(filepath.Join(chemin, "ordre.json"))
	if !strings.Contains(string(ordre), "\"ordre\"") || !strings.Contains(string(ordre), "v1") {
		t.Fatalf("ordre.json inattendu : %s", ordre)
	}
	journal, _ := os.ReadFile(filepath.Join(chemin, "journal.ndjson"))
	if !strings.Contains(string(journal), "TRANSMISSION") {
		t.Fatalf("journal.ndjson inattendu : %s", journal)
	}
	// État consultable (bandeau front) puis relance idempotente.
	if s, m, _ := c.req(t, "GET", "/api/drive/export/etude-marche", nil); s != 200 ||
		m["export_sortie"].(map[string]any)["etat"] != "ok" {
		t.Fatalf("état export : reçu %d %v", s, m)
	}
	if s, m, _ := c.req(t, "POST", "/api/drive/export/etude-marche", nil); s != 200 ||
		m["export_sortie"].(map[string]any)["etat"] != "ok" {
		t.Fatalf("relance export : reçu %d %v", s, m)
	}
}

func TestExportOptionAEchecNonBloquant(t *testing.T) {
	// Cadrage SANS dossier de sortie : échec propre, motivé.
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir(), ExportSortie: true},
	))
	c := connecter(t, srv, "alice")
	scenarioJusquaTransmission(t, c, "sans-sortie", "")
	s, p, _ := c.req(t, "POST", "/api/projets/sans-sortie/transmettre", nil)
	if s != 200 || p["etat"] != etatTransmis {
		t.Fatalf("transmission : la mission DOIT partir (non bloquant), reçu %d %v", s, p["etat"])
	}
	exp, _ := p["export_sortie"].(map[string]any)
	if exp["etat"] != "echec" || !strings.Contains(exp["motif"].(string), "dossier de sortie") {
		t.Fatalf("export sans dossier : attendu echec motivé, reçu %v", exp)
	}

	// Drive en panne (dossiersDefauts) : échec d'écriture signalé,
	// transmission 200, état consultable, relance possible.
	g, srv2 := gatewayTest(t, configTest(
		compteConf{Nom: "bob", Demo: true, DossierDrive: t.TempDir(), ExportSortie: true},
	))
	g.drives["bob"] = &dossiersDefauts{motif: "panne simulée du stockage"}
	b := connecter(t, srv2, "bob")
	scenarioJusquaTransmission(t, b, "mission-b", "sortie")
	s, p, _ = b.req(t, "POST", "/api/projets/mission-b/transmettre", nil)
	if s != 200 || p["etat"] != etatTransmis {
		t.Fatalf("transmission malgré export défaillant : reçu %d %v", s, p["etat"])
	}
	exp, _ = p["export_sortie"].(map[string]any)
	if exp["etat"] != "echec" || !strings.Contains(exp["motif"].(string), "panne simulée") {
		t.Fatalf("export défaillant : attendu echec/panne, reçu %v", exp)
	}
	if s, m, _ := b.req(t, "GET", "/api/drive/export/mission-b", nil); s != 200 ||
		m["export_sortie"].(map[string]any)["etat"] != "echec" {
		t.Fatalf("état export défaillant : reçu %d %v", s, m)
	}
	if s, m, _ := b.req(t, "POST", "/api/drive/export/mission-b", nil); s != 200 ||
		m["export_sortie"].(map[string]any)["etat"] != "echec" {
		t.Fatalf("relance export défaillant : reçu %d %v", s, m)
	}
}

func TestExportOptionADesactive(t *testing.T) {
	// Sans export_sortie : réponse de transmission TELLE QUELLE
	// (aucun champ export_sortie), aucun état connu côté gateway.
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: t.TempDir()},
	))
	c := connecter(t, srv, "alice")
	scenarioJusquaTransmission(t, c, "etude-marche", "sortie")
	s, _, raw := c.req(t, "POST", "/api/projets/etude-marche/transmettre", nil)
	if s != 200 || strings.Contains(raw, "export_sortie") {
		t.Fatalf("export désactivé : réponse NON augmentée attendue, reçu %d %s", s, raw)
	}
	if s, m, _ := c.req(t, "GET", "/api/drive/export/etude-marche", nil); s != 404 ||
		m["code"] != "EXPORT_INCONNU" {
		t.Fatalf("état export inconnu attendu : reçu %d %v", s, m)
	}
	// Relance : refusée pour un compte sans export_sortie.
	if s, m, _ := c.req(t, "POST", "/api/drive/export/etude-marche", nil); s != 403 ||
		m["code"] != "DRIVE_REFUS" {
		t.Fatalf("relance sans export_sortie : attendu 403 DRIVE_REFUS, reçu %d %v", s, m)
	}
}

/* ============ M-COORD-UI-01 : POST /api/drive/verifier (S4) ============ */

func TestVerifierDossiers(t *testing.T) {
	racine := t.TempDir()
	if err := os.MkdirAll(filepath.Join(racine, "entree-ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, srv := gatewayTest(t, configTest(
		compteConf{Nom: "alice", Demo: true, DossierDrive: racine, AutoriserEcritureDrive: true},
		compteConf{Nom: "bob", Demo: true, DossierDrive: racine}, // sans écriture Drive
	))

	// Session obligatoire.
	anonyme := &client{base: srv.URL}
	if s, _, _ := anonyme.req(t, "POST", "/api/drive/verifier",
		map[string]any{"dossier_entree": "a", "dossier_sortie": "b"}); s != 401 {
		t.Fatalf("sans session : attendu 401, reçu %d", s)
	}

	c := connecter(t, srv, "alice")

	// Champs manquants : 400 explicite.
	if s, m, _ := c.req(t, "POST", "/api/drive/verifier",
		map[string]any{"dossier_entree": "entree-ok"}); s != 400 || m["code"] != "CHAMPS_MANQUANTS" {
		t.Fatalf("champs manquants : attendu 400 CHAMPS_MANQUANTS, reçu %d %v", s, m)
	}

	// Cas nominal : lecture entrée OK, sonde écrire+supprimer sortie OK.
	s, m, _ := c.req(t, "POST", "/api/drive/verifier",
		map[string]any{"dossier_entree": "entree-ok", "dossier_sortie": "sortie-ok"})
	if s != 200 {
		t.Fatalf("verifier nominal : attendu 200, reçu %d", s)
	}
	entree, _ := m["entree"].(map[string]any)
	sortie, _ := m["sortie"].(map[string]any)
	if entree["ok"] != true || sortie["ok"] != true {
		t.Fatalf("verifier nominal : attendu ok:true partout, reçu %v %v", entree, sortie)
	}
	// La sonde a disparu du dossier de sortie.
	s, m, _ = c.req(t, "GET", "/api/drive?dossier=sortie-ok", nil)
	noms, _ := m["documents"].([]any)
	for _, n := range noms {
		if n == sondeVerifier {
			t.Fatalf("sonde %s non supprimée : %v", sondeVerifier, noms)
		}
	}

	// Dossier d'entrée hors périmètre : ok=false + motif compréhensible.
	// (Contrat Lister : un dossier inexistant mais lisible est
	// indistinguable d'un dossier vide ; la vérification porte sur
	// l'ACCESSIBILITÉ, pas l'existence.)
	s, m, _ = c.req(t, "POST", "/api/drive/verifier",
		map[string]any{"dossier_entree": "../hors", "dossier_sortie": "sortie-ok"})
	if s != 200 {
		t.Fatalf("verifier entrée refusée : attendu 200, reçu %d", s)
	}
	entree, _ = m["entree"].(map[string]any)
	if entree["ok"] != false || entree["motif"] == nil || entree["motif"] == "" {
		t.Fatalf("entrée refusée : attendu ok:false + motif, reçu %v", entree)
	}

	// Dossier interdit (remontée) : ok=false, motif de refus.
	s, m, _ = c.req(t, "POST", "/api/drive/verifier",
		map[string]any{"dossier_entree": "entree-ok", "dossier_sortie": "../evil"})
	if s != 200 {
		t.Fatalf("verifier sortie interdite : attendu 200, reçu %d", s)
	}
	sortie, _ = m["sortie"].(map[string]any)
	if sortie["ok"] != false || sortie["motif"] == nil || sortie["motif"] == "" {
		t.Fatalf("sortie interdite : attendu ok:false + motif, reçu %v", sortie)
	}

	// Écriture désactivée : refus EXPLICITE (403 DRIVE_REFUS), avant
	// toute écriture dans le Drive.
	bob := connecter(t, srv, "bob")
	if s, m, _ := bob.req(t, "POST", "/api/drive/verifier",
		map[string]any{"dossier_entree": "entree-ok", "dossier_sortie": "sortie-ok"}); s != 403 {
		t.Fatalf("écriture désactivée : attendu 403, reçu %d %v", s, m)
	}
}
