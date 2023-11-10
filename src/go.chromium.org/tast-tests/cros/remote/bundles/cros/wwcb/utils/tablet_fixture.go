// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// fixtureSetUpTimeout is a setup timeout.
const fixtureSetUpTimeout = 15 * time.Minute

// resetTimeout is a reset timeout.
const resetTimeout = 15 * time.Second

// TabletModeFixture is the test fixture used for enable or disable tablet mode.
type TabletModeFixture struct {
	// enableTableMode will ensure that tablet mode is enabled during the test
	// if true, or disabled if false.
	enableTabletMode bool
	// revertInitialModeCmd used to revert to the initial mode, whether it's tablet mode or clamshell mode.
	revertInitialModeCmd string
	// Proxy is a proxy object of the servo.
	proxy *servo.Proxy
}

// Servo obtain the proxy of a servo.
func (f *TabletModeFixture) Servo() *servo.Proxy {
	return f.proxy
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "enableServoAndTabletMode",
		Desc:     "Enables servo and tablet mode set up and disables it during tear down",
		Contacts: []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		Impl:     &TabletModeFixture{enableTabletMode: true},
		Vars: []string{
			"servo",
		},
		SetUpTimeout:    fixtureSetUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "enableServoAndDisableTabletMode",
		Desc:     "Enables servo and disables tablet mode set up and enables it during tear down",
		Contacts: []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		Impl:     &TabletModeFixture{enableTabletMode: false},
		Vars: []string{
			"servo",
		},
		SetUpTimeout:    fixtureSetUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
	})
}

// Reset does nothing currently, but is required for the test fixture.
func (f *TabletModeFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest does nothing currently, but is required for the test fixture.
func (f *TabletModeFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

// PostTest does nothing currently, but is required for the test fixture.
func (f *TabletModeFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

// SetUp sets up the test fixture and connect to servo.
func (f *TabletModeFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dut := s.DUT()
	servoSpec := s.RequiredVar("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	cmd, err := EnableDUTInTabletMode(ctx, dut, pxy, f.enableTabletMode)
	if err != nil && f.enableTabletMode {
		s.Fatal("Failed to enable tablet mode: ", err)
	} else if err != nil && !f.enableTabletMode {
		s.Fatal("Failed to disable tablet mode: ", err)
	}

	f.revertInitialModeCmd = cmd
	f.proxy = pxy
	return f
}

// TearDown release resources.
func (f *TabletModeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.proxy.Servo().RunECCommand(ctx, f.revertInitialModeCmd); err != nil {
		s.Fatal("Failed to revert to initial mode: ", err)
	}
	// Close servo proxy.
	f.proxy.Close(ctx)
}
