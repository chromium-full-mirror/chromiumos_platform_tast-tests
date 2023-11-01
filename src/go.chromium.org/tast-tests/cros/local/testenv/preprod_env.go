// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

// Runtime vars of host maps and route rules per each test environment
// PreprodEnv (internals from vars in private)
var (
	googleHostMapVar = testing.RegisterVarString(
		"testenv.Google.HostMap",
		"",
		"A map of Google service host names keyed by labels")

	googleRouteRulesVar = testing.RegisterVarString(
		"testenv.Google.RouteRules",
		"",
		"A map of host route rules for Google services")
)

// PreprodEnv is the preprod environment.
type PreprodEnv struct {
	*baseEnv
}

// NewPreprodEnv creates a PreprodEnv instance.
func NewPreprodEnv(ctx context.Context) (Env, error) {
	e, err := newBaseEnv(ctx,
		Preprod, googleHostMapVar.Value(), googleRouteRulesVar.Value())
	if err != nil {
		return nil, err
	}
	return &PreprodEnv{baseEnv: e}, nil
}

// SetUp sets up the preprod environment.
func (p *PreprodEnv) SetUp(ctx context.Context) error {
	// TODO: Implement host overrides for the given route rules
	testing.ContextLogf(ctx, "Setting up %v env with hosts: %v", p.name, p.registry.hostMap)
	return nil
}

// TearDown cleans up any changes made for the preprod environment.
func (p *PreprodEnv) TearDown(ctx context.Context) error {
	testing.ContextLogf(ctx, "Tearing down %v env", p.name)
	return nil
}
