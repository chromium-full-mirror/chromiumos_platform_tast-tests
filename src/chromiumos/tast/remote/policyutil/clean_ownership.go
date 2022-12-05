// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
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
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    3 * time.Minute,
		ServiceDeps:     []string{"tast.cros.hwsec.OwnershipService"},
	})
}

type cleanOwner struct {
	// dut and rpcHint hold references to objects that are not otherwise available in Reset().
	dut     *dut.DUT
	rpcHint *testing.RPCHint
}

func (co *cleanOwner) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}
	co.dut = s.DUT()
	co.rpcHint = s.RPCHint()
	return nil
}

func (co *cleanOwner) TearDown(ctx context.Context, s *testing.FixtState) {
	// After the last test we want to clean ownership.
	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}
}

func (co *cleanOwner) Reset(ctx context.Context) error {
	// Between each tests we want to clean ownership. The last test won't have
	// this executed hence we have to do it in TearDown().
	if err := EnsureTPMAndSystemStateAreReset(ctx, co.dut, co.rpcHint); err != nil {
		return errors.Wrap(err, "failed to reset TPM")
	}
	return nil
}

func (co *cleanOwner) PreTest(ctx context.Context, s *testing.FixtTestState) {
}
func (co *cleanOwner) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
