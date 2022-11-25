// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.CleanOwnership,
		Desc: "Fixture cleaning TPM ownership and system state. DUT might reboot before and after all tests using the fixture",
		Contacts: []string{
			"cros-hwsec@google.com",
			"cylai@google.com",
			"yich@google.com"},
		Impl:            &cleanOwner{},
		SetUpTimeout:    3 * time.Minute,
		TearDownTimeout: 5 * time.Second,
		ResetTimeout:    5 * time.Second,
		PostTestTimeout: 3 * time.Minute,
		ServiceDeps:     []string{"tast.cros.hwsec.OwnershipService"},
	})
}

type cleanOwner struct {
}

func (co *cleanOwner) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}
	return nil
}

// Clean up ownership between each pair of tests and after the last one.
func (co *cleanOwner) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}
}

func (co *cleanOwner) Reset(ctx context.Context) error {
	return nil
}

func (co *cleanOwner) PreTest(ctx context.Context, s *testing.FixtTestState) {
}
func (co *cleanOwner) TearDown(ctx context.Context, s *testing.FixtState) {
}
