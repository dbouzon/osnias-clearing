#!/bin/bash
# Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
#
# recette-vault-oa81.sh — RECETTE MONÉTAIRE REEL 0 EUR M-COORD-OPENAI-VPS-01 (issue #81).
# Mode REEL derrière pont loopback (PR #80) avec témoin factice 8791 :
# R1 plafond insuffisant => refus AVANT envoi, témoin 0 requête ;
# R2 plafond suffisant => témoin exactement 1 requête, APPEL_RESERVE centimes_reserves:3 ;
# R3 épuisement => nouvelle soumission refusée, témoin inchangé ;
# R4 cycle REEL complet critiquer+executer+soumettre+recevoir+acker => verdict
#    ACCEPTEE validé, appliqué à la bonne révision, ACK_RECU, 4 réservations ;
# R5 contrôles finaux (secrets, loopback, arrêts, journaux).
# Exécution sous le compte coord : sudo -u coord bash recette-vault-oa81.sh

set -u
umask 027

MV=/srv/coord/private/workspace/oa81
STATE=/srv/coord/private/state/oa81
TR=/srv/coord/private/recette/oa81
RUN="$TR/run-vault-$(date -u +%Y%m%dT%H%M%SZ)"
HARN="$MV/bin/harnais-oa81-reel"
PY="$MV/venv/bin/python3"
JETON="recette-oa81-vault-jetons-factice"
PONT=8790
TEMOIN_PORT=8791
JOURNAL_PONT="$RUN/pont-journal.jsonl"
JOURNAL_TEMOIN="$RUN/temoin-journal.jsonl"

PASS=0; FAIL=0
declare -a RESUME
mkdir -p "$RUN" "$STATE"
cd "$RUN"

log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$RUN/recette.log"; }
ok()   { PASS=$((PASS+1)); log "PASS $*"; RESUME+=("PASS $*"); }
ko()   { FAIL=$((FAIL+1)); log "FAIL $*"; RESUME+=("FAIL $*"); }
verif() { if [ "$1" = "0" ]; then ok "$2"; else ko "$2 (rc=$1)"; fi; }
fcontient() { grep -q "$2" "$1" 2>/dev/null; }
nl_journal() { [ -f "$1" ] && wc -l < "$1" || echo 0; }

tuer_par_pid() {
  local pid="$1" attendu="$2"
  [ -n "$pid" ] || { log "tuer_par_pid : pid vide pour $attendu"; return 1; }
  if tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -q "$attendu"; then
    kill -9 "$pid" 2>/dev/null
    sleep 0.3
    kill -0 "$pid" 2>/dev/null && { ko "processus $attendu (pid $pid) encore vivant"; return 1; }
    return 0
  fi
  log "tuer_par_pid : pid $pid ne correspond pas à $attendu — NON TUÉ"
  return 1
}

harn() {
  local mode="$1" issue="$2"; shift 2
  COORD_MODE_REEL=1 COORD_BRIDGE_JETON="$JETON" COORD_CLE_TEMOIN="factice-cle-temoin-oa81" \
    "$HARN" -data "$STATE" -issue "$issue" -mode "$mode" \
    -config-reel "$MV/config-reel-oa81.json" "$@"
}

# empreintes initiales
{
  echo "== empreintes $(date -u +%Y-%m-%dT%H:%M:%SZ) =="
  sha256sum "$HARN" "$MV/config-reel-oa81.json" "$MV/config-oa81-vault.json" \
            "$MV/temoin-oa81.py" "$0"
} > "$RUN/empreintes.txt"

# serveurs loopback (PID relevés AVANT toute action)
COORD_BRIDGE_JETON="$JETON" COORD_BRIDGE_JOURNAL="$JOURNAL_PONT" \
COORD_CLE_TEMOIN="factice-cle-temoin-oa81" \
  "$PY" "$MV/llmbridge/bridge.py" --config "$MV/config-oa81-vault.json" \
  --port "$PONT" > "$RUN/pont.out" 2>&1 &
PID_PONT=$!
TEMOIN_OA81_JOURNAL="$JOURNAL_TEMOIN" "$PY" "$MV/temoin-oa81.py" "$TEMOIN_PORT" \
  > "$RUN/temoin.out" 2>&1 &
PID_TEMOIN=$!
sleep 1
kill -0 "$PID_PONT" 2>/dev/null && ok "pont vault 8790 démarré (pid $PID_PONT)" || ko "pont NON démarré"
kill -0 "$PID_TEMOIN" 2>/dev/null && ok "témoin 8791 démarré (pid $PID_TEMOIN)" || ko "témoin NON démarré"
code_ping=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $JETON" "http://127.0.0.1:$PONT/ping")
[ "$code_ping" = "200" ] && ok "GET /ping 200" || ko "GET /ping code $code_ping"

printf 'MANDAT V1 (recette oa81, livrable factice).' > "$RUN/mandat.txt"
printf 'PLAN V1 (recette oa81, plan factice).' > "$RUN/plan.txt"

# ---------------------------------------------------------------------------
log "== R1 plafond 2 c : refus AVANT envoi =="
harn creer oa81-vault-r1 -plafond-centimes 2 \
  > "$RUN/r1-creer.out" 2>&1; verif $? "R1 creer"
harn executer oa81-vault-r1 -mandat "$RUN/mandat.txt" -sortie "$RUN/r1-livrable.txt" \
  > "$RUN/r1-executer.out" 2>&1
rc=$?
if [ "$rc" != "0" ] && fcontient "$RUN/r1-executer.out" "BUDGET_DEPASSE"; then
  ok "R1 executer refusé BUDGET_DEPASSE"
else
  ko "R1 BUDGET_DEPASSE attendu, rc=$rc"
fi
[ "$(nl_journal "$JOURNAL_TEMOIN")" = "0" ] && ok "R1 témoin 0 requête (aucun envoi)" \
  || ko "R1 témoin a reçu $(nl_journal "$JOURNAL_TEMOIN") requête(s)"
grep -rq "APPEL_RESERVE" "$STATE/oa81-vault-r1/" \
  && ko "R1 réservation journalisée à tort" || ok "R1 aucun APPEL_RESERVE"

# ---------------------------------------------------------------------------
log "== R2 plafond 3 c : réservation journalisée, 1 requête =="
harn creer oa81-vault-r2 -plafond-centimes 3 \
  > "$RUN/r2-creer.out" 2>&1; verif $? "R2 creer"
harn executer oa81-vault-r2 -mandat "$RUN/mandat.txt" -sortie "$RUN/r2-livrable.txt" \
  > "$RUN/r2-executer.out" 2>&1; verif $? "R2 executer (appel réel factice)"
[ "$(nl_journal "$JOURNAL_TEMOIN")" = "1" ] && ok "R2 témoin exactement 1 requête" \
  || ko "R2 témoin $(nl_journal "$JOURNAL_TEMOIN") requête(s) (attendu 1)"
grep -q '"model": *"oa81/temoin/exec"' "$JOURNAL_TEMOIN" 2>/dev/null \
  && ok "R2 modèle demandé = oa81/temoin/exec" || ko "R2 modèle divergent au témoin"
grep -rqE '"centimes_reserves": *3' "$STATE/oa81-vault-r2/" \
  && ok "R2 APPEL_RESERVE centimes_reserves:3 journalisé" \
  || ko "R2 réservation 3 c absente du journal"

# ---------------------------------------------------------------------------
log "== R3 épuisement : soumission refusée, témoin inchangé =="
harn soumettre oa81-vault-r2 -livrable "$RUN/r2-livrable.txt" \
  > "$RUN/r3-soumettre.out" 2>&1
rc=$?
if [ "$rc" != "0" ] && fcontient "$RUN/r3-soumettre.out" "BUDGET_DEPASSE"; then
  ok "R3 soumission refusée BUDGET_DEPASSE (3+3>3)"
else
  ko "R3 BUDGET_DEPASSE attendu, rc=$rc"
fi
[ "$(nl_journal "$JOURNAL_TEMOIN")" = "1" ] && ok "R3 témoin inchangé (1 requête)" \
  || ko "R3 témoin a reçu une requête supplémentaire"

# ---------------------------------------------------------------------------
BASE_TEMOIN=$(nl_journal "$JOURNAL_TEMOIN")
log "== R4 cycle REEL complet : verdict validé, appliqué, ACK_RECU (base témoin=$BASE_TEMOIN) =="
harn creer oa81-vault-r4 -plafond-centimes 12 \
  > "$RUN/r4-creer.out" 2>&1; verif $? "R4 creer"
harn critiquer oa81-vault-r4 -plan "$RUN/plan.txt" > "$RUN/r4-critiquer.out" 2>&1
rc=$?
if [ "$rc" = "0" ] && fcontient "$RUN/r4-critiquer.out" "PLAN_ACCEPTE"; then
  ok "R4 critique de plan PLAN_ACCEPTE (relecteur REEL via pont)"
else
  ko "R4 critique de plan rc=$rc"
fi
grep -q '"model": *"gpt-4o-mini-2024-07-18"' "$JOURNAL_TEMOIN" 2>/dev/null \
  && ok "R4 relecteur appelé avec l'identité exacte gpt-4o-mini-2024-07-18" \
  || ko "R4 identité du relecteur divergente au témoin"
harn executer oa81-vault-r4 -mandat "$RUN/mandat.txt" -sortie "$RUN/r4-livrable.txt" \
  > "$RUN/r4-executer.out" 2>&1; verif $? "R4 executer"
harn soumettre oa81-vault-r4 -livrable "$RUN/r4-livrable.txt" \
  > "$RUN/r4-soumettre.out" 2>&1; verif $? "R4 soumettre (révision commitée)"
harn recevoir oa81-vault-r4 -livrable "$RUN/r4-livrable.txt" \
  > "$RUN/r4-recevoir.out" 2>&1
rc=$?
if [ "$rc" = "0" ] && fcontient "$RUN/r4-recevoir.out" "ACCEPTEE"; then
  ok "R4 réception ACCEPTEE validée par le moteur"
else
  ko "R4 réception rc=$rc"
fi
grep -rqE '"type": *"RECEPTION"' "$STATE/oa81-vault-r4/" \
  && ok "R4 RECEPTION journalisée (verdict appliqué à la soumission en attente)" \
  || ko "R4 RECEPTION absente du journal"
harn acker oa81-vault-r4 > "$RUN/r4-acker.out" 2>&1; verif $? "R4 acker"
grep -rqE '"type": *"ACK"' "$STATE/oa81-vault-r4/" && ok "R4 ACK journalise (etat final ACK_RECU)" \
  || ko "R4 ACK absent du journal"
DELTA_TEMOIN=$(( $(nl_journal "$JOURNAL_TEMOIN") - BASE_TEMOIN ))
[ "$DELTA_TEMOIN" = "4" ] && ok "R4 témoin exactement 4 requêtes (critique+exec+soumission+réception)" \
  || ko "R4 témoin $DELTA_TEMOIN requête(s) (attendu 4)"
nres=$(grep -c '"type": *"APPEL_RESERVE"' "$STATE/oa81-vault-r4/journal.jsonl" 2>/dev/null || echo 0)
[ "$nres" = "4" ] && ok "R4 4 APPEL_RESERVE journalisés" || ko "R4 $nres APPEL_RESERVE (attendu 4)"

# ---------------------------------------------------------------------------
log "== R5 contrôles finaux =="
grep -rn "sk-\|AIza" "$STATE" "$RUN" 2>/dev/null | grep -q . \
  && ko "R5 secret trouvé" || ok "R5 aucun secret dans state/run"
ss -ltn 2>/dev/null | grep -E ":879[01]" | grep -v "127.0.0.1" | grep -q . \
  && ko "R5 écouteur hors loopback" || ok "R5 écouteurs 8790/8791 en loopback strict"
tuer_par_pid "$PID_PONT" "bridge.py" && ok "R5 pont arrêté (pid vérifié)"
tuer_par_pid "$PID_TEMOIN" "temoin-oa81.py" && ok "R5 témoin arrêté (pid vérifié)"
sleep 0.5
ss -ltn 2>/dev/null | grep -q ":879[01]" && ko "R5 écouteur résiduel" || ok "R5 aucun écouteur 8790/8791 résiduel"
systemctl is-active nginx >/dev/null 2>&1 && ok "R5 nginx actif" || ko "R5 nginx NON actif"

{
  echo "PASS=$PASS FAIL=$FAIL"
  printf '%s\n' "${RESUME[@]}"
} | tee "$RUN/dernier-bilan-vault.txt"

[ "$FAIL" = "0" ]
