#!/bin/bash
# Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
#
# recette-oa81.sh — RECETTE SIMULÉE M-COORD-OPENAI-VPS-01 (issue #81, adaptation du kit mvps01 #78, même moteur main 50f86b90).
# Boucle 100 % loopback, 0 €, aucune clé vraie. Exécution sous le compte
# `coord` :  sudo -u coord bash recette-oa81.sh
# Sections : S1 rejets pont — S2 cycle nominal + REJEU sans nouvel appel —
# S3 arrêt réel pendant l'appel + issue incertaine — S4 rejets moteur via la
# chaîne mauvais — S5 plafond avant envoi — S6 contrôles.
# Arrêt : TOUJOURS par PID vérifié (/proc/<pid>/cmdline), jamais par groupe.

set -u
umask 027

MV=/srv/coord/private/workspace/oa81
STATE=/srv/coord/private/state/oa81
TR=/srv/coord/private/recette/oa81
RUN="$TR/run-$(date -u +%Y%m%dT%H%M%SZ)"
TRACES="$TR/traces"
HARN="$MV/bin/harnais-mvps01"
PY="$MV/venv/bin/python3"
JETON="recette-oa81-jetons-factice"
PONT1=8790
PONT2=8795
MAUVAIS_PORT=8792
JOURNAL_PONT1="$TRACES/pont-factice.jsonl"
JOURNAL_PONT2="$TRACES/pont-mauvais.jsonl"
JOURNAL_MAUVAIS="$TRACES/mauvais.jsonl"

PASS=0; FAIL=0
declare -a RESUME

mkdir -p "$RUN" "$TRACES" "$STATE"
cd "$RUN"

log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$RUN/recette.log"; }
ok()   { PASS=$((PASS+1)); log "PASS $*"; RESUME+=("PASS $*"); }
ko()   { FAIL=$((FAIL+1)); log "FAIL $*"; RESUME+=("FAIL $*"); }
verif() { if [ "$1" = "0" ]; then ok "$2"; else ko "$2 (rc=$1)"; fi; }
contient() { printf '%s' "$1" | grep -q "$2"; }
fcontient() { grep -q "$2" "$1" 2>/dev/null; }

# tuer_par_pid : arrêt PAR PID UNIQUEMENT, après vérification de l'identité
# du processus via /proc/<pid>/cmdline (jamais kill de groupe en recette).
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

nl_journal() { [ -f "$1" ] && wc -l < "$1" || echo 0; }

harn() { # harn <mode> <issue> [args...]
  local mode="$1" issue="$2"; shift 2
  COORD_BRIDGE_JETON="$JETON" "$HARN" -data "$STATE" -issue "$issue" -mode "$mode" "$@"
}

# ---------------------------------------------------------------------------
# Empreintes initiales (traçabilité opposable)
# ---------------------------------------------------------------------------
{
  echo "== empreintes $(date -u +%Y-%m-%dT%H:%M:%SZ) =="
  sha256sum "$HARN" "$MV/bin/coord-mission" \
            "$MV/src/coordengine/cmd/harnais-mvps01/main.go" \
            "$MV/mauvais.py" "$MV/config-mauvais.json" "$0" 2>&1
} > "$RUN/empreintes.txt"

# ---------------------------------------------------------------------------
# Démarrage des serveurs loopback (PID relevés AVANT toute action)
# ---------------------------------------------------------------------------
PIDS_SERVEURS=""

MAUVAIS_JOURNAL="$JOURNAL_MAUVAIS" "$PY" "$MV/mauvais.py" "$MAUVAIS_PORT" \
  > "$RUN/mauvais.out" 2>&1 &
PID_MAUVAIS=$!
sleep 0.5
kill -0 "$PID_MAUVAIS" 2>/dev/null && ok "mauvais.py 8792 démarré (pid $PID_MAUVAIS)" \
  || ko "mauvais.py 8792 NON démarré"

COORD_BRIDGE_JETON="$JETON" COORD_BRIDGE_CLE_FACTICE="factice-cle-pont" \
COORD_CLE_MAUVAIS="factice-cle-mauvais" \
COORD_BRIDGE_JOURNAL="$JOURNAL_PONT2" \
  "$PY" "$MV/llmbridge/bridge.py" --config "$MV/config-mauvais.json" \
  --port "$PONT2" > "$RUN/pont2.out" 2>&1 &
PID_PONT2=$!
sleep 0.8
kill -0 "$PID_PONT2" 2>/dev/null && ok "pont mauvais 8795 démarré (pid $PID_PONT2)" \
  || ko "pont mauvais 8795 NON démarré"

COORD_BRIDGE_JETON="$JETON" COORD_BRIDGE_CLE_FACTICE="factice-cle-pont" \
COORD_BRIDGE_JOURNAL="$JOURNAL_PONT1" \
  "$PY" "$MV/llmbridge/bridge.py" --factice --port "$PONT1" \
  > "$RUN/pont1.out" 2>&1 &
PID_PONT1=$!
sleep 0.8
kill -0 "$PID_PONT1" 2>/dev/null && ok "pont factice 8790 démarré (pid $PID_PONT1)" \
  || ko "pont factice 8790 NON démarré"

code_ping=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $JETON" \
  "http://127.0.0.1:$PONT1/ping")
[ "$code_ping" = "200" ] && ok "GET /ping 200" || ko "GET /ping code $code_ping"

PIDS_SERVEURS="$PID_MAUVAIS $PID_PONT2 $PID_PONT1"

# ---------------------------------------------------------------------------
# S1 — Rejets du pont coord.llm.v1 (contrat, fail closed)
# ---------------------------------------------------------------------------
log "== S1 rejets pont 8790 =="
P1=$(mktemp)
printf '{"model":"factice/exec/x","messages":[{"role":"user","content":"x"}]}' > "$P1"

code=$(curl -s -o "$RUN/s1-401.json" -w '%{http_code}' \
  -H "Authorization: Bearer jeton-faux-recette" \
  -H "Content-Type: application/json" \
  --data-binary "@$P1" "http://127.0.0.1:$PONT1/v1/chat/completions")
[ "$code" = "401" ] && fcontient "$RUN/s1-401.json" "JETON_INVALIDE" \
  && ok "S1 401 JETON_INVALIDE" || ko "S1 401 attendu, obtenu $code"

printf '{pas du json' > "$P1"
code=$(curl -s -o "$RUN/s1-json.json" -w '%{http_code}' \
  -H "Authorization: Bearer $JETON" -H "Content-Type: application/json" \
  --data-binary "@$P1" "http://127.0.0.1:$PONT1/v1/chat/completions")
[ "$code" = "400" ] && fcontient "$RUN/s1-json.json" "JSON_INVALIDE" \
  && ok "S1 400 JSON_INVALIDE" || ko "S1 400 JSON attendu, obtenu $code"

printf '{"model":"openai/interdit","messages":[{"role":"user","content":"x"}]}' > "$P1"
code=$(curl -s -o "$RUN/s1-dest.json" -w '%{http_code}' \
  -H "Authorization: Bearer $JETON" -H "Content-Type: application/json" \
  --data-binary "@$P1" "http://127.0.0.1:$PONT1/v1/chat/completions")
[ "$code" = "400" ] && fcontient "$RUN/s1-dest.json" "DESTINATION_REFUSEE" \
  && ok "S1 400 DESTINATION_REFUSEE" || ko "S1 DESTINATION_REFUSEE attendue, obtenu $code"

code=$(curl -s -o "$RUN/s1-405.json" -w '%{http_code}' -X DELETE \
  -H "Authorization: Bearer $JETON" \
  "http://127.0.0.1:$PONT1/v1/chat/completions")
[ "$code" = "405" ] && fcontient "$RUN/s1-405.json" "METHODE_NON_AUTORISEE" \
  && ok "S1 405 METHODE_NON_AUTORISEE" || ko "S1 405 attendu, obtenu $code"

head -c 3145728 /dev/zero | tr '\0' 'a' > "$RUN/s1-gros.corps"
code=$(curl -s -o "$RUN/s1-413.json" -w '%{http_code}' \
  -H "Authorization: Bearer $JETON" -H "Content-Type: application/json" \
  --data-binary "@$RUN/s1-gros.corps" "http://127.0.0.1:$PONT1/v1/chat/completions")
[ "$code" = "413" ] && ok "S1 413 CORPS_TROP_GRAND" || ko "S1 413 attendu, obtenu $code"
rm -f "$RUN/s1-gros.corps" "$P1"

# ---------------------------------------------------------------------------
# S2 — Cycle nominal complet + REJEU D5 sans nouvel appel
# ---------------------------------------------------------------------------
log "== S2 cycle nominal temoin-1 + rejeu =="
M1=temoin-1
printf 'PLAN TEMOIN RECETTE MVPS01\nObjectif : livrable de demonstration COORD, 0 EUR, loopback.\n' > "$RUN/plan.txt"
printf 'MANDAT V1 — produire le livrable de demonstration (version initiale).\n' > "$RUN/mandat-v1.txt"
printf 'MANDAT V2 — corriger le livrable selon les points du verdict de reception.\n' > "$RUN/mandat-v2.txt"

harn creer "$M1" > "$RUN/$M1-creer.out" 2>&1; rc=$?
verif $rc "S2 creer $M1"
fcontient "$STATE/$M1/mission.json" '"State"' && ok "S2 état persisté (mission.json)" || ko "S2 mission.json inattendu"

harn critiquer "$M1" -plan "$RUN/plan.txt" > "$RUN/$M1-critique.out" 2>&1; rc=$?
[ $rc -eq 0 ] && fcontient "$RUN/$M1-critique.out" "PLAN_ACCEPTE" \
  && ok "S2 critique PLAN_ACCEPTE" || ko "S2 critique rc=$rc"

harn executer "$M1" -mandat "$RUN/mandat-v1.txt" -sortie "$RUN/livrable-v1.txt" \
  > "$RUN/$M1-exec1.out" 2>&1; rc=$?
verif $rc "S2 executer v1"
fcontient "$RUN/livrable-v1.txt" "SCENARIO:LIVRABLE_A_CORRIGER" \
  && ok "S2 livrable v1 avec marqueur d'erreur" || ko "S2 marqueur v1 absent"

# id_operation de l'exécution v1 (registre) et compteurs AVANT rejeu
J1=$(nl_journal "$JOURNAL_PONT1")
grep -i 'ouverte' "$STATE/$M1/operations.jsonl" | grep -i 'executer' | tail -1 > "$RUN/$M1-op-exec1.json"
OP1=$(grep -o '"id_operation":"[0-9a-f]*"' "$RUN/$M1-op-exec1.json" | head -1)
[ -n "$OP1" ] || OP1=$(grep -o '"id_operation": *"[0-9a-f]*"' "$STATE/$M1/operations.jsonl" | tail -1)
L1=$(nl_journal "$STATE/$M1/operations.jsonl")

harn rejeu "$M1" -mandat "$RUN/mandat-v1.txt" \
  > "$RUN/$M1-rejeu.out" 2>&1; rc=$?
J2=$(nl_journal "$JOURNAL_PONT1")
if [ $rc -ne 0 ] && fcontient "$RUN/$M1-rejeu.out" "OPERATION_DEJA_TERMINEE"; then
  ok "S2 rejeu d'une opération TERMINÉE : refus explicite D5 (rc=$rc, zéro appel, zéro budget)"
else
  ko "S2 rejeu terminée : rc=$rc, message inattendu"
fi
[ "$J1" = "$J2" ] && ok "S2 rejeu SANS nouvel appel pont ($J1 lignes inchangées)" \
  || ko "S2 rejeu : journal pont $J1 -> $J2"
L2=$(nl_journal "$STATE/$M1/operations.jsonl")
NV=$((L2 - L1))
[ "$NV" = "1" ] && ok "S2 rejeu : exactement 1 entrée OPERATION_REJETEE au registre" \
  || ko "S2 rejeu : registre $L1 -> $L2 (attendu +1)"
tail -1 "$STATE/$M1/operations.jsonl" | grep -q "OPERATION_REJETEE" && grep -q "$OP1" <(tail -1 "$STATE/$M1/operations.jsonl") \
  && ok "S2 rejeu : id_operation CONSERVÉ dans le refus ($OP1)" \
  || ko "S2 rejeu : id non conservé dans l'entrée rejetée"
OUV_AVANT=$(grep -c "OPERATION_OUVERTE" "$STATE/$M1/operations.jsonl")
harn rejeu "$M1" -mandat "$RUN/mandat-v1.txt" > /dev/null 2>&1
OUV_APRES=$(grep -c "OPERATION_OUVERTE" "$STATE/$M1/operations.jsonl")
[ "$OUV_AVANT" = "$OUV_APRES" ] && ok "S2 rejeu : AUCUNE nouvelle opération OUVERTE" \
  || ko "S2 rejeu : OUVERTE $OUV_AVANT -> $OUV_APRES"

harn soumettre "$M1" -livrable "$RUN/livrable-v1.txt" > "$RUN/$M1-soumis1.out" 2>&1; rc=$?
verif $rc "S2 soumettre v1"
harn recevoir "$M1" -livrable "$RUN/livrable-v1.txt" > "$RUN/$M1-recep1.out" 2>&1; rc=$?
[ $rc -eq 2 ] && fcontient "$RUN/$M1-recep1.out" "A_CORRIGER" \
  && ok "S2 réception A_CORRIGER (rc 2 attendu)" || ko "S2 réception rc=$rc"
[ "$(grep -o "correction" "$RUN/$M1-recep1.out" | wc -l)" = "2" ] \
  && ok "S2 verdict A_CORRIGER à 2 points" || ko "S2 points du verdict inattendus"

harn executer "$M1" -mandat "$RUN/mandat-v2.txt" -sortie "$RUN/livrable-v2.txt" \
  > "$RUN/$M1-exec2.out" 2>&1; rc=$?
verif $rc "S2 executer v2"
harn soumettre "$M1" -livrable "$RUN/livrable-v2.txt" > "$RUN/$M1-soumis2.out" 2>&1; rc=$?
verif $rc "S2 soumettre v2"
harn recevoir "$M1" -livrable "$RUN/livrable-v2.txt" > "$RUN/$M1-recep2.out" 2>&1; rc=$?
[ $rc -eq 0 ] && fcontient "$RUN/$M1-recep2.out" "ACCEPTEE" \
  && ok "S2 réception ACCEPTEE" || ko "S2 réception v2 rc=$rc"
harn acker "$M1" > "$RUN/$M1-acker.out" 2>&1; rc=$?
verif $rc "S2 acker ACK_RECU"
harn cloturer "$M1" > "$RUN/$M1-cloture.out" 2>&1; rc=$?
[ $rc -eq 2 ] && ok "S2 clôture refusée par l'automatisme (CLOTURE_REFUSEE)" \
  || ko "S2 clôture rc=$rc inattendu"

if [ -f "$STATE/$M1/journal.jsonl" ]; then
  sort "$STATE/$M1/journal.jsonl" | uniq -d | grep -q . \
    && ko "S2 doublons dans le journal mission" \
    || ok "S2 journal mission sans doublon"
else
  ko "S2 journal mission absent"
fi

# ---------------------------------------------------------------------------
# S3 — Arrêt réel pendant l'appel + issue incertaine (fail closed)
# ---------------------------------------------------------------------------
log "== S3 arrêt réel pendant l'appel, mission temoin-2 =="
M2=temoin-2
harn creer "$M2" > "$RUN/$M2-creer.out" 2>&1; verif $? "S3 creer $M2"
harn critiquer "$M2" -plan "$RUN/plan.txt" > "$RUN/$M2-critique.out" 2>&1
verif $? "S3 critique $M2"
harn executer "$M2" -mandat "$RUN/mandat-v1.txt" -sortie "$RUN/$M2-livrable-v1.txt" \
  > "$RUN/$M2-exec1.out" 2>&1; verif $? "S3 executer v1"
harn soumettre "$M2" -livrable "$RUN/$M2-livrable-v1.txt" > "$RUN/$M2-soumis1.out" 2>&1
verif $? "S3 soumettre v1"

# SIGSTOP du pont : l'appel en vol se fige (fenêtre d'arrêt réel)
kill -STOP "$PID_PONT1"
COORD_BRIDGE_JETON="$JETON" "$HARN" -data "$STATE" -issue "$M2" -mode recevoir \
  -livrable "$RUN/$M2-livrable-v1.txt" > "$RUN/$M2-recep-avortee.out" 2>&1 &
PID_H=$!
sleep 1.2
tr '\0' ' ' < /proc/$PID_H/cmdline 2>/dev/null | grep -q "harnais-mvps01" \
  && kill -9 "$PID_H" && ok "S3 arrêt réel du harnais (pid $PID_H SIGKILL pendant l'appel)" \
  || ko "S3 harnais introuvable à l'arrêt"
kill -CONT "$PID_PONT1"
sleep 0.5

[ -f "$STATE/$M2/appel_en_cours.json" ] \
  && ok "S3 appel_en_cours persisté (issue non résolue)" \
  || ko "S3 appel_en_cours absent"

harn recevoir "$M2" -livrable "$RUN/$M2-livrable-v1.txt" > "$RUN/$M2-recep-refusee.out" 2>&1; rc=$?
if [ $rc -ne 0 ] && { fcontient "$RUN/$M2-recep-refusee.out" "APPEL_EN_COURS_NON_RESOLU" \
  || fcontient "$RUN/$M2-recep-refusee.out" "en cours"; }; then
  ok "S3 reprise auto REFUSÉE (APPEL_EN_COURS_NON_RESOLU)"
else
  ko "S3 reprise auto non refusée (rc=$rc)"
fi

harn resoudre "$M2" -resolution PERDU -motif "recette oa81 : appel avorté par arrêt réel" \
  > "$RUN/$M2-resoudre.out" 2>&1; verif $? "S3 résolution pilote PERDU"

harn recevoir "$M2" -livrable "$RUN/$M2-livrable-v1.txt" > "$RUN/$M2-recep1.out" 2>&1; rc=$?
[ $rc -eq 2 ] && fcontient "$RUN/$M2-recep1.out" "A_CORRIGER" \
  && ok "S3 réception A_CORRIGER après résolution" || ko "S3 réception rc=$rc"
harn executer "$M2" -mandat "$RUN/mandat-v2.txt" -sortie "$RUN/$M2-livrable-v2.txt" \
  > "$RUN/$M2-exec2.out" 2>&1; verif $? "S3 executer v2"
harn soumettre "$M2" -livrable "$RUN/$M2-livrable-v2.txt" > "$RUN/$M2-soumis2.out" 2>&1
verif $? "S3 soumettre v2"
harn recevoir "$M2" -livrable "$RUN/$M2-livrable-v2.txt" > "$RUN/$M2-recep2.out" 2>&1; rc=$?
[ $rc -eq 0 ] && fcontient "$RUN/$M2-recep2.out" "ACCEPTEE" \
  && ok "S3 réception ACCEPTEE" || ko "S3 réception v2 rc=$rc"
harn acker "$M2" > "$RUN/$M2-acker.out" 2>&1; verif $? "S3 acker"
harn cloturer "$M2" > "$RUN/$M2-cloture.out" 2>&1; rc=$?
[ $rc -eq 2 ] && ok "S3 clôture refusée" || ko "S3 clôture rc=$rc"

# rejeu EXPLICITE par id_operation de l'exécution v1 TERMINÉE, APRÈS l'arrêt réel :
# refus D5 sans nouvel appel, id_operation conservé (exigence de l'ordre #78)
OP2=$(grep '"action":"executer"' "$STATE/$M2/operations.jsonl" | grep OPERATION_OUVERTE \
  | head -1 | grep -o '"id_operation":"[0-9a-f]*"' | head -1 | cut -d'"' -f4)
JP_AV=$(nl_journal "$JOURNAL_PONT1")
COORD_BRIDGE_JETON="$JETON" "$HARN" -data "$STATE" -issue "$M2" -mode rejeu \
  -rejeu-cible "$OP2" -mandat "$RUN/mandat-v1.txt" > "$RUN/$M2-rejeu-id.out" 2>&1; rc=$?
JP_AP=$(nl_journal "$JOURNAL_PONT1")
if [ $rc -ne 0 ] && fcontient "$RUN/$M2-rejeu-id.out" "OPERATION_DEJA_TERMINEE"; then
  ok "S3 rejeu par id d'une opération terminée : refus D5 (zéro appel, zéro budget)"
else
  ko "S3 rejeu par id : rc=$rc inattendu"
fi
[ "$JP_AV" = "$JP_AP" ] && ok "S3 rejeu SANS nouvel appel fournisseur après l'arrêt réel" \
  || ko "S3 rejeu : appel passé ($JP_AV -> $JP_AP)"
tail -1 "$STATE/$M2/operations.jsonl" | grep -q "$OP2" \
  && ok "S3 rejeu : id_operation conservé ($OP2)" \
  || ko "S3 rejeu : id_operation divergent"

# ---------------------------------------------------------------------------
# S4 — Rejets moteur via la chaîne mauvais (pont 8795 -> mauvais.py 8792)
# ---------------------------------------------------------------------------
log "== S4 rejets moteur via chaîne mauvais =="
PONT_MAUVAIS="http://127.0.0.1:$PONT2/v1/chat/completions"

attendre_erreur_exec() { # issue modele retraits profil nb_requetes_attendues libelle
  local issue="$1" modele="$2" retraits="$3" profil="$4" nb="$5" lib="$6"
  harn creer "$issue" > "$RUN/$issue-creer.out" 2>&1
  harn critiquer "$issue" -plan "$RUN/plan.txt" > "$RUN/$issue-critique.out" 2>&1
  local avant apres
  avant=$(nl_journal "$JOURNAL_MAUVAIS")
  COORD_BRIDGE_JETON="$JETON" "$HARN" -data "$STATE" -issue "$issue" -mode executer \
    -mandat "$RUN/mandat-v1.txt" -pont "$PONT_MAUVAIS" \
    -modele-exec "$modele" -retraits "$retraits" ${profil:+-profil-exec "$profil"} \
    > "$RUN/$issue-exec.out" 2>&1
  local rc=$?
  apres=$(nl_journal "$JOURNAL_MAUVAIS")
  local vues=$((apres - avant))
  if [ $rc -ne 0 ] && [ "$vues" = "$nb" ]; then
    ok "$lib (rc=$rc, requêtes fournisseur=$vues)"
  else
    ko "$lib (rc=$rc, requêtes=$vues, attendues=$nb)"
  fi
}

attendre_erreur_exec temoin-3 "mauvais/429/oa81" 1 "" 2 "S4 429 retraits_max=1 : 2 tentatives fournisseur"
attendre_erreur_exec temoin-4 "mauvais/429/oa81" 0 "" 1 "S4 429 retraits_max=0 : 1 seule requête"
attendre_erreur_exec temoin-5 "mauvais/divergent/oa81" 0 "" 1 "S4 MODELE_DIVERGENT"
attendre_erreur_exec temoin-6 "mauvais/finish-length/oa81" 0 "mistral" 1 "S4 FIN_NON_NOMINAL (profil mistral)"
attendre_erreur_exec temoin-7 "mauvais/usage-absent/oa81" 0 "" 1 "S4 usage absent : APPEL_ECHOUE"

# rejeu D5 positif : l'opération ÉCHOUÉE de temoin-4 est REJOUABLE — nouvelle
# tentative FOURNISSEUR avec le MÊME id_operation (clé d'idempotence neuve)
OP4=$(grep '"action":"executer"' "$STATE/temoin-4/operations.jsonl" | grep OPERATION_OUVERTE \
  | head -1 | grep -o '"id_operation":"[0-9a-f]*"' | head -1 | cut -d'"' -f4)
MB_AV=$(nl_journal "$JOURNAL_MAUVAIS")
COORD_BRIDGE_JETON="$JETON" "$HARN" -data "$STATE" -issue temoin-4 -mode rejeu \
  -rejeu-cible "$OP4" -mandat "$RUN/mandat-v1.txt" -pont "$PONT_MAUVAIS" \
  -modele-exec "mauvais/429/oa81" -retraits 0 > "$RUN/temoin-4-rejeu.out" 2>&1; rc=$?
MB_AP=$(nl_journal "$JOURNAL_MAUVAIS")
if [ $rc -ne 0 ] && [ "$((MB_AP - MB_AV))" = "1" ]; then
  ok "S4 rejeu opération ÉCHOUÉE : relance MÊME id ($OP4), 1 nouvelle requête fournisseur"
else
  ko "S4 rejeu échouée : rc=$rc, requêtes $MB_AV -> $MB_AP"
fi
N_OUV=$(grep OPERATION_OUVERTE "$STATE/temoin-4/operations.jsonl" | grep -c '"action":"executer"')
IDS=$(grep OPERATION_OUVERTE "$STATE/temoin-4/operations.jsonl" | grep '"action":"executer"' \
  | grep -o '"id_operation":"[0-9a-f]*"' | sort -u | wc -l)
if [ "$N_OUV" = "2" ] && [ "$IDS" = "1" ]; then
  ok "S4 rejeu : 2 tentatives OUVERTES sous le MÊME id_operation ($OP4, clé d'idempotence neuve)"
else
  ko "S4 rejeu : OUVERTES=$N_OUV ids distincts=$IDS (id attendu $OP4)"
fi

fcontient "$RUN/temoin-5-exec.out" "MODELE_DIVERGENT" \
  && ok "S4 message moteur MODELE_DIVERGENT explicite" \
  || ko "S4 MODELE_DIVERGENT absent du message"
fcontient "$RUN/temoin-6-exec.out" "FIN_NON_NOMINAL" \
  && ok "S4 message moteur FIN_NON_NOMINAL explicite" \
  || ko "S4 FIN_NON_NOMINAL absent du message"
for i in 3 4 7; do
  fcontient "$STATE/temoin-$i/journal.jsonl" "APPEL_ECHOUE" \
    && ok "S4 APPEL_ECHOUE journalisé (temoin-$i)" \
    || ko "S4 APPEL_ECHOUE absent du journal (temoin-$i)"
done

# ---------------------------------------------------------------------------
# S5 — Plafond de jetons atteint AVANT tout envoi (voûte monétaire)
# ---------------------------------------------------------------------------
log "== S5 plafond avant envoi =="
M8=temoin-8
COORD_BRIDGE_JETON="$JETON" "$HARN" -data "$STATE" -issue "$M8" -mode creer \
  -plafond-jetons 50 > "$RUN/$M8-creer.out" 2>&1; verif $? "S5 creer plafond 50 jetons"
JAV=$(nl_journal "$JOURNAL_PONT1")
COORD_BRIDGE_JETON="$JETON" "$HARN" -data "$STATE" -issue "$M8" -mode executer \
  -mandat "$RUN/mandat-v1.txt" > "$RUN/$M8-exec.out" 2>&1; rc=$?
JAP=$(nl_journal "$JOURNAL_PONT1")
fcontient "$RUN/$M8-exec.out" "BUDGET_DEPASSE" \
  && ok "S5 exécution refusée BUDGET_DEPASSE (rc=$rc)" \
  || ko "S5 BUDGET_DEPASSE absent (rc=$rc)"
[ "$JAV" = "$JAP" ] && ok "S5 aucun appel envoyé au pont ($JAV lignes)" \
  || ko "S5 appel passé malgré le plafond ($JAV -> $JAP)"

# ---------------------------------------------------------------------------
# S6 — Contrôle final : isolation, secrets, empreintes, services, arrêts
# ---------------------------------------------------------------------------
log "== S6 contrôles =="
ss -ltn 2>/dev/null | grep "879[0-9]" | grep -v "127.0.0.1" | grep -q . \
  && ko "S6 écouteur 879x hors loopback" || ok "S6 tous les écouteurs 879x en loopback strict"

grep -rn "sk-\|AIza" "$STATE" "$TR" 2>/dev/null | grep -q . \
  && ko "S6 secret trouvé dans state/recette" || ok "S6 aucun secret dans state ni recette"

ST_MV=$(stat -c '%a' /srv/coord/private/workspace/oa81 2>/dev/null)
[ "$ST_MV" = "750" ] && ok "S6 perms oa81 0750" || ko "S6 perms oa81=$ST_MV"

# release fc1bac77 + MANIFEST intacts (lecture seule, empreintes consignées)
REL=$(ls -d /srv/coord/private/release* /srv/coord/workspace/release* 2>/dev/null | head -1)
if [ -n "$REL" ]; then
  sha256sum "$REL/MANIFEST" >> "$RUN/empreintes.txt" 2>/dev/null
  ok "S6 release présente, MANIFEST sha256 consigné ($REL)"
else
  ko "S6 release introuvable"
fi

systemctl is-active nginx >/dev/null 2>&1 \
  && ok "S6 nginx actif" || ko "S6 nginx NON actif"
code_https=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 https://coord.anhydra.fr)
[ "$code_https" = "200" ] && ok "S6 https://coord.anhydra.fr 200" \
  || ko "S6 coord.anhydra.fr code $code_https"

# arrêt des serveurs PAR PID VÉRIFIÉ (jamais par groupe)
tuer_par_pid "$PID_PONT1" "bridge.py" && ok "S6 pont factice arrêté (pid vérifié)"
tuer_par_pid "$PID_PONT2" "bridge.py" && ok "S6 pont mauvais arrêté (pid vérifié)"
tuer_par_pid "$PID_MAUVAIS" "mauvais.py" && ok "S6 mauvais.py arrêté (pid vérifié)"
sleep 0.5
ss -ltn 2>/dev/null | grep -q ":879[0-9]" \
  && ko "S6 écouteur 879x résiduel" || ok "S6 aucun écouteur 879x résiduel"

# ---------------------------------------------------------------------------
# Bilan
# ---------------------------------------------------------------------------
{
  echo "== BILAN $(date -u +%Y-%m-%dT%H:%M:%SZ) =="
  echo "PASS=$PASS FAIL=$FAIL"
  printf '%s\n' "${RESUME[@]}"
} | tee -a "$RUN/recette.log"

cp -f "$RUN/recette.log" "$TR/dernier-bilan.txt" 2>/dev/null
[ "$FAIL" = "0" ] && exit 0 || exit 1
