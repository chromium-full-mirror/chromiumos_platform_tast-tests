// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

// Runtime vars of host maps and route rules per each test environment
// FakeEnv
var (
	fakeHostMapVar = testing.RegisterVarString(
		"testenv.FakeEnv.HostMap",
		"",
		"A map of fake host names keyed by labels")

	fakeRouteRulesVar = testing.RegisterVarString(
		"testenv.FakeEnv.RouteRules",
		"",
		"A map of fake host route rules")
)

// FakeEnv is a fake environment used to test the interfaces of Env.
type FakeEnv struct {
	*baseEnv
}

// NewFakeEnv creates a FakeEnv instance.
func NewFakeEnv(ctx context.Context) (Env, error) {
	e, err := newBaseEnv(ctx,
		Fake, fakeHostMapVar.Value(), fakeRouteRulesVar.Value())
	if err != nil {
		return nil, err
	}
	return &FakeEnv{baseEnv: e}, nil
}

// SetUp sets up the fake environment.
func (p *FakeEnv) SetUp(ctx context.Context) error {
	testing.ContextLogf(ctx, "Setting up %v env with hosts: %v", p.name, p.registry.hostMap)
	return nil
}

// TearDown cleans up any changes made to set up the fake environment.
func (p *FakeEnv) TearDown(ctx context.Context) error {
	testing.ContextLogf(ctx, "Tearing down %v env", p.name)
	return nil
}
