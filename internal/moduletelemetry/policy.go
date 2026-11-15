// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

// Package moduletelemetry contains the building blocks for module-declared
// usage telemetry (see hashicorp/terraform#39521). It is intentionally free of
// dependencies on the rest of Terraform so it can be wired into configuration,
// plan, state, and apply handling incrementally.
//
// Nothing in this package is an external interface yet. Names, environment
// variables, and payload shapes are all subject to change.
package moduletelemetry

import (
	"fmt"
	"net/url"
	"strconv"
)

// Environment variables that mirror the root-level terraform { telemetry {} }
// settings. The issue does not fix names, so these are proposals.
const (
	EnvEnabled          = "TF_MODULE_TELEMETRY"
	EnvAllowNonRegistry = "TF_MODULE_TELEMETRY_ALLOW_NON_REGISTRY"
	EnvEndpoint         = "TF_MODULE_TELEMETRY_ENDPOINT"
)

// RootSettings holds the values explicitly set in the root configuration's
// terraform { telemetry {} } block. A nil field means "not set".
type RootSettings struct {
	Enabled          *bool
	AllowNonRegistry *bool
	Endpoint         *string
}

// Policy is the effective, resolved telemetry policy for a run.
type Policy struct {
	// Enabled controls all module telemetry, including nested modules.
	Enabled bool
	// AllowNonRegistry permits reporting for modules not from the public
	// Terraform Registry.
	AllowNonRegistry bool
	// EndpointOverride, when non-empty, replaces every module-declared
	// endpoint.
	EndpointOverride string
}

// DefaultPolicy is used when neither the root configuration nor the
// environment sets anything.
func DefaultPolicy() Policy {
	return Policy{Enabled: true, AllowNonRegistry: false}
}

// ResolvePolicy applies the precedence rules from the proposal: an explicit
// root setting wins over its environment variable, the environment variable
// wins over the default. getenv is usually os.LookupEnv.
func ResolvePolicy(root RootSettings, getenv func(string) (string, bool)) (Policy, error) {
	p := DefaultPolicy()

	if root.Enabled != nil {
		p.Enabled = *root.Enabled
	} else if v, ok := getenv(EnvEnabled); ok && v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Policy{}, fmt.Errorf("invalid %s: %w", EnvEnabled, err)
		}
		p.Enabled = b
	}

	if root.AllowNonRegistry != nil {
		p.AllowNonRegistry = *root.AllowNonRegistry
	} else if v, ok := getenv(EnvAllowNonRegistry); ok && v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Policy{}, fmt.Errorf("invalid %s: %w", EnvAllowNonRegistry, err)
		}
		p.AllowNonRegistry = b
	}

	if root.Endpoint != nil {
		if err := ValidateEndpoint(*root.Endpoint); err != nil {
			return Policy{}, fmt.Errorf("terraform.telemetry.endpoint: %w", err)
		}
		p.EndpointOverride = *root.Endpoint
	} else if v, ok := getenv(EnvEndpoint); ok && v != "" {
		if err := ValidateEndpoint(v); err != nil {
			return Policy{}, fmt.Errorf("%s: %w", EnvEndpoint, err)
		}
		p.EndpointOverride = v
	}

	return p, nil
}

// ValidateEndpoint enforces the transport rules from the proposal: HTTPS only,
// a host is required, and no credentials may be embedded in the URL (version 1
// has no authentication support).
func ValidateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("endpoint must use https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint must include a host")
	}
	if u.User != nil {
		return fmt.Errorf("endpoint must not contain credentials")
	}
	return nil
}

// EffectiveEndpoint returns the endpoint to use for a module instance: the
// root override if set, otherwise the module-declared endpoint. An empty
// result means "do not send".
func (p Policy) EffectiveEndpoint(moduleEndpoint string) string {
	if p.EndpointOverride != "" {
		return p.EndpointOverride
	}
	return moduleEndpoint
}

// ShouldReport reports whether an event for a module with the given source
// address may be sent under this policy.
func (p Policy) ShouldReport(source string) bool {
	if !p.Enabled {
		return false
	}
	return p.AllowNonRegistry || IsRegistrySource(source)
}
