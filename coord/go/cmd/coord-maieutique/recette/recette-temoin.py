#!/usr/bin/env python3
# Recette TEMOIN M-COORD-MAIEUTIQUE-01 (issue #42, plan v2 §5 et §7).
# Conduit la chaine complete en SIMULE, loopback uniquement, aucune cle,
# 0 EUR : intention ambigue -> questions -> proposition VOLONTAIREMENT
# incoherente (rejet deterministe) -> proposition coherente refusee par
# critique BLOQUANTE -> version corrigee -> qualification -> validation
# explicite du pilote (jeton_action) -> SIGKILL reels (dialogue puis
# publication puis transmission) -> reprise sans double publication ni
# double transmission ni second appel -> cycle COORD complet de la mission
# fille MAIEUTIQUE-TEMOIN-01 (executer -> soumettre -> recevoir ACCEPTEE ->
# acker -> cloturer par le pilote) -> cadrage/devis/accord sans paiement
# reel. Preuves par journaux, compteurs et empreintes (critere 4 : les
# trois empreintes validee == publiee == transmise).
#
# Usage :
#   recette-temoin.py --work DIR --service BIN --harnais BIN \
#     --profil openai|anthropic [--sigkill 1|0] \
#     --port-directeur 8292 --port-exec 8293 --port-recep 8294 \
#     --port-service 8410 --projet temoin
# Ecrit : <work>/rapport-recette.json + journal console detaille.
import argparse, hashlib, json, os, re, secrets, signal, socket, subprocess, sys, threading, time
import urllib.error, urllib.request

REF_SOURCE = "issue dbouzon/osnias-clearing#42 version 1.4"
INTENTION = "Je veux que COORD m'aide à formuler mes missions"
FILLE = "MAIEUTIQUE-TEMOIN-01"

def ordres_temoins():
    base = {
        "identification": {"mission": FILLE, "version": "v0", "protocole": "COORD v1.5"},
        "etat_des_lieux": ("Le pilote formule ses missions sans méthode structurée ; "
                           "l'issue dbouzon/osnias-clearing#42 version 1.4 mandate le module maïeutique."),
        "sources": [{"ref": REF_SOURCE}],
        "limites": ["Aucun paiement réel (paiement_reel:false)", "Aucune clé API",
                    "Loopback uniquement, aucun écouteur public"],
        "liberte_execution": ("Le dialogue maïeutique conduit la formulation ; "
                              "le pilote décide seul de chaque transition."),
        "criteres_reception": ["Empreinte validée identique aux empreintes publiée et transmise",
                               "Aucun second appel pour une opération terminée"],
        "application_coord": "COORD v1.5, cycle Mistral exécutant / OpenAI réceptionnaire",
    }
    v1 = dict(base)
    v1["objectif"] = "Formuler un ordre de mission unique et produire un rapport complet"
    v1["etapes"] = [
        {"nom": "Rédaction", "controle_intermediaire": "contrôle du directeur sur la forme"},
        {"nom": "Rédaction", "controle_intermediaire": ""},
    ]
    v2 = dict(base)
    v2["objectif"] = "Produire un ordre de mission qualifié pour la mission témoin du module maïeutique"
    v2["etapes"] = [
        {"nom": "Formulation", "controle_intermediaire": "critique sémantique du directeur obligatoire"},
        {"nom": "Qualification", "controle_intermediaire": ""},
    ]
    v2["livrable"] = "Un ordre de mission qualifié"
    v3 = dict(v2)
    v3["livrable"] = ("Un ordre de mission qualifié, figé, publié puis transmis "
                      "à la mission fille " + FILLE)
    v1["livrable"] = v3["livrable"]
    return v1, v2, v3

REPONSES = [
    {"action": "QUESTION",
     "contenu": "Quel livrable précis attends-tu de COORD pour formuler tes missions, et pour quel destinataire ?"},
    {"action": "CLARIFICATION",
     "contenu": "Précise la source de référence qui fait foi pour cet ordre de mission."},
    None,  # PROPOSITION_ORDRE v1 (incohérente) injectée au démarrage
    {"action": "VERDICT", "verdict": "A_CORRIGER",
     "points_detail": [{"severite": "BLOQUANT",
                        "texte": "Le livrable annoncé n'est pas vérifiable : aucune forme précise."}]},
    {"action": "VERDICT", "verdict": "RESERVES_NON_BLOQUANTES",
     "points_detail": [{"severite": "RESERVE_NON_BLOQUANTE",
                        "texte": "Réserve : le périmètre exclut tout paiement réel."}]},
]

PREUVES = []
def preuve(nom, ok, detail=""):
    PREUVES.append({"nom": nom, "ok": bool(ok), "detail": str(detail)})
    print(("PASS " if ok else "FAIL ") + nom + (" : " + str(detail) if detail else ""))
    return ok


def sha256f(chemin):
    h = hashlib.sha256()
    with open(chemin, "rb") as f:
        h.update(f.read())
    return h.hexdigest()


def attendre_port(port, delai=10.0):
    fin = time.time() + delai
    while time.time() < fin:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.5):
                return True
        except OSError:
            time.sleep(0.1)
    return False


class Service:
    def __init__(self, chemin, args, log):
        self.chemin, self.args, self.log = chemin, args, log
        self.proc = None
        self.token = None

    def demarrer(self):
        try:
            self.logf.close()
        except Exception:
            pass
        # ne lire QUE l'ajout de CE démarrage (le token change à chaque
        # démarrage ; l'ancien token est périmé)
        try:
            offset = os.path.getsize(self.log)
        except FileNotFoundError:
            offset = 0
        self.logf = open(self.log, "ab")
        self.proc = subprocess.Popen([self.chemin] + self.args, stdout=self.logf,
                                     stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL)
        self.token = None
        fin = time.time() + 10
        while time.time() < fin:
            try:
                with open(self.log, "rb") as f:
                    f.seek(offset)
                    ajout = f.read().decode("utf-8", "replace")
                if "NON_EXECUTE" in ajout or "ERREUR" in ajout:
                    return False
                for ligne in ajout.splitlines():
                    if ligne.startswith("Token local : "):
                        self.token = ligne.split(" : ", 1)[1].strip()
                if self.token:
                    return True
            except FileNotFoundError:
                pass
            time.sleep(0.1)
        return False

    def tuer(self):
        if self.proc and self.proc.poll() is None:
            os.kill(self.proc.pid, signal.SIGKILL)
            self.proc.wait()

    def arreter(self):
        if self.proc and self.proc.poll() is None:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.proc.kill()

class Client:
    def __init__(self, port, token):
        self.base = "http://127.0.0.1:%d" % port
        self.token = token
        self.latences = []

    def appel(self, methode, chemin, corps=None, timeout=60):
        req = urllib.request.Request(self.base + chemin, method=methode)
        req.add_header("X-Coord-Pilot-Token", self.token)
        data = None
        if corps is not None:
            data = json.dumps(corps).encode()
            req.add_header("Content-Type", "application/json")
        debut = time.time()
        try:
            with urllib.request.urlopen(req, data=data, timeout=timeout) as rep:
                statut, brut = rep.status, rep.read()
        except urllib.error.HTTPError as e:
            statut, brut = e.code, e.read()
        self.latences.append((chemin, round(time.time() - debut, 4)))
        try:
            return statut, json.loads(brut)
        except Exception:
            return statut, {"brut": brut.decode("utf-8", "replace")}

def lire_journal_maieutique(work, projet):
    chemin = os.path.join(work, "data", projet, "maieutique.jsonl")
    entrees = []
    if os.path.exists(chemin):
        with open(chemin, encoding="utf-8") as f:
            for ligne in f:
                ligne = ligne.strip()
                if ligne:
                    entrees.append(json.loads(ligne))
    return entrees

def compter_factice(log):
    if not os.path.exists(log):
        return 0
    n = 0
    with open(log, encoding="utf-8") as f:
        for ligne in f:
            if ligne.startswith("POST"):
                n += 1
    return n

def principale():
    ap = argparse.ArgumentParser()
    ap.add_argument("--work", required=True)
    ap.add_argument("--service", required=True)
    ap.add_argument("--harnais", required=True)
    ap.add_argument("--profil", default="openai")
    ap.add_argument("--sigkill", type=int, default=1)
    ap.add_argument("--port-directeur", type=int, default=8292)
    ap.add_argument("--port-exec", type=int, default=8293)
    ap.add_argument("--port-recep", type=int, default=8294)
    ap.add_argument("--port-service", type=int, default=8410)
    ap.add_argument("--projet", default="temoin")
    args = ap.parse_args()
    work = args.work
    os.makedirs(work, exist_ok=True)
    data = os.path.join(work, "data")
    missions = os.path.join(work, "missions")
    logs = os.path.join(work, "logs")
    os.makedirs(logs, exist_ok=True)
    projet = args.projet
    v1, v2, v3 = ordres_temoins()
    reponses = [r if r is not None else
                {"action": "PROPOSITION_ORDRE", "ordre": v1} for r in REPONSES]
    fich_reponses = os.path.join(work, "reponses-directeur.json")
    with open(fich_reponses, "w", encoding="utf-8") as f:
        json.dump(reponses, f, ensure_ascii=False, indent=1)

    ici = os.path.dirname(os.path.abspath(__file__))
    log_dir = os.path.join(logs, "factice-directeur.log")
    log_exec = os.path.join(logs, "factice-exec.log")
    log_recep = os.path.join(logs, "factice-recep.log")
    log_service = os.path.join(logs, "service.log")
    factices = [
        subprocess.Popen([sys.executable, os.path.join(ici, "factice-directeur.py"),
                         str(args.port_directeur), fich_reponses, log_dir]),
        subprocess.Popen([sys.executable, os.path.join(ici, "factice-exec-temoin.py"),
                         str(args.port_exec), log_exec]),
        subprocess.Popen([sys.executable, os.path.join(ici, "factice-recep-temoin.py"),
                         str(args.port_recep), log_recep, "maieutique-" + args.profil + "-verdict"]),
    ]
    try:
        for port in (args.port_directeur, args.port_exec, args.port_recep):
            if not attendre_port(port):
                print("NON_EXECUTE : factice inaccessible sur %d" % port)
                return 2

        def args_service():
            return ["-addr", "127.0.0.1:%d" % args.port_service,
                    "-data", data, "-missions", missions,
                    "-fournisseur", "directeur-%s=http://127.0.0.1:%d/" % (args.profil, args.port_directeur),
                    "-fournisseur", "openai-exec=http://127.0.0.1:%d/" % args.port_exec,
                    "-fournisseur", "openai-recep=http://127.0.0.1:%d/" % args.port_recep]

        svc = Service(args.service, args_service(), log_service)
        if not svc.demarrer():
            print("NON_EXECUTE : service coord-maieutique non démarré")
            return 2
        c = Client(args.port_service, svc.token)

        # ---- chaine maieutique ----
        st, m = c.appel("POST", "/projets", {"id": projet, "intention": INTENTION})
        preuve("intention déclarée (INTENTION)", st == 201 and m.get("etat") == "INTENTION", st)
        st, m = c.appel("POST", "/projets/%s/qualifier" % projet)
        preuve("qualification impossible sans ordre (422 ORDRE_ABSENT)",
               st == 422 and m.get("code") == "ORDRE_ABSENT", m)
        st, m = c.appel("POST", "/projets/%s/sources" % projet, {"sources": [REF_SOURCE]})
        preuve("source déclarée par le pilote", st == 200 and REF_SOURCE in m.get("sources_declarees", []), st)
        st, m = c.appel("POST", "/projets/%s/dialogue" % projet,
                        {"directeur": "directeur-" + args.profil, "executant": "openai-exec"})
        preuve("binôme distinct ouvert (DIALOGUE)", st == 200 and m.get("etat") == "DIALOGUE", st)

        st, m = c.appel("POST", "/projets/%s/tour" % projet,
                        {"entree": "Contexte : la mission mère est dbouzon/osnias-clearing#42 version 1.4."})
        preuve("intention ambiguë -> QUESTION (jamais un ordre)",
               st == 200 and m.get("reponse", {}).get("action") == "QUESTION", m.get("reponse", {}).get("action"))

        appels_factice = lambda: compter_factice(log_dir)

        if args.sigkill:
            # ---- SIGKILL #1 : phase de dialogue ----
            avant = appels_factice()
            resultat = {}

            def tour_fenetre():
                try:
                    resultat["st"], resultat["m"] = c.appel(
                        "POST", "/projets/%s/tour" % projet,
                        {"entree": "SIGKILL_FENETRE : précise le livrable attendu."}, timeout=30)
                except Exception as e:
                    resultat["err"] = repr(e)

            th = threading.Thread(target=tour_fenetre)
            th.start()
            time.sleep(1.5)
            svc.tuer()
            th.join(timeout=30)
            preuve("SIGKILL réel pendant l'appel du directeur (dialogue)",
                   svc.proc.poll() is not None, resultat.get("err", "coordinateur tué"))
            # le factice termine son appel (fenêtre 4 s) : le log doit montrer
            # l'appel reçu PENDANT la fenêtre lentille, réponse jamais consommée
            ligne_lente = ""
            fin = time.time() + 8
            while time.time() < fin:
                lignes = [l for l in open(log_dir, encoding="utf-8")] if os.path.exists(log_dir) else []
                if len(lignes) >= avant + 1:
                    ligne_lente = lignes[avant]
                    break
                time.sleep(0.2)
            preuve("l'appel tué a atteint le directeur (fenêtre réelle, réponse non consommée)",
                   "fenetre_sigkill" in ligne_lente, ligne_lente.strip())
            if not svc.demarrer():
                print("NON_EXECUTE : service non redémarré après SIGKILL")
                return 2
            c = Client(args.port_service, svc.token)
            st, m = c.appel("GET", "/projets/%s" % projet)
            journal = lire_journal_maieutique(work, projet)
            preuve("reprise : état DIALOGUE intact, aucune trace du tour tué",
                   m.get("etat") == "DIALOGUE" and m.get("appels") == 1
                   and sum(1 for e in journal if e.get("type") == "TOUR_DIRECTEUR") == 1,
                   "etat=%s appels=%s" % (m.get("etat"), m.get("appels")))

        st, m = c.appel("POST", "/projets/%s/tour" % projet,
                        {"entree": "Voici la source qui fait foi : " + REF_SOURCE + "."})
        if args.sigkill:
            preuve("rejeu du tour : file du directeur INTACTE (CLARIFICATION servie, pas la proposition)",
                   st == 200 and m.get("reponse", {}).get("action") == "CLARIFICATION",
                   m.get("reponse", {}).get("action"))
        else:
            preuve("clarification du directeur (CLARIFICATION)",
                   st == 200 and m.get("reponse", {}).get("action") == "CLARIFICATION",
                   m.get("reponse", {}).get("action"))

        avant = appels_factice()
        st, m = c.appel("POST", "/projets/%s/tour" % projet, {"entree": "Propose un ordre."})
        apres = appels_factice()
        preuve("proposition INCOHÉRENTE rejetée (422 OBJECTIF_NON_UNIQUE)",
               st == 422 and m.get("code") == "OBJECTIF_NON_UNIQUE", m)
        preuve("rejet déterministe : AUCUN second appel du directeur",
               apres - avant == 1, "appels factice %d -> %d" % (avant, apres))

        st, m = c.appel("POST", "/projets/%s/ordre" % projet, v2)
        preuve("proposition cohérente refusée par critique BLOQUANTE (422)",
               st == 422 and m.get("code") == "CRITIQUE_BLOQUANTE", m)

        st, m = c.appel("POST", "/projets/%s/ordre" % projet, v3)
        preuve("version corrigée -> ORDRE_PROPOSE (réserves non bloquantes)",
               st == 200 and m.get("etat") == "ORDRE_PROPOSE", m.get("etat"))

        st, m = c.appel("POST", "/projets/%s/qualifier" % projet)
        empreinte = m.get("empreinte_ordre", "")
        preuve("qualification (ORDRE_QUALIFIE, figée v1 + empreinte)",
               st == 200 and m.get("etat") == "ORDRE_QUALIFIE"
               and m.get("version") == 1 and re.fullmatch(r"[0-9a-f]{64}", empreinte),
               "v%s %s" % (m.get("version"), empreinte[:12]))

        jeton = secrets.token_hex(16)
        st, m = c.appel("POST", "/projets/%s/valider" % projet, {"jeton_action": jeton})
        chemin_validation = os.path.join(work, "data", projet, "validation.json")
        brut_validation = open(chemin_validation, encoding="utf-8").read() if os.path.exists(chemin_validation) else ""
        preuve("validation EXPLICITE du pilote (VALIDE, jeton exigé)",
               st == 200 and m.get("etat") == "VALIDE", st)
        preuve("preuve validation.json SANS le jeton", "jeton" not in brut_validation and jeton not in brut_validation,
               brut_validation[:80])

        st, m = c.appel("POST", "/projets/%s/cadrage" % projet,
                        {"perimetre": "démonstration du module maïeutique",
                         "paiement_reel": False, "cible": "VPS 1 vCPU / 2 Go"})
        preuve("cadrage fixé (gratuit, avant figage de la chaîne de publication)",
               st == 200 and m.get("cadrage", {}).get("paiement_reel") is False, st)

        if args.sigkill:
            # ---- SIGKILL #2 : phase de publication ----
            def publier_bg():
                try:
                    c.appel("POST", "/projets/%s/publier" % projet, {}, timeout=30)
                except Exception:
                    pass
            th = threading.Thread(target=publier_bg)
            th.start()
            time.sleep(0.05)
            svc.tuer()
            th.join(timeout=30)
            if not svc.demarrer():
                print("NON_EXECUTE : service non redémarré après SIGKILL publication")
                return 2
            c = Client(args.port_service, svc.token)
            st, m = c.appel("GET", "/projets/%s" % projet)
            preuve("SIGKILL en phase de publication : reprise depuis le dernier état durable",
                   m.get("etat") in ("VALIDE", "PUBLIE"), m.get("etat"))

        st, m = c.appel("POST", "/projets/%s/publier" % projet)
        publie_premier = st == 200
        st2, m2 = c.appel("POST", "/projets/%s/publier" % projet)
        preuve("publication : exactement UN effet (200 puis 409 OPERATION_DEJA_EFFECTUEE)",
               (publie_premier or st == 409) and st2 == 409
               and m2.get("code") == "OPERATION_DEJA_EFFECTUEE",
               "premier=%s second=%s" % (st, st2))

        if args.sigkill:
            # ---- SIGKILL #3 : phase de transmission ----
            def transmettre_bg():
                try:
                    c.appel("POST", "/projets/%s/transmettre" % projet, {}, timeout=30)
                except Exception:
                    pass
            th = threading.Thread(target=transmettre_bg)
            th.start()
            time.sleep(0.05)
            svc.tuer()
            th.join(timeout=30)
            if not svc.demarrer():
                print("NON_EXECUTE : service non redémarré après SIGKILL transmission")
                return 2
            c = Client(args.port_service, svc.token)

        st, m = c.appel("POST", "/projets/%s/transmettre" % projet)
        transmis_premier = st == 200
        st2, m2 = c.appel("POST", "/projets/%s/transmettre" % projet)
        preuve("transmission : exactement UN effet (puis 409 OPERATION_DEJA_EFFECTUEE)",
               (transmis_premier or st == 409) and st2 == 409
               and m2.get("code") == "OPERATION_DEJA_EFFECTUEE",
               "premier=%s second=%s" % (st, st2))

        # ---- preuves empreintes (critere 4) ----
        dir_projet = os.path.join(work, "data", projet)
        fige = os.path.join(dir_projet, "ordres", "ordre-fige.json")
        livr = os.path.join(dir_projet, "livrable", "ordre-v1.json")
        sha_fige = sha256f(fige)
        sha_livr = sha256f(livr)
        pub = json.load(open(os.path.join(dir_projet, "publication.json"), encoding="utf-8"))
        tr = json.load(open(os.path.join(dir_projet, "transmission.json"), encoding="utf-8"))
        val = json.load(open(os.path.join(dir_projet, "validation.json"), encoding="utf-8"))
        st, m = c.appel("GET", "/projets/%s" % projet)
        preuve("empreintes validée == publiée == transmise == figée (sha256 vérifiés)",
               val.get("empreinte") == pub.get("empreinte") == tr.get("empreinte")
               == m.get("empreinte_ordre") == sha_fige == sha_livr,
               "figé=%s" % sha_fige[:12])
        fichiers_livrable = os.listdir(os.path.join(dir_projet, "livrable"))
        preuve("un SEUL fichier livrable publié", fichiers_livrable == ["ordre-v1.json"], fichiers_livrable)

        # ---- preuves journal (0 double, rejets tracés) ----
        journal = lire_journal_maieutique(work, projet)
        trans_pub = [e for e in journal if e.get("type") == "TRANSITION_MAIEUTIQUE"
                     and e.get("action") == "publier"]
        trans_tr = [e for e in journal if e.get("type") == "TRANSITION_MAIEUTIQUE"
                    and e.get("action") == "transmettre"]
        preuve("journal : UNE transition publier, UNE transition transmettre",
               len(trans_pub) == 1 and len(trans_tr) == 1,
               "publier=%d transmettre=%d" % (len(trans_pub), len(trans_tr)))
        preuve("journal : rejets tracés (REJET_ORDRE + CRITIQUE_BLOQUANTE)",
               any(e.get("type") == "REJET_ORDRE" for e in journal)
               and sum(1 for e in journal if e.get("type") == "CRITIQUE_BLOQUANTE") == 1)
        preuve("journal : AUCUNE attente pilote subie (échecs directeur < borne)",
               all(e.get("etat_apres") != "ATTENTE_PILOTE" for e in journal))
        refus = [e for e in journal if e.get("type") in ("REFUS_PUBLICATION", "REFUS_TRANSMISSION")]
        preuve("journal : refus idempotents tracés (REFUS_PUBLICATION/TRANSMISSION)",
               len(refus) >= 2, len(refus))

        # ---- cycle COORD complet de la mission fille ----
        def harnais(mode, extra=()):
            cmd = [args.harnais, "-data", missions, "-issue", FILLE, "-mode", mode,
                   "-exec-endpoint", "http://127.0.0.1:%d/" % args.port_exec,
                   "-recep-endpoint", "http://127.0.0.1:%d/" % args.port_recep] + list(extra)
            r = subprocess.run(cmd, capture_output=True, text=True)
            return r

        r = harnais("executer", ["-mandat", livr, "-sortie", os.path.join(work, "livrable-fille.txt")])
        preuve("fille : executer -> EXECUTION", r.returncode == 0 and "EXECUTION" in r.stdout, r.stdout.strip())
        r = harnais("cloturer")
        preuve("fille : clôture par automatisme REFUSÉE hors ACK_RECU", r.returncode == 2, r.stdout.strip())
        r = harnais("soumettre", ["-livrable", os.path.join(work, "livrable-fille.txt")])
        preuve("fille : soumettre -> DECISION_ATTENDUE",
               r.returncode == 0 and "DECISION_ATTENDUE" in r.stdout, r.stdout.strip())
        r = harnais("recevoir", ["-livrable", os.path.join(work, "livrable-fille.txt")])
        preuve("fille : recevoir -> ACCEPTEE (réception indépendante)",
               r.returncode == 0 and "ACCEPTEE" in r.stdout, r.stdout.strip().splitlines()[:1])
        r = harnais("cloturer")
        preuve("fille : clôture REFUSÉE avant ACK (garde pilote)", r.returncode == 2, r.stdout.strip())
        r = harnais("acker")
        preuve("fille : acker -> ACK_RECU", r.returncode == 0 and "ACK_RECU" in r.stdout, r.stdout.strip())
        r = harnais("cloturer")
        preuve("fille : clôture RÉSERVÉE au pilote humain (refus de l'automatisme)",
               r.returncode == 2 and "CLOTURE_REFUSEE" in r.stdout, r.stdout.strip()[:80])

        # ---- journal de la mission fille ----
        jf = os.path.join(missions, FILLE, "journal.jsonl")
        entrees = [json.loads(l) for l in open(jf, encoding="utf-8") if l.strip()]
        types = [e.get("type") for e in entrees]
        preuve("fille : ancre TRANSMISSION_MAIEUTIQUE unique avec lien vers #42",
               types.count("TRANSMISSION_MAIEUTIQUE") == 1
               and any(e.get("type") == "TRANSMISSION_MAIEUTIQUE"
                       and e.get("lien_mere") == "dbouzon/osnias-clearing#42"
                       and e.get("empreinte") == empreinte for e in entrees))
        preuve("fille : cycle complet journalisé (création -> réception ACCEPTEE -> ACK -> clôture)",
               types.count("CREATION") == 1 and types.count("CREATION_ORCHESTREE") == 1
               and any(e.get("type") == "RECEPTION" and e.get("verdict") == "ACCEPTEE" for e in entrees)
               and any(e.get("type") == "ACK" for e in entrees))
        mission_fille = json.load(open(os.path.join(missions, FILLE, "mission.json"), encoding="utf-8"))
        preuve("fille : état final ACK_RECU (clôture réservée au pilote humain), identifiant DISTINCT",
               mission_fille["Mission"]["State"] == "ACK_RECU"
               and mission_fille["Issue"] == FILLE, mission_fille["Mission"]["State"])

        # ---- devis sans paiement réel ----
        st, m = c.appel("POST", "/projets/%s/devis" % projet)
        preuve("devis DÉRIVÉ de l'ordre (étapes + livrable + limites, jamais saisi à la main)",
               st == 200 and m.get("devis", {}).get("etapes") == ["Formulation", "Qualification"]
               and m.get("devis", {}).get("version") == "v1", m.get("devis"))
        st, m = c.appel("POST", "/projets/%s/accord-devis" % projet, {"decision": "ACCORD"})
        journal = lire_journal_maieutique(work, projet)
        accords = [e for e in journal if e.get("type") == "ACCORD_DEVIS"]
        preuve("accord du pilote sur le devis (ACCORD, paiement_reel:false consigné)",
               st == 200 and m.get("accord_devis") is True
               and len(accords) == 1 and accords[0].get("decision") == "ACCORD"
               and accords[0].get("paiement_reel") is False, accords)

        # ---- mesures (critere 7) ----
        try:
            with open("/proc/%d/status" % svc.proc.pid, encoding="utf-8") as f:
                rss = [l for l in f if l.startswith("VmHWM")]
            with open("/proc/%d/stat" % svc.proc.pid, encoding="utf-8") as f:
                champs = f.read().rsplit(")", 1)[1].split()
            cpu = (int(champs[11]) + int(champs[12])) / os.sysconf("SC_CLK_TCK")
        except Exception:
            rss, cpu = ["?"], -1
        mesures = {"latences_s": c.latences, "rss_service": rss[0].strip() if rss else "?",
                   "cpu_service_s": round(cpu, 3),
                   "appels_directeur": appels_factice(),
                   "appels_exec": compter_factice(log_exec),
                   "appels_recep": compter_factice(log_recep)}
        print("MESURES : " + json.dumps(mesures, ensure_ascii=False))
    finally:
        try:
            svc.arreter()
        except Exception:
            pass
        for f in factices:
            f.terminate()
        for f in factices:
            try:
                f.wait(timeout=5)
            except Exception:
                f.kill()

    ok = all(p["ok"] for p in PREUVES)
    rapport = {"profil": args.profil, "sigkill": bool(args.sigkill), "projet": projet,
               "ok": ok, "preuves": PREUVES, "mesures": mesures if "mesures" in dir() else {}}
    with open(os.path.join(work, "rapport-recette.json"), "w", encoding="utf-8") as f:
        json.dump(rapport, f, ensure_ascii=False, indent=1)
    print("RECETTE %s : %d/%d preuves" % ("VERTE" if ok else "ROUGE",
                                          sum(1 for p in PREUVES if p["ok"]), len(PREUVES)))
    return 0 if ok else 1

if __name__ == "__main__":
    sys.exit(principale())
