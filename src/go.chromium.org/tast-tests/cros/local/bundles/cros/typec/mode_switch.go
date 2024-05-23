// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ModeSwitch,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks USB Type C mode switch behaviour on login",
		Contacts:     []string{"chromeos-usb-champs@google.com", "pmalani@chromium.org"},
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_lab"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Data:         []string{"testcert.p12"},
	})
}

// ModeSwitch does the following:
// - Go to the login screen.
// - Validate USB+DP alt mode is working correctly.
// - Login.
// - Validate that TBT alt mode is working correctly.
//
// This test requires the following H/W topology to run.
//
//	  DUT ------> Thunderbolt3 (>= Titan Ridge) dock -----> DP monitor.
//	(USB4)
func ModeSwitch(ctx context.Context, s *testing.State) {
	// This check is for test executions which take place on
	// CQ (where TBT peripherals aren't connected).
	port, err := typecutils.CheckPortsForTBTPartner(ctx)
	if err != nil {
		s.Fatal("Couldn't determine TBT device from PD identity: ", err)
	}

	if port == -1 {
		s.Fatal("No TBT device connected to DUT")
	}

	// Get to the Chrome login screen.
	cr, err := chrome.New(ctx,
		chrome.DeferLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))
	if err != nil {
		s.Fatal("Failed to start Chrome at login screen: ", err)
	}
	defer cr.Close(ctx)

	testConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get Test API connection: ", err)
	}

	if err := typecutils.EnablePeripheralDataAccess(ctx, s.DataPath("testcert.p12")); err != nil {
		s.Fatal("Failed to enable peripheral data access setting: ", err)
	}

	s.Log("Verifying that no TBT devices enumerated")

	err = testing.Poll(ctx, func(ctx context.Context) error {
		return typecutils.CheckTBTDevice(false)
	}, &testing.PollOptions{Interval: 100 * time.Millisecond, Timeout: 10 * time.Second})
	if err != nil {
		s.Fatal("Failed to verify no TBT devices connected at login screen: ", err)
	}

	s.Log("Verifying that DP monitor enumerated correctly")

	err = testing.Poll(ctx, func(ctx context.Context) error {
		return typecutils.FindConnectedDPMonitor(ctx, testConn)
	}, &testing.PollOptions{Interval: 200 * time.Millisecond, Timeout: 20 * time.Second})
	if err != nil {
		s.Fatal("Failed to verify DP monitor working at login screen: ", err)
	}

	if err := cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	s.Log("Verifying that TBT device & DP monitor enumerated correctly")

	if err := typecutils.CheckTBTAndDP(ctx, testConn); err != nil {
		s.Fatal("Failed to verify TBT & DP after login: ", err)
	}
}
