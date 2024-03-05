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
)

// PreprodEnv is the preprod environment.
type PreprodEnv struct {
	*BaseEnv
}

// NewPreprodEnv creates a PreprodEnv instance.
func NewPreprodEnv(ctx context.Context, opts ...Option) (Env, error) {
	opts = append(opts, HostMap(googleHostMapVar.Value()))
	base, err := NewBase(ctx, Preprod, opts...)
	if err != nil {
		return nil, err
	}
	return &PreprodEnv{BaseEnv: base}, nil
}

// Close stops the running servers and cleans up any resources used to set up this environment.
func (e *PreprodEnv) Close(ctx context.Context) error {
	testing.ContextLogf(ctx, "Tearing down %v env", e.Name)
	return e.BaseEnv.Close(ctx)
}
