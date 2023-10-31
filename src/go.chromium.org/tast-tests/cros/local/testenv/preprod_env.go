// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"

	"go.chromium.org/tast/core/errors"
)

// PreprodEnv is the preprod environment.
type PreprodEnv struct {
	*baseEnv
}

// NewPreprodEnv creates a PreprodEnv instance.
func NewPreprodEnv(ctx context.Context) (Env, error) {
	e, err := newBaseEnv(ctx,
		Preprod, fakeHostMapVar.Value() /* replace */, fakeRouteRulesVar.Value() /* replace */)
	if err != nil {
		return nil, err
	}
	return &PreprodEnv{baseEnv: e}, nil
}

// SetUp sets up the preprod environment.
func (p *PreprodEnv) SetUp(ctx context.Context) error {
	// TODO: Implement host overrides for the given route rules
	return errors.New("Not implemented")
}

// TearDown cleans up any changes made for the preprod environment.
func (p *PreprodEnv) TearDown(ctx context.Context) error {
	// TODO: Clean up any changes made for this environment
	return errors.New("Not implemented")
}
