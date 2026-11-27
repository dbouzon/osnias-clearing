# Rapport M-COORD-JCS-01 — alignement des empreintes JSON canoniques (RFC 8785) Python ⇄ Go

Issue #85 — ordre du 27/11/2026. Protocole COORD v1.5 / Specification 1.0.
Exécutant : Mistral. Critique/réceptionnaire : OpenAI. Pilote : Denis Bouzon.
Copyright © 2026 Denis Bouzon — ORCID : https://orcid.org/0009-0007-8894-8902 — SPDX Apache-2.0.

## Contexte et reproduction (T2, arbre PRISTINE au SHA de départ 3844d6a9)

La Specification 1.0 (§5.2) impose la forme canonique RFC 8785 : clés d'objet
triées par **unités de code UTF-16**, échappement minimal, nombres entiers
uniquement ; §5.3 : empreinte = SHA-256 sur la forme canonique, hex minuscule.
Constat : le moteur Go (`coord/go/internal/jcs`, `utf16Less`) est conforme ;
le chemin Python d'empreintes v1.5 (`coord06_v15.py` `_empreinte_contenu`,
`champs_divergents`) utilisait `coord06.canonical_json` = `json.dumps(sort_keys=True)`
= tri par **points de code**, et sérialisait les non-entiers en repr non normatif.

Reproduction au SHA `3844d6a9` (octets et sha256 croisés, avant toute correction) :

| Cas | Python (avant) | Go (attendu RFC) |
| --- | --- | --- |
| clés U+10000/U+E000 | `{"\ue000":2,"\U00010000":1}` sha256 `daec79d8…` | `{"\U00010000":1,"\ue000":2}` sha256 `4045c21a23c8ae8f8d9add81f54bd506bee65885099876fb4afb378b1f2c3516` |
| objet imbriqué dans tableau | clés imbriquées triées par points de code, sha256 `bed65c85…` | clés imbriquées triées UTF-16, sha256 `5b77a91a0ccdfcf6444e2d76237b1749f165f943240e464a139335e5b42c96a1` ; ordre des éléments du tableau préservé par les deux |
| non-entier 1.5 | `{"valeur":1.5}` (repr non normatif) | refus fail-closed « nombre non canonique (entier attendu) » (§5.3) |

Trace ancrée : `knowledge/coord-projet/m-coord-jcs-01/reproduction-sha-3844d6a9.txt`
(sha256 `41b39b4d…`). En pratique les clés de production sont ASCII : aucun
artefact historique connu avec clés astrales. `coord06.py` (v1.0..v1.4) est
figé par le manifeste M1 ; `coord06_v15.py` et `tests_coord06_v15.py` ne le
sont pas (ligne de développement active). Plan v1 soumis au pont (session
`m-coord-jcs-01`) : **PLAN_ACCEPTE** tour 1, trois décisions de compatibilité
arbitrées (refus des non-entiers côté Python aligné Go/spec ; vecteur public
plutôt que fixture privée ; spec inchangée).

## Correction (minimal, Python uniquement ; Go INTACT)

- `coord06_v15.py` : nouvelle fonction `_canonique_v15(obj)` — sérialisation
  canonique récursive parité exacte avec `go/internal/jcs` : clés triées par
  unités UTF-16 (clé de tri = `s.encode("utf-16-be")`, équivalent exact de la
  comparaison par unités de code), séparateurs compacts, `ensure_ascii=False`
  (échappement minimal identique), tableaux à ordre préservé, booléens/null
  traités avant entiers, nombres non entiers REFUSÉS (`ValueError`, motif
  « nombre non canonique (entier attendu) », jamais approximés ; borne 2^53
  = parité Go). `_empreinte_contenu` et `champs_divergents` l'utilisent.
  Dans `champs_divergents`, une valeur non canonique est marquée
  `<non canonique>` sans faire échouer l'arbitrage : parité exacte avec
  `resume.ChampsDivergents` Go.
- `coord06.py` et tout fichier figé au manifeste : INTACTS. Le chemin de
  STOCKAGE (`ecriture_atomique`, journaux JSONL) continue d'utiliser
  `coord06.canonical_json` (figé) : octets de stockage inchangés ; pour les
  clés ASCII les deux formes sont identiques octet par octet (vérifié).
- Aucune modification du protocole, du format de stockage, des preuves ou des
  manifestes historiques. `docs/spec/coord-spec-1.0.md` INTACT (l.15 : constat
  historique « 22/22 conformance vectors » de la qualification d'origine).

## Vecteur partagé public (consommé par les DEUX moteurs)

- `coord/docs/conformite-v15/ordre_cles_utf16.json`, format
  **`coord-conformite-jcs-v1`**, famille `serialisation_canonique`, 3 vecteurs :
  JCS-1 ordre des clés UTF-16 (octets + sha256 attendus fixés indépendamment
  par le moteur Go conforme avant la correction Python), JCS-2 tri récursif
  des clés imbriquées avec ordre des tableaux préservé, JCS-3 non-entier →
  refus fail-closed avec motif.
- Les DEUX runners rejouent la famille : branche additive `empreinte` dans
  `runner_conformite.py` et dans `go/cmd/vector-runner` (operation
  `{"operation":"empreinte","objet":…}`, attendu `{octets, sha256}` ou
  `{erreur: true, contient}`).
- `go/internal/jcs/jcs_test.go` : `TestVecteursPartagesJCS` consomme LE MÊME
  fichier (preuve de parité au même artefact).
- `schemas/vector-jcs.schema.json` (JSON Schema 2020-12, additif) + branche
  additive dans `.github/scripts/coord-validate-schemas.py` : la nouvelle
  famille est validée par schéma comme la famille v1 (22 vecteurs/4 fichiers
  v1 INTACTS, + 3 vecteurs/1 fichier jcs).
- Compteur public : **22 → 25 vecteurs** (5 fichiers). Note de fidélité au
  plan : le plan v1 mentionnait « 22 → 23 » ; 3 nouveaux cas étant ajoutés,
  le total arithmétique est 25. Delta doc-only, signalé au réceptionnaire.

## Tests et non-régression (REEL, rejoués après correction)

- `tests_coord06_v15.py` : 6 tests nouveaux (JCSa..JCSf : ordre UTF-16,
  tri récursif + ordre des tableaux, refus des non-entiers, types de base et
  échappement minimal, empreintes H5 par la forme normative + parité
  `<non canonique>`, compatibilité octet par octet avec
  `coord06.canonical_json` sur clés ASCII). Batterie v1.5 : **53/53 PASS**
  (47 + 6), M1 PASS (fichiers historiques inchangés contre main `323d8b9`).
- Historiques : 24+13+11+13+10+17 = **88/88 PASS** ; robustesse incluse.
- Vecteurs : **25/25** sur le module Python ET sur le moteur Go ;
  cross-check Python ⇄ Go : les 25 verdicts [OK]/[KO] sont IDENTIQUES ligne à
  ligne (sorties des deux runners comparées octet par octet).
- Go : `go vet ./...`, `go build ./...`, `go test ./...` verts (jcs inclus).
- Schémas : le validateur CI (section 6 étendue) rejoué avec jsonschema
  2020-12 sur un harnais équivalent (le sandbox de l'exécutant interdit
  l'installation de paquets) : 22/4 famille v1 + 3/1 famille jcs, schéma
  `vector-jcs` valide.

## Compatibilité en lecture seule

Recalcul ancien vs nouveau code Python des empreintes sur : tous les
enregistrements `arbitrer` des vecteurs existants, échantillon d'enregistrements
v1.5 réels (reprise, journal, habilitations, curseur grand entier) →
**empreintes IDENTIQUES** (clés ASCII). Octets de stockage
(`coord06.canonical_json`, figé) inchangés. Les preuves et archives
historiques conservent leurs octets et empreintes ; elles se relisent
normalement. Cas non vérifiables : aucun identifié.

## Documentation (docs vivantes uniquement)

Compteurs mis à jour 47→53 tests et 22→25 vecteurs, 5→6 schémas :
`README.md`, `index.html`, `implementations/index.html`,
`CALL-FOR-IMPLEMENTATIONS.md`, `docs/coord-guides.md`,
`docs/proto-coord06-v15.md` (description de la famille jcs ajoutée),
`.github/workflows/coord-go-ci.yml` (noms d'étapes). Rapports et
qualifications historiques INTACTS.

## Livraison

Branche `mission/coord-jcs-01`, PR au SHA exact, CI (moteur Python modifié +
validateur schémas étendu), réception indépendante OpenAI, consignations
(issue #85, Drive, KNOWLEDGE.md). Fusion et clôture réservées au pilote.
0 €, aucune clé, aucun appel facturable, loopback uniquement.
