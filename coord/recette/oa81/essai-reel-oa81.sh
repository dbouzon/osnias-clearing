#!/bin/bash
# Copyright © 2026 Denis Bouzon — ORCID: https://orcid.org/0009-0007-8894-8902
# SPDX-License-Identifier: Apache-2.0
#
# essai-reel-oa81.sh — ESSAI RÉEL M-COORD-OPENAI-VPS-01 (issue #81, étape 4).
# UN SEUL appel facturable OpenAI (gpt-4o-mini-2024-07-18, plafond 5 c,
# retraits 0). Argument : rep (répétition 0 € avec témoin factice 8791)
# ou reel (appel réel ; exige OPENAI_API_KEY dans l'environnement).
# La clé n'est JAMAIS écrite sur le disque ni journalisée : uniquement env.
# Soumission produite en simulation (pont factice 8793), réception réelle
# via le pont coord.llm.v1 8790. Suspension sans renvoi automatique.
# Exécution : sudo -u coord bash essai-reel-oa81.sh rep|reel

set -u
umask 027

MODE_ESSAI="${1:-rep}"
MV=/srv/coord/private/workspace/oa81
STATE=/srv/coord/private/state/oa81
TR=/srv/coord/private/recette/oa81
RUN="$TR/essai-$MODE_ESSAI-$(date -u +%Y%m%dT%H%M%SZ)"
HARN_SIM="$MV/bin/harnais-mvps01"
HARN_REEL="$MV/bin/harnais-oa81-reel"
PY="$MV/venv/bin/python3"
JETON="essai-oa81-jetons-factice"
ISSUE="oa81-essai-$MODE_ESSAI"
PONT_FACTICE=8793
PONT=8790
TEMOIN_PORT=8791

if [ "$MODE_ESSAI" = "reel" ]; then
  CONFIG_PONT="$MV/config-pont-essai-reel.json"
  if [ -z "${OPENAI_API_KEY:-}" ]; then
    echo "NON_EXECUTE : OPENAI_API_KEY absente de l'environnement (refus, aucun appel)"
    exit 3
  fi
else
  CONFIG_PONT="$MV/config-pont-essai-rep.json"
fi

PASS=0; FAIL=0
declare -a RESUME
mkdir -p "$RUN" "$STATE"
cd "$RUN"

# pré-contrôles : aucun écouteur résiduel, état d'essai vierge (idempotence)
if ss -ltn 2>/dev/null | grep -qE ":$PONT_FACTICE |:$PONT |:$TEMOIN_PORT "; then
  echo "NON_EXECUTE : écouteur résiduel sur 879x — SUSPENDRE (nettoyage requis)"
  exit 3
fi
rm -rf "$STATE/$ISSUE"

log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$RUN/essai.log"; }
ok()   { PASS=$((PASS+1)); log "PASS $*"; RESUME+=("PASS $*"); }
ko()   { FAIL=$((FAIL+1)); log "FAIL $*"; RESUME+=("FAIL $*"); }
verif() { if [ "$1" = "0" ]; then ok "$2"; else ko "$2 (rc=$1)"; fi; }
fcontient() { grep -q "$2" "$1" 2>/dev/null; }
nl_journal() { [ -f "$1" ] && wc -l < "$1" || echo 0; }
tuer_par_pid() {
  local pid="$1" attendu="$2"
  [ -n "$pid" ] || { log "tuer_par_pid : pid vide"; return 1; }
  if tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -q "$attendu"; then
    kill -9 "$pid" 2>/dev/null; sleep 0.3
    kill -0 "$pid" 2>/dev/null && { ko "$attendu encore vivant"; return 1; }
    return 0
  fi
  log "tuer_par_pid : pid $pid != $attendu — NON TUÉ"; return 1
}

JOURNAL_PONT="$RUN/pont-essai.jsonl"
JOURNAL_TEMOIN="$RUN/temoin-essai.jsonl"

{
  echo "== empreintes $(date -u +%Y-%m-%dT%H:%M:%SZ) mode=$MODE_ESSAI =="
  sha256sum "$HARN_SIM" "$HARN_REEL" "$MV/config-reel-essai-oa81.json" \
            "$CONFIG_PONT" "$MV/livrable-defaut.txt" "$0"
} > "$RUN/empreintes.txt"

# serveurs loopback (PID relevés AVANT toute action)
COORD_BRIDGE_JETON="$JETON" COORD_BRIDGE_JOURNAL="$RUN/pont-factice.jsonl" \
  "$PY" "$MV/llmbridge/bridge.py" --factice --port "$PONT_FACTICE" \
  > "$RUN/pontf.out" 2>&1 &
PID_PF=$!
# lancement direct (PAS via eval/env) : $! doit être le PID du processus python
if [ "$MODE_ESSAI" = "reel" ]; then
  COORD_BRIDGE_JETON="$JETON" COORD_BRIDGE_JOURNAL="$JOURNAL_PONT" \
    "$PY" "$MV/llmbridge/bridge.py" --config "$CONFIG_PONT" --port "$PONT" \
    > "$RUN/pont.out" 2>&1 &
else
  COORD_BRIDGE_JETON="$JETON" COORD_BRIDGE_JOURNAL="$JOURNAL_PONT" \
    COORD_CLE_TEMOIN=factice-cle-temoin-oa81 \
    "$PY" "$MV/llmbridge/bridge.py" --config "$CONFIG_PONT" --port "$PONT" \
    > "$RUN/pont.out" 2>&1 &
fi
PID_PONT=$!
if [ "$MODE_ESSAI" = "rep" ]; then
  TEMOIN_OA81_JOURNAL="$JOURNAL_TEMOIN" "$PY" "$MV/temoin-oa81.py" "$TEMOIN_PORT" \
    > "$RUN/temoin.out" 2>&1 &
  PID_TEMOIN=$!
fi
sleep 1
kill -0 "$PID_PF" 2>/dev/null && ok "pont factice 8793 démarré" || ko "pont factice NON démarré"
kill -0 "$PID_PONT" 2>/dev/null && ok "pont essai 8790 démarré" || ko "pont essai NON démarré"

# ---------------------------------------------------------------------------
log "== pré-contrôle 0 EUR =="
if [ "$MODE_ESSAI" = "reel" ]; then
  code_models=$(curl -s -o "$RUN/models.json" -w '%{http_code}' --max-time 20 \
    -H "Authorization: Bearer $OPENAI_API_KEY" https://api.openai.com/v1/models)
  if [ "$code_models" = "200" ]; then
    ok "pré-contrôle GET /v1/models 200 (0 EUR)"
    grep -q "gpt-4o-mini-2024-07-18" "$RUN/models.json" \
      && ok "modèle gpt-4o-mini-2024-07-18 présent au catalogue" \
      || ko "modèle gpt-4o-mini-2024-07-18 ABSENT du catalogue — SUSPENDRE"
  else
    ko "pré-contrôle /v1/models code $code_models — SUSPENDRE (aucun appel)"
  fi
  # AUCUNE trace de la clé dans les sorties
  grep -rqs "$OPENAI_API_KEY" "$RUN" && ko "pré-contrôle : clé présente dans les traces" \
    || ok "pré-contrôle : clé absente des traces"
else
  code_ping=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $JETON" "http://127.0.0.1:$PONT/ping")
  [ "$code_ping" = "200" ] && ok "répétition : pont 8790 /ping 200 (témoin factice)" || ko "répétition : /ping $code_ping"
fi

# ---------------------------------------------------------------------------
log "== mission : creer (REEL, plafond 5 c, 0 appel) =="
COORD_MODE_REEL=1 COORD_BRIDGE_JETON="$JETON" COORD_CLE_TEMOIN=factice-cle-temoin-oa81 \
  "$HARN_REEL" -data "$STATE" -issue "$ISSUE" -mode creer \
  -config-reel "$MV/config-reel-essai-oa81.json" -plafond-centimes 5 \
  > "$RUN/creer.out" 2>&1; verif $? "creer (mission $ISSUE)"

# ---------------------------------------------------------------------------
log "== livrable synthétique avec défauts connus (produit en simulation) =="
cp "$MV/livrable-defaut.txt" "$RUN/livrable-v1.txt"
sha256sum "$RUN/livrable-v1.txt" >> "$RUN/empreintes.txt"
ok "livrable v1 posé (défauts connus : 200 centimes =/= 20 euros ; 0,006 centime)"

# ---------------------------------------------------------------------------
log "== executer (SIMULATION, pont factice 8793, 0 EUR ; PLANIFIEE -> EXECUTION) =="
cat > "$RUN/mandat-essai.txt" <<'EOF'
Mandat d'exécution (essai oa81) : produire le livrable de test.
Exécution en SIMULATION via le pont factice : aucun appel réel, 0 EUR.
Le livrable réellement soumis est le fichier synthétique à défauts connus.
EOF
COORD_BRIDGE_JETON="$JETON" "$HARN_SIM" -data "$STATE" -issue "$ISSUE" -mode executer \
  -mandat "$RUN/mandat-essai.txt" \
  -pont "http://127.0.0.1:$PONT_FACTICE/v1/chat/completions" \
  -modele-exec "factice/exec/oa81-essai" \
  > "$RUN/executer.out" 2>&1; verif $? "executer (PLANIFIEE -> EXECUTION, 0 EUR)"

# ---------------------------------------------------------------------------
log "== soumettre (SIMULATION, pont factice 8793, 0 EUR) =="
COORD_BRIDGE_JETON="$JETON" "$HARN_SIM" -data "$STATE" -issue "$ISSUE" -mode soumettre \
  -pont "http://127.0.0.1:$PONT_FACTICE/v1/chat/completions" \
  -modele-exec "factice/exec/oa81-essai" -livrable "$RUN/livrable-v1.txt" \
  > "$RUN/soumettre.out" 2>&1; verif $? "soumettre (révision en attente de réception)"

# ---------------------------------------------------------------------------
log "== recevoir : L'APPEL REEL UNIQUE via coord.llm.v1 =="
COORD_MODE_REEL=1 COORD_BRIDGE_JETON="$JETON" \
  "$HARN_REEL" -data "$STATE" -issue "$ISSUE" -mode recevoir \
  -config-reel "$MV/config-reel-essai-oa81.json" -livrable "$RUN/livrable-v1.txt" \
  > "$RUN/recevoir.out" 2>&1
RC_REC=$?
if [ "$RC_REC" = "0" ]; then
  ok "réception validée par le moteur (rc=0)"
elif [ "$RC_REC" = "2" ]; then
  ok "réception validée par le moteur (rc=2 : verdict A_CORRIGER attendu sur défauts connus)"
else
  ko "réception rc=$RC_REC — SUSPENDRE, aucun renvoi automatique"
fi
cat "$RUN/recevoir.out" | tee -a "$RUN/essai.log"

# ---------------------------------------------------------------------------
log "== contrôles : un seul appel, bonne révision, budget, secrets =="
if [ "$MODE_ESSAI" = "reel" ]; then
  n_appels_pont=$(nl_journal "$JOURNAL_PONT")
  [ "$n_appels_pont" = "1" ] && ok "exactement 1 inférence au journal du pont" \
    || ko "$n_appels_pont inférences au journal du pont (attendu 1)"
else
  n_appels_pont=$(nl_journal "$JOURNAL_PONT")
  [ "$n_appels_pont" = "1" ] && ok "répétition : exactement 1 inférence au journal du pont" \
    || ko "répétition : $n_appels_pont inférences (attendu 1)"
  grep -q '"model": *"gpt-4o-mini-2024-07-18"' "$JOURNAL_TEMOIN" 2>/dev/null \
    && ok "répétition : identité exacte gpt-4o-mini-2024-07-18 au témoin" \
    || ko "répétition : identité divergente au témoin"
fi
grep -rqE '"type": *"APPEL_RESERVE"' "$STATE/$ISSUE/" \
  && ok "APPEL_RESERVE journalisé avant l'appel" || ko "APPEL_RESERVE absent"
grep -rqE '"centimes_reserves": *3' "$STATE/$ISSUE/" \
  && ok "réservation 3 centimes (enveloppe ceil(bornes)+requête)" \
  || ko "réservation inattendue"
grep -rqE '"type": *"RECEPTION"' "$STATE/$ISSUE/" \
  && ok "RECEPTION journalisée (verdict appliqué à la soumission en attente)" \
  || ko "RECEPTION absente du journal"
# verdict appliqué à la bonne révision : décision ancrée à l'empreinte du livrable v1
EMP=$(sha256sum "$RUN/livrable-v1.txt" | cut -d' ' -f1)
grep -rq "$EMP" "$STATE/$ISSUE/" && ok "décision ancrée à l'empreinte du livrable v1 ($EMP)" \
  || ko "empreinte du livrable absente de l'état"
if [ "$MODE_ESSAI" = "reel" ] && [ -n "${OPENAI_API_KEY:-}" ]; then
  grep -rqs "$OPENAI_API_KEY" "$RUN" "$STATE" "$MV" 2>/dev/null \
    && ko "SECRET TROUVÉ dans run/state/workspace" || ok "aucun secret dans run/state/workspace"
else
  ok "aucun secret à chercher (mode répétition)"
fi
grep -rn "sk-" "$RUN" "$STATE/$ISSUE" 2>/dev/null | grep -q . \
  && ko "trace de clé sk- détectée" || ok "aucune trace de clé sk-"

# ---------------------------------------------------------------------------
log "== arrêts (PID vérifiés) =="
tuer_par_pid "$PID_PONT" "bridge.py" && ok "pont essai arrêté"
tuer_par_pid "$PID_PF" "bridge.py" && ok "pont factice arrêté"
[ -n "${PID_TEMOIN:-}" ] && { tuer_par_pid "$PID_TEMOIN" "temoin-oa81.py" && ok "témoin arrêté"; }
sleep 0.5
ss -ltn 2>/dev/null | grep -qE ":879[013]" && ko "écouteur résiduel" || ok "aucun écouteur 879x résiduel"
systemctl is-active nginx >/dev/null 2>&1 && ok "nginx actif" || ko "nginx NON actif"

{
  echo "PASS=$PASS FAIL=$FAIL mode=$MODE_ESSAI"
  printf '%s\n' "${RESUME[@]}"
} | tee "$RUN/bilan-essai.txt"

[ "$FAIL" = "0" ]
