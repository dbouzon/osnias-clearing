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
