module github.com/dbouzon/osnias-clearing/coord/a2a-agent

go 1.27

require (
	coordengine v0.0.0
	github.com/a2aproject/a2a-go/v2 v2.9.0
	github.com/dbouzon/osnias-clearing/coord/a2a v0.0.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
)

replace github.com/dbouzon/osnias-clearing/coord/a2a => ../a2a

replace coordengine => ../go
