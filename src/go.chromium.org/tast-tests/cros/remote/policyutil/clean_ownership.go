// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.CleanOwnership,
		Desc: "Fixture cleaning TPM ownership and system state. DUT might reboot before and after all tests using the fixture",
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@google.com"},
		Impl:            &cleanOwner{},
		SetUpTimeout:    3*time.Minute + /* b/239013478 */ 2*time.Minute,
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
	// Make sure the DUT is connected at the beginning.
	// TODO(b/239013478): Clean up the connection checks when the issue is resolved.
	if err := s.DUT().Health(ctx); err != nil {
		s.Log("Failed DUT connection check at the beginning: ", err)

		// Try to reconnect to the DUT.
		waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if err := s.DUT().WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to the DUT at the beginning: ", err)
		}
	}

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
