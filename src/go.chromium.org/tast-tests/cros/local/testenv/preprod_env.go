// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"

	"go.chromium.org/tast/core/errors"
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
	cleanup func(context.Context) error
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
	testing.ContextLogf(ctx, "Setting up %v env", p.name)

	// Find the route rules whose destination is preprod.
	dests, err := p.destRules(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the destination rules for preprod")
	}

	// Update the hosts file to redirect traffic to the preprod destinations.
	hostsUpdater, cleanup, err := newHostsUpdater(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to init the hosts updater")
	}
	p.cleanup = cleanup
	if _, err := hostsUpdater.Override(ctx, dests...); err != nil {
		cleanup(ctx)
		return errors.Wrap(err, "failed to set up the preprod env")
	}
	return nil
}

// TearDown cleans up any changes made for the preprod environment.
func (p *PreprodEnv) TearDown(ctx context.Context) error {
	testing.ContextLogf(ctx, "Tearing down %v env", p.name)

	if p.cleanup == nil {
		return nil
	}
	// Reset any overridden hosts in /etc/hosts after use.
	if err := p.cleanup(ctx); err != nil {
		return errors.Wrap(err, "failed to reset the hosts file")
	}
	return nil
}
