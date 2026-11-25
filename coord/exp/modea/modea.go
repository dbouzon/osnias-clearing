// Copyright 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package modea implémente le MODE A de M-COORD-EXP-01 : A2A DIRECT.
// Un serveur a2asrv (JSON-RPC) par agent en loopback HTTP (127.0.0.1),
// client a2aclient officiel, AgentCard servie sur le chemin standard
// A2A v1.0 (/.well-known/agent-card.json). Les mesures d'octets et
// d'échanges sont collectées par un http.RoundTripper compteur.
package modea

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/dbouzon/osnias-clearing/coord/exp/agents"
)

// Mesures collectées sur le transport HTTP loopback (corps des requêtes
// et réponses ; en-têtes identiques entre les deux modes par
// construction, pour une comparaison reproductible).
type Mesures struct {
	Octets     int64
	Echanges   int64
	DerniereMs int64
}

// compteur mesure octets et échanges de chaque aller-retour A2A.
type compteur struct {
	base http.RoundTripper
	mes  *Mesures
}

func (c *compteur) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	var octetsReq int64
	if req.Body != nil {
		br, _ := io.ReadAll(req.Body)
		octetsReq = int64(len(br))
		req.Body = io.NopCloser(bytes.NewReader(br))
		req.ContentLength = int64(len(br))
	}
	resp, err := c.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	octetsRep, _ := io.Copy(&buf, resp.Body)
	resp.Body.Close()
	resp.Body = io.NopCloser(&buf)
	resp.ContentLength = int64(buf.Len())
	c.mes.Echanges++
	atomic.AddInt64(&c.mes.Octets, octetsReq+octetsRep)
	atomic.StoreInt64(&c.mes.DerniereMs, time.Since(start).Milliseconds())
	return resp, nil
}

// Pile du mode A : un serveur JSON-RPC par agent + un client par agent.
type Pile struct {
	listenExec net.Listener
	listenCrit net.Listener
	urlExec    string
	urlCrit    string
	Mesures    *Mesures
	clientExec *a2aclient.Client
	clientCrit *a2aclient.Client
}

// NouvellePile démarre les deux serveurs A2A (loopback) et construit
// les clients a2aclient. Fermer() libère les écoutes.
func NouvellePile(ctx context.Context) (*Pile, error) {
	p := &Pile{Mesures: &Mesures{}}
	var err error

	demarrerAgent := func(nom string, exec a2asrv.AgentExecutor) (net.Listener, string, error) {
		handler := a2asrv.NewHandler(exec)
		mux := http.NewServeMux()
		mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(handler))
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, "", err
		}
		// L'AgentCard est servie sur le chemin standard A2A v1.0 ;
		// l'URL de l'interface n'est connue qu'après l'écoute.
		url := fmt.Sprintf("http://127.0.0.1:%d/invoke", ln.Addr().(*net.TCPAddr).Port)
		var carte *a2a.AgentCard
		if nom == "exec" {
			carte, _ = agents.Cartes(url, url)
		} else {
			_, carte = agents.Cartes(url, url)
		}
		mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(carte))
		go http.Serve(ln, mux)
		return ln, url, nil
	}

	if p.listenExec, p.urlExec, err = demarrerAgent("exec", agents.Exec{}); err != nil {
		return nil, err
	}
	if p.listenCrit, p.urlCrit, err = demarrerAgent("crit", agents.Crit{}); err != nil {
		return nil, err
	}

	nouveauClient := func(carte *a2a.AgentCard) (*a2aclient.Client, error) {
		httpClient := &http.Client{Transport: &compteur{base: http.DefaultTransport, mes: p.Mesures}}
		return a2aclient.NewFromCard(ctx, carte, a2aclient.WithJSONRPCTransport(httpClient))
	}
	var cexec, ccrit *a2a.AgentCard
	cexec, ccrit = p.Cartes()
	if p.clientExec, err = nouveauClient(cexec); err != nil {
		return nil, err
	}
	if p.clientCrit, err = nouveauClient(ccrit); err != nil {
		return nil, err
	}
	return p, nil
}

// Cartes retourne les AgentCard courantes des deux agents.
func (p *Pile) Cartes() (*a2a.AgentCard, *a2a.AgentCard) {
	return agents.Cartes(p.urlExec, p.urlCrit)
}

// ClientExec retourne le client A2A de l'agent exécutant.
func (p *Pile) ClientExec() *a2aclient.Client { return p.clientExec }

// ClientCrit retourne le client A2A de l'agent critique.
func (p *Pile) ClientCrit() *a2aclient.Client { return p.clientCrit }

// URLExec retourne l'adresse de l'interface de l'agent exécutant.
func (p *Pile) URLExec() string { return p.urlExec }

// URLCrit retourne l'adresse de l'interface de l'agent critique.
func (p *Pile) URLCrit() string { return p.urlCrit }

// BaseExec retourne l'URL de BASE (/.well-known) de l'agent exécutant.
func (p *Pile) BaseExec() string { return strings.TrimSuffix(p.urlExec, "/invoke") }

// Fermer arrête les serveurs loopback et détruit les clients.
func (p *Pile) Fermer() {
	if p.listenExec != nil {
		p.listenExec.Close()
	}
	if p.listenCrit != nil {
		p.listenCrit.Close()
	}
	if p.clientExec != nil {
		p.clientExec.Destroy()
	}
	if p.clientCrit != nil {
		p.clientCrit.Destroy()
	}
}

// Carte télécharge l'AgentCard via le résolveur standard (discovery S1).
// baseURL est l'URL de base (pas l'interface /invoke).
func Carte(ctx context.Context, baseURL string) (*a2a.AgentCard, error) {
	return agentcard.DefaultResolver.Resolve(ctx, baseURL)
}
