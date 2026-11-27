#!/bin/bash
# recette-flow01.sh : lanceur de recette REPRODUCTIBLE M-COORD-FLOW-01 (issue #76,
# plan v2, T3). Compile le harnais depuis les sources ECRITEES (SHA fixe par le
# kit), demarre DEUX factices loopback SEPARES (processus coord, jamais tues avec
# le coordinateur), execute les scenarios N/R/V/X/I/P, verifie les compteurs,
# puis ARCHIVE les traces. HORS RESEAU, AUCUNE CLE, 0 EUR.
#
# T3 (arrets ciblés) : chaque processus coordinateur inscrit son PID/PGID reel
# dans un fichier temoin AVANT toute action ; le lanceur verifie l'appartenance
# (existence, PGID, ligne de commande attendue) AVANT tout signal, et n'utilise
# JAMAIS kill de groupe ni pkill par motif.
#
# Usage : recette-flow01.sh <racine-kit-absolue> <repertoire-travail-absolu>
#   racine-kit  : dossier contenant ce script + factices + sources coord/go
#   travail     : dossier ETAT (data, traces, temoins, factices) ; CREE par le
#                 lanceur, jamais dans le repo.
set -u
KIT="${1:?racine du kit requise}"
TRAVAIL="${2:?dossier de travail requis}"
HARNAIS_BIN=""
DATA="$TRAVAIL/state"
TRACES="$TRAVAIL/traces"
TEMOINS="$TRAVAIL/temoins"
FACTLOG="$TRAVAIL/factices"
PORT_EXEC=8093
PORT_RECEP=8094
ISSUE_N=flow01-recette-n
ISSUE_X=flow01-recette-x
ISSUE_I=flow01-recette-i
PASS=0; FAIL=0

verif() { if [ "$1" -eq 0 ]; then PASS=$((PASS+1)); echo "  [OK] $2"; else FAIL=$((FAIL+1)); echo "  [ECHEC] $2"; fi; }
contient() { grep -q "$2" "$1"; }
cmp_chaine() { [ "$1" = "$2" ]; }

# tuer_par_pid <fichier-temoin> <marqueur-cmdline-attendu> : T3, arret cible.
# Verifie AVANT le signal : existence du temoin, existence du processus,
# appartenance du PGID (conforme au temoin) et presence du marqueur dans la
# ligne de commande. Aucun kill de groupe, aucun pkill par motif.
tuer_par_pid() {
  local temoin="$1" marqueur="$2" pid pgid rpid rpgid cmdline
  [ -f "$temoin" ] || { echo "  [ECHEC] temoin absent : $temoin"; return 1; }
  pid=$(sed -n 's/^PID=//p' "$temoin"); pgid=$(sed -n 's/^PGID=//p' "$temoin")
  [ -d "/proc/$pid" ] || { echo "  [ECHEC] processus $pid absent (deja mort ?)"; return 1; }
  rpgid=$(awk '{print $5}' "/proc/$pid/stat")
  cmp_chaine "$rpgid" "$pgid" || { echo "  [ECHEC] PGID divergent ($rpgid != $pgid) : refus d'arret"; return 1; }
  cmdline=$(tr '\0' ' ' < "/proc/$pid/cmdline")
  case "$cmdline" in *"$marqueur"*) ;; *) echo "  [ECHEC] ligne de commande inattendue : $cmdline"; return 1;; esac
  kill -9 "$pid" || { echo "  [ECHEC] kill $pid"; return 1; }
  for _ in 1 2 3 4 5 6 7 8 9 10; do [ -d "/proc/$pid" ] || break; sleep 0.2; done
  [ -d "/proc/$pid" ] && { echo "  [ECHEC] processus $pid toujours vivant"; return 1; }
  echo "  [OK] arret cible PID=$pid (marqueur: $marqueur)"
  return 0
}

harnais() { "$HARNAIS_BIN" "$@"; }

echo "== M-COORD-FLOW-01 : recette N/R/V/X/I/P (0 EUR, loopback) =="
echo "== preparation : $TRAVAIL"
rm -rf "$TRAVAIL"; mkdir -p "$DATA" "$TRACES" "$TEMOINS" "$FACTLOG"

echo "== compilation du harnais (sources ecrites, SHA fixe par le kit)"
# Toolchain Go : PATH standard, puis /usr/local/go/bin, puis toolchain VPS
# (/srv/toolchain/go*/bin et ~/.go-toolchain/go*/bin), GOTOOLCHAIN local.
export PATH="$PATH:/usr/local/go/bin:$(ls -d /srv/toolchain/go*/bin 2>/dev/null | head -1)$(ls -d "$HOME"/.go-toolchain/go*/bin 2>/dev/null | head -1)"
export GOTOOLCHAIN=local
( cd "$KIT/go" 2>/dev/null || cd "$KIT" ; export GOFLAGS=-mod=mod; go build -o "$TRAVAIL/harnais-flow01" ./cmd/harnais-flow01/ ) > "$TRACES/build.log" 2>&1
if [ -x "$TRAVAIL/harnais-flow01" ]; then PASS=$((PASS+1)); echo "  [OK] harnais compile"; else FAIL=$((FAIL+1)); echo "  [ECHEC] compilation"; cat "$TRACES/build.log"; exit 1; fi
HARNAIS_BIN="$TRAVAIL/harnais-flow01"

echo "== demarrage des factices (processus coord SEPARES, temoins)"
python3 "$KIT/factice-exec.py" "$PORT_EXEC" "$FACTLOG/factice-exec.log" > "$FACTLOG/factice-exec.err" 2>&1 &
PID_FEXEC=$!
python3 "$KIT/factice-recep.py" "$PORT_RECEP" "$FACTLOG/factice-recep.log" > "$FACTLOG/factice-recep.err" 2>&1 &
PID_FRECEP=$!
sleep 1
kill -0 "$PID_FEXEC" 2>/dev/null && kill -0 "$PID_FRECEP" 2>/dev/null
verif $? "factices demarres (exec=$PID_FEXEC, recep=$PID_FRECEP)"
# les factices ne sont JAMAIS tues par tuer_par_pid : verification finale
# qu'ils sont TOUJOURS vivants (scenario P) apres chaque arret de coordinateur.

EPEX="http://127.0.0.1:$PORT_EXEC/"; RPX="http://127.0.0.1:$PORT_RECEP/"
APPELS_FEXEC_AVANT=$(grep -c POST "$FACTLOG/factice-exec.log" 2>/dev/null || echo 0)

# ---------- S1 : scenario V (volontaires au contenu IDENTIQUE, D4) ----------
echo "== S1 (V) : deux volontaires identiques -> identites DISTINCTES"
harnais -data "$DATA" -issue "$ISSUE_N" -mode creer > "$TRACES/v-01-creer.log" 2>&1
verif $? "creation mission $ISSUE_N"
harnais -data "$DATA" -issue "$ISSUE_N" -mode critiquer -plan "$KIT/plan-flow01.txt" > "$TRACES/v-02-critique1.log" 2>&1
verif $? "critique 1 PLAN_ACCEPTE"
ID_V1=$(harnais -data "$DATA" -issue "$ISSUE_N" -mode derniere 2>/dev/null)
harnais -data "$DATA" -issue "$ISSUE_N" -mode critiquer -plan "$KIT/plan-flow01.txt" > "$TRACES/v-03-critique2.log" 2>&1
verif $? "critique 2 (contenu IDENTIQUE, D4)"
ID_V2=$(harnais -data "$DATA" -issue "$ISSUE_N" -mode derniere 2>/dev/null)
[ -n "$ID_V1" ] && [ -n "$ID_V2" ] && [ "$ID_V1" != "$ID_V2" ]
verif $? "deux identites DISTINCTES pour un contenu identique ($ID_V1 vs $ID_V2)"
grep -q '"id_operation"' "$DATA/missions/$ISSUE_N/journal-mission.jsonl" 2>/dev/null \
  || grep -q 'id_operation' "$DATA"/*/journal*.jsonl 2>/dev/null \
  || contient "$(find "$DATA" -name '*.jsonl' | head -1)" 'id_operation'
verif $? "journal corrélé (id_operation present, D3)"

# ---------- S2 : scenario N + P (nominal avec arret reel + pont vivant) ----------
echo "== S2 (N+P) : nominal, SIGKILL du coordinateur, pont vivant, reprise"
harnais -data "$DATA" -issue "$ISSUE_N" -mode executer -mandat "$KIT/mandat-flow01.txt" -sortie "$TRAVAIL/livrable-r1.txt" > "$TRACES/n-01-execution.log" 2>&1
verif $? "execution v1"
harnais -data "$DATA" -issue "$ISSUE_N" -mode soumettre -livrable "$TRAVAIL/livrable-r1.txt" > "$TRACES/n-02-soumission.log" 2>&1
verif $? "soumission r1"
harnais -data "$DATA" -issue "$ISSUE_N" -mode recevoir -livrable "$TRAVAIL/livrable-r1.txt" > "$TRACES/n-03-reception.log" 2>&1
contient "$TRACES/n-03-reception.log" "A_CORRIGER"
verif $? "verdict A_CORRIGER (mission EN_CORRECTION)"
# coordinateur VIVANT cible de l'arret reel (temoin PID/PGID, T3)
harnais -data "$DATA" -issue "$ISSUE_N" -mode vivant -secondes 300 -temoin "$TEMOINS/coordinateur-n.txt" > "$TRACES/n-04-vivant.log" 2>&1 &
sleep 1
tuer_par_pid "$TEMOINS/coordinateur-n.txt" "harnais-flow01"
verif $? "arret REEL du coordinateur (PID temoin, appartenance verifiee)"
kill -0 "$PID_FEXEC" 2>/dev/null && kill -0 "$PID_FRECEP" 2>/dev/null
verif $? "P : les deux factices (ponts) sont TOUJOURS VIVANTS"
# reprise : MEME identifiant, correction lue depuis le disque
harnais -data "$DATA" -issue "$ISSUE_N" -mode creer > "$TRACES/n-05-reprise.log" 2>&1
contient "$TRACES/n-05-reprise.log" "existante" && contient "$TRACES/n-05-reprise.log" "EN_CORRECTION"
verif $? "reprise meme identifiant, etat EN_CORRECTION conserve"
printf 'MANDAT V2 : corriger le livrable selon le point du verdict relu depuis le disque.\n' > "$TRAVAIL/mandat-v2.txt"
harnais -data "$DATA" -issue "$ISSUE_N" -mode executer -mandat "$TRAVAIL/mandat-v2.txt" -sortie "$TRAVAIL/livrable-r2.txt" > "$TRACES/n-06-execution-v2.log" 2>&1
verif $? "execution corrective v2"
harnais -data "$DATA" -issue "$ISSUE_N" -mode soumettre -livrable "$TRAVAIL/livrable-r2.txt" > "$TRACES/n-07-soumission.log" 2>&1
verif $? "soumission r2"
harnais -data "$DATA" -issue "$ISSUE_N" -mode recevoir -livrable "$TRAVAIL/livrable-r2.txt" > "$TRACES/n-08-reception.log" 2>&1
contient "$TRACES/n-08-reception.log" "ACCEPTEE"
verif $? "verdict ACCEPTEE"
harnais -data "$DATA" -issue "$ISSUE_N" -mode acker > "$TRACES/n-09-ack.log" 2>&1
verif $? "ack (ACK_RECU)"
harnais -data "$DATA" -issue "$ISSUE_N" -mode cloturer > "$TRACES/n-10-cloture.log" 2>&1
contient "$TRACES/n-10-cloture.log" "CLOTURE_REFUSEE"
verif $? "cloture reservee au pilote (refusee par construction)"

# ---------- S4 : scenarios E + X (echouee rejouable ; charge divergente refusee) ----------
echo "== S4 (E/X) : operation ECHOUEE, charge divergente, rejeu meme id cle neuve"
harnais -data "$DATA" -issue "$ISSUE_X" -mode creer > "$TRACES/x-01-creer.log" 2>&1
verif $? "creation mission $ISSUE_X"
printf 'MANDAT FLOW01 ESSAI X : contenu identique pour E et X.\n' > "$TRAVAIL/mandat-x.txt"
# appel vers un endpoint MORT -> ECHOUEE explicite
harnais -data "$DATA" -issue "$ISSUE_X" -mode executer -exec-endpoint "http://127.0.0.1:8099/" -mandat "$TRAVAIL/mandat-x.txt" > "$TRACES/x-02-echec.log" 2>&1
contient "$TRACES/x-02-echec.log" "échec\|échouée\|refusé\|connection"
verif $? "appel echoue (endpoint mort) -> ECHOUEE"
ID_ECH=$(harnais -data "$DATA" -issue "$ISSUE_X" -mode derniere 2>/dev/null)
CMP_X=$(harnais -data "$DATA" -issue "$ISSUE_X" -mode compter 2>/dev/null)
contient <(echo "$CMP_X") '"echouees":1'
verif $? "registre : 1 operation ECHOUEE"
# X : rejeu a charge DIVERGENTE -> refus AVANT tout appel
printf 'AUTRE CHARGE DIVERGENTE.\n' > "$TRAVAIL/mandat-x-divergent.txt"
harnais -data "$DATA" -issue "$ISSUE_X" -mode executer -rejeu "$ID_ECH" -mandat "$TRAVAIL/mandat-x-divergent.txt" > "$TRACES/x-03-rejeu-divergent.log" 2>&1
contient "$TRACES/x-03-rejeu-divergent.log" "OPERATION_INCOMPATIBLE"
verif $? "X : OPERATION_INCOMPATIBLE avant tout appel"
# E : rejeu MEME id, MEME charge, endpoint retabli -> succes, cle NEUVE
harnais -data "$DATA" -issue "$ISSUE_X" -mode executer -rejeu "$ID_ECH" -mandat "$TRAVAIL/mandat-x.txt" > "$TRACES/x-04-rejeu-ok.log" 2>&1
verif $? "E : relance meme identite aboutit"
ID_ECH2=$(harnais -data "$DATA" -issue "$ISSUE_X" -mode derniere 2>/dev/null)
cmp_chaine "$ID_ECH" "$ID_ECH2"
verif $? "E : la derniere operation reste la MEME identite ($ID_ECH)"
JOURNAL_X=$(find "$DATA" -path "*$ISSUE_X*" -name '*.jsonl' ! -name 'operations*' | head -1)
NB_CLES=$(grep -F '"id_operation":"'"$ID_ECH"'"' "$JOURNAL_X" | grep -c 'APPEL_RESERVE')
[ "$NB_CLES" -eq 2 ]
verif $? "E : deux tentatives distinctes, cles NEUVES (2 APPEL_RESERVE, N-19/#75)"

# ---------- S3 (deplace apres S4) : scenario R, rejeu d'une TERMINEE ----------
echo "== S3 (R) : rejeu d'une operation TERMINEE -> sans effet"
CMP_AVANT=$(harnais -data "$DATA" -issue "$ISSUE_X" -mode compter 2>/dev/null)
APPELS_AVANT_R=$(grep -c POST "$FACTLOG/factice-exec.log" 2>/dev/null || echo 0)
harnais -data "$DATA" -issue "$ISSUE_X" -mode executer -rejeu "$ID_ECH" -mandat "$TRAVAIL/mandat-x.txt" > "$TRACES/r-01-rejeu.log" 2>&1
RC=$?
contient "$TRACES/r-01-rejeu.log" "OPERATION_DEJA_TERMINEE"
verif $? "OPERATION_DEJA_TERMINEE (RC=$RC attendu != 0)"
APRES_R=$(grep -c POST "$FACTLOG/factice-exec.log" 2>/dev/null || echo 0)
cmp_chaine "$APRES_R" "$APPELS_AVANT_R"
verif $? "zero appel factice (R)"
CMP_APRES=$(harnais -data "$DATA" -issue "$ISSUE_X" -mode compter 2>/dev/null)
cmp_chaine "$CMP_AVANT" "$CMP_APRES"
verif $? "zero effet sur le registre (comptage identique, R)"

# ---------- S5 : scenario I (interruption pendant l'appel, resultat incertain) ----------
echo "== S5 (I) : SIGKILL pendant l'appel reelle -> incertain, arbitrage pilote"
printf 'MANDAT LENT FLOW01 : appel lent (4 s) pour tuer le coordinateur en cours d appel.\n' > "$TRAVAIL/mandat-lent.txt"
harnais -data "$DATA" -issue "$ISSUE_I" -mode creer > "$TRACES/i-01-creer.log" 2>&1
verif $? "creation mission $ISSUE_I"
harnais -data "$DATA" -issue "$ISSUE_I" -mode executer -mandat "$TRAVAIL/mandat-lent.txt" -temoin "$TEMOINS/coordinateur-i.txt" -sortie "$TRAVAIL/livrable-i.txt" > "$TRACES/i-02-execution-tuee.log" 2>&1 &
sleep 2
tuer_par_pid "$TEMOINS/coordinateur-i.txt" "harnais-flow01"
verif $? "SIGKILL pendant l'appel (TX1 fait, appel en cours)"
# incertain => blocage (jamais de nouvel appel automatique)
harnais -data "$DATA" -issue "$ISSUE_I" -mode executer -mandat "$TRAVAIL/mandat-lent.txt" > "$TRACES/i-03-blocage.log" 2>&1
contient "$TRACES/i-03-blocage.log" "APPEL_EN_COURS_NON_RESOLU"
verif $? "incertain : nouvel appel BLOQUE (arbitrage pilote requis)"
ID_I=$(harnais -data "$DATA" -issue "$ISSUE_I" -mode derniere 2>/dev/null)
harnais -data "$DATA" -issue "$ISSUE_I" -mode resoudre -resolution RECU -motif "recette I : reponse orpheline arrivee (donnee de recette SIMULEE)" > "$TRACES/i-04-resolution.log" 2>&1
verif $? "resolution pilote RECU (SIMULEE, identifiee comme donnee de recette)"
harnais -data "$DATA" -issue "$ISSUE_I" -mode executer -rejeu "$ID_I" -mandat "$TRAVAIL/mandat-lent.txt" > "$TRACES/i-05-rejeu.log" 2>&1
contient "$TRACES/i-05-rejeu.log" "OPERATION_NON_REJOUABLE"
verif $? "rejeu de l'interrompue REFUSE (RESULTAT_RECU non persiste)"
harnais -data "$DATA" -issue "$ISSUE_I" -mode executer -mandat "$TRAVAIL/mandat-v2.txt" -sortie "$TRAVAIL/livrable-i2.txt" > "$TRACES/i-06-nouvelle.log" 2>&1
verif $? "nouvelle operation VOLONTAIRE possible (voie normale, D4)"

# ---------- controles finaux ----------
echo "== controles finaux"
kill -0 "$PID_FEXEC" 2>/dev/null && kill -0 "$PID_FRECEP" 2>/dev/null
verif $? "P : factices intacts du debut a la fin (aucun dommage colleteral)"
python3 "$KIT/controles-flow01.py" "$DATA" "$ISSUE_N" "$ISSUE_X" "$ISSUE_I" > "$TRACES/controles.log" 2>&1
verif $? "controles opposes (correlation, doublons, registre)"
cat "$TRACES/controles.log"

echo "== arret des factices (fin de recette, par PID direct connu)"
kill "$PID_FEXEC" "$PID_FRECEP" 2>/dev/null
wait "$PID_FEXEC" "$PID_FRECEP" 2>/dev/null

echo "== archivage"
tar czf "$TRAVAIL/archive-recette-flow01.tar.gz" -C "$TRAVAIL" traces temoins factices 2>/dev/null
[ -f "$TRAVAIL/archive-recette-flow01.tar.gz" ]
verif $? "archive des traces/temoins/factices"
sha256sum "$TRAVAIL/archive-recette-flow01.tar.gz" | tee "$TRAVAIL/archive.sha256"

echo ""
echo "BILAN RECETTE FLOW-01 : PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
