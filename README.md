# Osnias Clearing

**Multi-Node Clearing Network for recurring economic obligations**

Osnias Clearing is an open technical project for organizing multilateral clearing between economic actors through a network of local nodes.

The protocol is designed to reduce gross settlement flows by calculating net positions over synchronized economic cycles, while keeping the clearing layer strictly separated from custody and settlement infrastructure.

Official website: https://www.osnias-clearing.com/

English website: https://www.osnias-clearing.com/en/

---

## Overview

Osnias Clearing is structured around a common clearing layer and multiple local nodes.

Each node serves participants within its own legal and operational environment. Reciprocal obligations are first netted locally. Only residual positions are then exposed at inter-node level.

The architecture is designed around several core principles:

- multilateral netting of recurring obligations;
- local node responsibility under its applicable jurisdiction;
- strict separation between clearing and settlement;
- non-custodial protocol design;
- no DEX, AMM or CEX dependency;
- no protocol-level lending or leverage;
- dedicated clearing cycles;
- public technical documentation and testnet references.

The current design targets economic actors in the **middle market and beyond**, especially where recurring bilateral obligations create meaningful opportunities for organized netting.

---

## Functional Architecture

### Clearing layer

The canonical clearing register is designed to operate on **Sei**.

It records accepted obligations, participant positions, clearing-cycle states and the data required to calculate net positions.

### Settlement layer

Assets used for settlement remain segregated from the clearing register.

Settlement and escrow functions are operated at node level on compatible EVM infrastructure, including Ethereum-based components.

Osnias Clearing does not custody participant funds and does not move settlement assets itself.

### Node model

Each node is intended to remain:

- operationally local;
- subject to its own legal and regulatory framework;
- connected to the common clearing protocol;
- responsible for its own settlement infrastructure.

---

## Clearing Cycles

Osnias Clearing uses synchronized economic cycles intended to reflect real commercial horizons:

- **1 week**
- **4 weeks**
- **13 weeks**

During a cycle, reciprocal obligations accumulate before net positions are calculated and finalized.

The objective is to reduce both the number and the volume of gross settlements by resolving reciprocal obligations before final settlement.

---

## Osnias-ID

**Osnias-ID** is the internal clearing-account identifier used within the Osnias Clearing network.

The current format uses a 19-digit identifier designed to associate a clearing account with:

- a registry;
- a network;
- a currency;
- a server domain;
- a clearing-cycle duration;
- a checksum.

Osnias-ID is an internal technical identifier. It is not a legal identity and does not constitute a regulatory authorization.

Documentation:

- https://www.osnias-clearing.com/osniasid/
- https://www.osnias-clearing.com/en/osniasid/

Permanent archive:

- DOI: https://doi.org/10.5281/zenodo.22978105

---

## Public Documentation

### Book 1 — Osnias-ID

Internal clearing-account identifier, registry structure and interoperability.

- FR: https://www.osnias-clearing.com/osniasid/
- EN: https://www.osnias-clearing.com/en/osniasid/
- DOI: https://doi.org/10.5281/zenodo.22978105

### Book 2 — Architecture

Functional architecture, clearing register, time coordination and separation between clearing and settlement.

- FR: https://www.osnias-clearing.com/architecture/
- EN: https://www.osnias-clearing.com/en/architecture/
- Zenodo: https://zenodo.org/records/23012840
- HAL: https://hal.science/hal-05752756

### Book 3 — Netting Rules

Multilateral netting rules, position calculation, aggregation, residual balances, controls, disputes, finalisation and interaction with node settlement.

- FR: https://www.osnias-clearing.com/rules/
- EN: https://www.osnias-clearing.com/en/rules/
- DOI: https://doi.org/10.5281/zenodo.22922199
- HAL: https://hal.science/hal-05760527

### Security

Security principles, invariants and control architecture.

- https://www.osnias-clearing.com/security/

### Onboarding

Technical onboarding and settlement-side interaction model.

- https://www.osnias-clearing.com/onboarding/

### POP / Infrastructure

Point-of-Presence and dedicated infrastructure architecture.

- https://www.osnias-clearing.com/pop/

### Regulation

Public regulatory memoranda and qualification work.

- https://www.osnias-clearing.com/regulation/

---

## Public Testnet References

Osnias Clearing publishes technical testnet references for review, audit and reproducibility.

Current public material includes:

- Sei Testnet contracts and registries;
- Ethereum Sepolia escrow and controller components;
- public Solidity source files and ABIs;
- deployment references;
- security and architecture documentation.

Official deployment page:

https://www.osnias-clearing.com/deployment/

---

## Design Invariants

The project is built around a set of non-negotiable architectural invariants.

Among them:

- clearing is not custody;
- clearing is not settlement;
- the clearing protocol does not hold participant funds;
- no hidden mint or burn authority;
- no DEX / AMM dependency;
- no protocol-level leverage;
- no protocol-level lending;
- separation of governance roles from clearing-token control;
- node-level responsibility for settlement infrastructure;
- public and auditable technical references.

---

## Status

Osnias Clearing is currently under technical development, regulatory review and security qualification.

Public documentation and testnet components are available for research, review, audit and proof-of-concept work.

No production contract should be considered active unless it is explicitly listed in the official deployment registry.

---

## Repository Scope

This repository contains public technical material related to the Osnias Clearing project, including source files, ABI files, deployment references, website pages and supporting documentation.

The repository is intended for:

- technical review;
- audit preparation;
- regulatory review;
- proof-of-concept work;
- reproducibility;
- public documentation.

---

## Author

**Denis Bouzon**

ORCID: https://orcid.org/0009-0007-8894-8902

Website: https://www.osnias-clearing.com/

Contact: contact@osnias-clearing.com

---

## Disclaimer

Osnias Clearing provides public technical information for research, development, review and audit purposes.

Nothing in this repository constitutes:

- an offer of securities;
- investment advice;
- a prospectus;
- a solicitation;
- legal advice;
- regulatory advice;
- accounting advice;
- audit advice.

Regulatory classification remains subject to the applicable jurisdiction, implementation and operating model.
