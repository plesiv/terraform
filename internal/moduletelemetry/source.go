// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package moduletelemetry

import (
	"regexp"
	"strings"
)

// registryHost is the public Terraform Registry.
const registryHost = "registry.terraform.io"

var embeddedCredentials = regexp.MustCompile(`://[^/@]+@`)

// IsRegistrySource reports whether a module source address refers to the
// public Terraform Registry. Only the explicit host form and the implicit
// "namespace/name/system" shorthand count; local paths, git, HTTP, and other
// registries do not.
func IsRegistrySource(source string) bool {
	s := strings.TrimSpace(source)
	if s == "" || strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") {
		return false
	}
	if strings.Contains(s, "::") || strings.Contains(s, "://") {
		return false
	}
	parts := strings.Split(s, "/")
	if strings.Contains(parts[0], ".") {
		// Explicit host form: host/namespace/name/system.
		return len(parts) == 4 && strings.EqualFold(parts[0], registryHost)
	}
	// Implicit shorthand: namespace/name/system.
	return len(parts) == 3
}

// CanonicalSource returns the source address with any embedded credentials
// removed, for reporting when non-registry sources are enabled.
func CanonicalSource(source string) string {
	return embeddedCredentials.ReplaceAllString(strings.TrimSpace(source), "://")
}
