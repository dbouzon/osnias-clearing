module coordengine

go 1.27

require (
	github.com/dbouzon/osnias-clearing/coord/a2a v0.0.0
	github.com/modelcontextprotocol/go-sdk v1.8.0
	golang.org/x/sys v0.48.0
)

// Le rail A2A vit dans le MODULE AUTONOME coord/a2a (M-COORD-RAIL-01) :
// la dépendance a2a-go v2 n'apparaît plus dans le go.mod du moteur (elle
// n'y figure que comme dépendance indirecte du module rail via le replace
// local ci-dessous, l'isolation est de périmètre source/CI, pas du binaire).
replace github.com/dbouzon/osnias-clearing/coord/a2a => ../a2a

require (
	github.com/a2aproject/a2a-go/v2 v2.9.0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)
