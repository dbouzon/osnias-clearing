// Copyright © 2026 Denis Bouzon
// ORCID: https://orcid.org/0009-0007-8894-8902
// SPDX-License-Identifier: Apache-2.0

// Package policy implements COORD Specification 1.0 sections 3 and 4:
// identities qualified by registered adapters, and fail-closed habilitations.
// The engine never authenticates; adaptation happens at the trust boundary.
package policy

import (
	"sort"
	"strings"

	"coordengine/internal/spec"
)

// Identity (spec 3): qualified at the trust boundary by an adapter.
type Identity struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	QualifiedBy string `json:"qualifiee_par"`
}

// ValidateConfig mirrors the specification N-01..N-05 validation. It returns
// the list of errors; a non-empty list means the configuration is invalid and
// every operation of the engine MUST be refused (fail closed).
func ValidateConfig(adapters []string, roles map[string][]string) []string {
	var errs []string
	if !allNonEmpty(adapters) {
		errs = append(errs, "adaptateurs : liste d'identifiants non vides attendue")
	} else if hasDuplicates(adapters) {
		errs = append(errs, "adaptateurs : doublon interdit")
	}
	for _, role := range sortedKeys(roles) {
		if !in(spec.Roles, role) {
			errs = append(errs, "role inconnu: "+role)
			continue
		}
		actions := roles[role]
		for _, a := range actions {
			if a == "*" {
				errs = append(errs, "joker interdit dans les actions du role: "+role)
			}
			if !in(spec.Actions, a) {
				errs = append(errs, "action inconnue: "+a)
			}
		}
		if hasDuplicates(actions) {
			errs = append(errs, "actions en doublon pour le role: "+role)
		}
	}
	return errs
}

// Qualified is the engine-level qualification of a section-3 identity: the
// adapter MUST be registered in the configuration (spec 3, N-01).
func Qualified(id Identity, adapters []string) bool {
	if strings.TrimSpace(id.ID) == "" || strings.TrimSpace(id.Role) == "" ||
		strings.TrimSpace(id.QualifiedBy) == "" {
		return false
	}
	if !in(spec.Roles, id.Role) {
		return false
	}
	return in(adapters, id.QualifiedBy)
}

// Authorized (spec 4): fail closed: invalid configuration, unqualified
// identity, or an action outside the role's list all refuse.
func Authorized(id Identity, action string, adapters []string, roles map[string][]string) bool {
	if errs := ValidateConfig(adapters, roles); len(errs) > 0 {
		return false
	}
	if !Qualified(id, adapters) {
		return false
	}
	return in(roles[id.Role], action)
}

func in(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func allNonEmpty(list []string) bool {
	if len(list) == 0 {
		return false
	}
	for _, x := range list {
		if x == "" {
			return false
		}
	}
	return true
}

func hasDuplicates(list []string) bool {
	seen := map[string]bool{}
	for _, x := range list {
		if seen[x] {
			return true
		}
		seen[x] = true
	}
	return false
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
