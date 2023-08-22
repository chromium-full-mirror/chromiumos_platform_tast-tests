// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.DeskAPILacros,
		Desc:            "Fixture providing Desk API feature access for lacros browser",
		Contacts:        []string{"chromeos-commercial-remote-management@google.com", "cros-commercial-productivity-eng@google.com", "aprilzhou@google.com"},
		Impl:            &deskLacrosFixt{},
		SetUpTimeout:    15 * time.Minute,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.graphics.ScreenshotService",
			"tast.cros.tape.Service",
			"tast.cros.browser.ChromeService",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

type deskLacrosFixt struct {
	accManager *tape.OwnedTestAccountManager
}

func (e *deskLacrosFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	username, password, accManager := DeskAPISetup(ctx, s, true)
	e.accManager = accManager
	return DeskFixtData{Username: username, Password: password}
}

func (e *deskLacrosFixt) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}
	// Release accounts.
	e.accManager.CleanUp(ctx)
}

func (*deskLacrosFixt) Reset(ctx context.Context) error                        { return nil }
func (*deskLacrosFixt) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (*deskLacrosFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {}
