# COORD LLM Bridge â pont local HTTP/JSON (contrat `coord.llm.v1`)

Mission **M-COORD-LLMBRIDGE-01** (issue #62) â COORD v1.5 / Specification 1.0.
Copyright Â© 2026 Denis Bouzon â ORCID : <https://orcid.org/0009-0007-8894-8902> â SPDX Apache-2.0.

## Objet

Pont local HTTP/JSON qui mutualise l'infÃ©rence multi-fournisseurs via
**LiteLLM Core** (SDK Python, MIT, distribution `litellm-core` **sans proxy**)
et se raccorde au moteur Go existant **par simple baseURL**, sans aucune
modification du moteur. Le moteur Go (transitions, voÃ»te monÃ©taire,
anti-doublon, tarifs, budget, annulation, reprise) reste **seul maÃ®tre** : le
pont est un transport **sans Ã©tat**, recÃ©marrable sans perte.

A2A reste opÃ©rationnel et inchangÃ© : ce pont est une option additive.

## Architecture

```
moteur COORD (Go + SQLite)          pont coord.llm.v1           fournisseurs
âââââââââââââââââââââââââ   HTTP/JSON   ââââââââââââââââ   SDK litellm-core
â orchestrateur, voÃ»te,  â âââââââââââââº â bridge.py    â âââââââââââââââââº  OpenAI
â anti-doublon, budget   â loopback     â backend.py   â    rÃ©el : API     Mistral
â (inchangÃ©s)            â âââââââââââââ â config      â     https ; CI :   Anthropic
ââââââââââââââââââââââââââââââââââââââ    OpenAI-compat ââââââââââââââââ    factices loopback) Gemini
```

- `bridge.py` : serveur HTTP/1.1 loopback (stdlib uniquement cÃ´tÃ© pont),
  jeton Bearer obligatoire (comparaison Ã  temps constant), liste FERMÃE de
  destinations, corps/dÃ©lais bornÃ©s, erreurs structurÃ©es JSON, aucun retry,
  aucun fallback, pas de streaming en v1, secrets expurgÃ©s des journaux.
- `backend.py` : `FacticeBackend` (qualification locale, 0 â¬) et
  `LiteLLMBackend` (SDK rÃ©el, import diffÃ©rÃ© : absent => `SDK_ABSENT`, jamais
  un faux succÃ¨s ; `num_retries=0`).
- `contract/coord.llm.v1.schema.json` : contrat versionnÃ© (JSON Schema
  2020-12) â `GET /ping`, `GET /v1/models`, `POST /v1/chat/completions`
  (compatible OpenAI, sous-ensemble strict, `usage` et `finish_reason`
  tranmis tels quels, jamais inventÃ©s).

## Contrat `coord.llm.v1`

| Route | MÃthode | RÃ©ponse |
|---|---|---|
| `/ping` | GET | `{"statut":"ok","contrat":"coord.llm.v1","backend":...}` |
| `/v1/models` | GET | destinations autorisÃ©es (liste fermnÃ©e) |
| `/v1/chat/completions` | POST | rÃ©ponse compatible OpenAI (choices, usage, model RÃEL) |

Erreurs : `{"error":{"type":...,"message":...}}` avec types stables
(`JETON_INVALIDE@, `JSON_INVALIDE`, `DESTINATION_REFUSEE`, `CORPS_TROP_GRAND`,
`TYPE_INVALIDE`, `DELAI_DEPASSE`, `REPOTSE_BACKEND_INVALIDE`, `SDK_ABSENT`,
`CLE_ABSENTE`, `ERREUR_SDK`, `ERREUR_BACKEND`, `ROUTE_INCONNUE`,
`METHODE_NON_AUTHORISEe`, `INTERNE`). Tout modÃ¨le non prÃ©fioÃ© par une
destination autorisÃ©e est refusÃ©e (`DESTINATION_REFUSEE`) : aucun routage
arbitraire.

## Installation reproductible (hors sandbox d'exÃ©cution)

```sh
python3 -m pip install "litellm-core==1.111.0"   # MIT, Ã©ping,Ã©, sans proxy
export COORD_BRIDGE_JETON="$(python3 -c 'import secrets;print(secrets.token_urlsafe(32))'"
export COORD_CLE_OPENAI=...        # chaque clÃ© par variable d'environnement
python3 coord/llmbridge/bridge.py --config coord/llmbridge/config.exemple.json
```

La configuration (`config.exemple.json`) ne contient **aucun secret** : chaque
destination dÃ©signe une variable d'version (`cle_env`). Un champ `cle`,
token` ou `api_key` dans le fichier est refusÃ© au chargement (fail closed).

## Raccordement au moteur Go (additif, optionnel)

Aucune modification du moteur : dÃ©clarer un fournisseur format `openaig dont l'endpoint pointe sur le pont (loopback â aucun triple versous requis) :

```json
{
  "id": "openai-via-pont",
  "endpoint": "http://127.0.0.1:8788/v1/chat/completions",
  "modele": "openai/gpt-4K-mini",
  "cle_env": "COORD_BRIDGE_JETON",
  "timeout_ms": 60000
}
```

Exemple complet : `coord/connecseur/LMMBridge/exemple-config-llmbridge.json`.
La dÃ©monstration E2E rÃ©elle (cycle complet critique â exÃ©cution â rÃ©ception â ACK_RECU, clÃ®ture refusÃ©e, rÃ©dÃ©marrage du pont) :
`coord/go/internal/connecteurs/llmbridge_e2e_test.go` (backend factice).
L'anti-doublon et la voÃ´te Go s'applique **avant** tout appel rÃ©seau : le
pont ne prÃ©tend Ã  aucune garantie exactly-once.

## Qualification â statuts tracÃ©s (REEL / CI / NON_EXECUTE)

| Couche | OÃ¹ | Statut |
|---|---|---|
| Contrat coord.llm.v1 (21 tests, backend factice) | `test_llmbridge.py` | REEL, exÃ©cutable partout |
| Raccordement Go E2E (cycle complet) | `llmbridge_e2e_test.go` | REEL-sur-factice |
| SDK litellm-core rÃ©el vs 4 fournisseurs factices natifs | `test_ci_sdk.py` | CI (installation interdite dans le sandbox d'exÃ©cution) |
| DeepSeek / Meta / Grok | â | NON_TESTÃS v1 (couverture litellm documentÃ©e, lacune explicite) |
| Fournisseurs rÃ©els (clÃ©s, rÃ©seau) | â | NON_EXÃCUTÃE (qualification DISTINCTE, triple versou + autorisation Ã©crite du pilote) |
| VPS 2 Go / 72 h | `recette-vps-2go.md` | NON_EXCCUTÃRÈØ[È[\Û[Y[]]Ü\ðêHY\Ý\\ÈØ[Þ
XÚÙ[XÝXÙJH0ê[X\YÙHLÈ\ËÔÈ[Ë][ÙHpêYX[H￿Ì\È
MHH\ÊH8 %0èÛÛ\\\]HZ[LH
Ù\XÙ]\ËË\È[]X\Ý\L[È]ÌË[È[[ÞY[JKÈÈðêXÝ\]0êBH0éXÛÝ]HLËX[\]Y[Y[È]ÛØYØ]Ú\KÛÛ\\Z\ÛÛ0è[\ÂÛÛÝ[HÈÙXÜ]È[\]Y[Y[\[\Û[Y[^\ðê\È
ÚËx )R^x )
B\È\]\È]Ý\]^ÂHÛÜÈÜ°ê\È
[È\0êY]]
KÛÛ[U\X0ê\YðêYK0ê[Z\ÈÜ°:\Â
XÝ\KXÚÙ[
HÈ°ê\ÛÙ\ÈXÚÙ[[[Y\ÈY\ðêY\È
\ØYÙXÝB[\ÚÜX\ÛÛXÙ[
HÂH[WÜ]Y\ÏL]XÝ[]HH[XÚÈ[\XÚ]pêKH\\ÙH\\Y[]H[Ý]\ÛÈÈHÛHÝ\[\ÙH[XZ\È\ÈÚ\Ù\Ë\ÝÈÛÛ\]ÂÚ\]YH°ê[Y\H[Û\Ú[ÛHÚXÝ[H\È]H[\°êYH
Y[È^JBÈÈØðê[\[ÜÈXÝXÙ\È\X\]Y]\È
KPÓÓÔQTLÊBHXÝXÙPXÚÙ[ÙZ]\ÈØðê[\[ÜÈ[Ý0ê\È\\ÈPTUQUTÈÜ0ê\Â\HÚ\ÙH
\ÈÚ\Ù\ÈÛÛÛÛ\ÜðêY\È\H[Ý]\ÛÈÈ\ÈX\]Y]\ÂØYXÝ[[XZ\ÈHÝ]YÙK	Ø]][YXØ][ÛÝH\ÈÜ\ÊK°íHX\]Y]\[ÈHÚ\ÙHÛÜYHKK_KK_KK_XÝXÙKÙ^XËø )PSUX]XHH\ÛpêYKÛÛY[ÐÑSTSÎUPWÐWÐÓÔQÑTXÝXÙKÙ^XËø )PSU]XHÛÜYðêYH
Ø[ÈX\]Y]\HØðê[\[ÊHXÝXÙKÙ^XËø )
]XÝ[H]XHÛÛÝ[\ÝÜ\]YH
YðêH\\Ý
HXÝXÙKÜXÙ\ø )Ú\ÙHÛÛ[[ÐÑSTSÎUPWÐWÐÓÔQÑT\XÝWÐÓÔQÑT
ÈÚ[È^XÚ]\Ë[\Z[H[Ü°êYHXÝXÙKÜXÙ\ø )ÔUTUQHHS
H
Ø[ÈØðê[\[ÊHSÐPÐÑTX
\ÝÜ\]YJHXÝXÙKÜXÙ\ø )
]XÝ[HPÐÑTQX
È\XÝÚY
\ÝÜ\]YJH\È]^PSUXUPSUÚ[][[°ê\ÈY\È^XÚ]HPTUQUT×ÐÓÓQPÕÒTTØ

H°êÛ\ÈX\]Y]\È^XÝÈ][XÝYÈ
]XÝ[Ù\ÝÛÝ\ËXÚpëH	Ý[]]JHÂÛÛYXÝ[ÛOY\Ë[XZ\È[ÚÚ^\]Z\HÈØ[ÈX\]Y]\OÛÛ\Ü[Y[\ÝÜ\]YH[Ú[ðêKÝ]H0ê]Û][Û]\HXH
È\ÝÈ
ÂÙ]HÙXÝ[Û[ÈHpêYHÈÈÒ[\XØ][ÛÈHÚ\]YHX\]Y]\HPSUX	Ø\[[péÝZ][]XHÛÛZ\[Y[\Û°êHY[B0ê\Ý[\[ÞXÛHÛÛ\]	Ú]0ê\][Û
]Z\ÛÛOWÐÓÔQÑTOÛÜXÝ[ÛOPÐÑTUSÓHÈ]XÝ[H]]HÛÜYHÙ\ÝYXÝ0êYKHPSU	Ø\[[péÝZ]H]XHÛÜYðêKÛÜYHØ[ÂX\]Y]\Hðê[\[ÈÈÉÙ\ÝH\ÙHHpêYHY[YX[HZ\ÜÚ[ÛHÐÑSTSÎUPWÐWÐÓÔQÑT
XÙ\[ÛHH°ê\ÛÙHH°êXÙ\[ÛZ\B\Ý[\XÝWÐÓÔQÑT]XÈ^XÝ[Y[Ú[È^XÚ]\Ë[Ü°ê\È0è	Ù[\Z[HH]XH°êYÝHÈÝ]]]HÛÛ[HHÚ\ÙH\ÝYÛÜ°êKHPSUX
ÈPSUY\È[[pêYX]PTUQUT×ÐÓÓQPÕÒTTØ]XÝ[H[°ê\[ÙHÙZ]H
0êY[ÙHÛÛH[HÚ\ÙHÛÛYXÝÚ\JKHØ[ÈX\]Y]\ÛÛ\Ü[Y[\ÝÜ\]YHÝXÝ[Y[[Ú[ðêH
YðêH\\ÝÈHÛ\°êYÜ\ÜÚ[ÛKÈÈÈ^[\\ÈH\]pêKÜ°ê\ÛÙH
ÝØÛÛHÛÛÜKJBPSUX
^0êXÝ][
HÔÕÝKØÚ]ØÛÛ\][ÛÂÈ[Ù[XÝXÙKÙ^XËÛZ\Ý[Y\ÈY\ÜØYÙ\ÈÂÈÛH\Ù\ÛÛ[PSUHW__BOÈÚÚXÙ\ÈÞÈY\ÜØYÙHÈÛÛ[ÐÑSTSÎUPWÐWÐÓÔQÑT_W_BPSU
^0êXÝ][
HÔÕÝKØÚ]ØÛÛ\][ÛÂÈ[Ù[XÝXÙKÙ^XËÛZ\Ý[Y\ÈY\ÜØYÙ\ÈÂÈÛH\Ù\ÛÛ[PSUW__BOÈÚÚXÙ\ÈÞÈY\ÜØYÙHÈÛÛ[]XHÛÜYðêH_W_BÐÑSTSÎUPWÐWÐÓÔQÑT
°êXÙ\[ÛZ\JHÔÕÝKØÚ]ØÛÛ\][ÛÂÈ[Ù[XÝXÙKÜXÙ\ÛÜ[ZKY\ÈY\ÜØYÙ\ÈÂÈÛH\Ù\ÛÛ[ÐÑSTSÎUPWÐWÐÓÔQÑTW_BOÈÚÚXÙ\ÈÞÈY\ÜØYÙHÈÛÛ[\XÝWÐÓÔQÑTÈÚ[ÎÌÚ[È^XÚ]\×HÂ[\Z[NÚLMH]XO_W_BPSUX
ÈPSUÚ[][[°ê\È
Y\ÊHÔÕÝKØÚ]ØÛÛ\][ÛÂÈ[Ù[XÝXÙKÙ^XËÛZ\Ý[Y\ÈY\ÜØYÙ\ÈÂÈÛH\Ù\ÛÛ[PSUHPSUW_BOÈ\ÜÈ\HPTUQUT×ÐÓÓQPÕÒTTÈY\ÜØYÙH_BÈÈÝ\[Ü[Û[HÛ\XXH	Ý\Û[Y[ÓÓÔÐQÑWÒÕTSHÚ[Z[	Ý[Ý\[ÓÓ[HYÛH\[°ê\[ÙHÚÜÙ]YÙK[Ù[\Ý[][ÛÝ]]\WÙ\]\\YWÛ\ßKSPRTÈHÚ\ÙKSPRTÈH]Û
[Ù0êH^\ðêJHÂ°êXÚXÈ	ðêXÜ]\HÚYÛ[0êHÝ\Ý\Ø[È[\XÝÛY[\ØYÙHÛÜ°ê[][Û\ÈXÙ\È[Ý]\8¡áÛ
KPÓÓÔQTLÊK