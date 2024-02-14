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
)

// FakeEnv is a fake environment used to test the interfaces of Env.
type FakeEnv struct {
	*BaseEnv
}

// NewFakeEnv creates a FakeEnv instance.
func NewFakeEnv(ctx context.Context) (Env, error) {
	base, err := NewBase(ctx, Fake, HostMap(fakeHostMapVar.Value()))
	if err != nil {
		return nil, err
	}
	return &FakeEnv{BaseEnv: base}, nil
}

// Start starts the fake environment.
func (e *FakeEnv) Start(ctx context.Context) error {
	testing.ContextLogf(ctx, "Setting up %v env with hosts: %v", e.Name, e.hostMap)
	return nil
}

// Stop cleans up any changes made to set up the fake environment.
func (e *FakeEnv) Stop(ctx context.Context) error {
	testing.ContextLogf(ctx, "Stopping %v env", e.Name)
	return nil
}
