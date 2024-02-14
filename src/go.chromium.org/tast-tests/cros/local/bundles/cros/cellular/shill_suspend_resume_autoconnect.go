// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type autoconnectTestParams struct {
	autoconnectState bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillSuspendResumeAutoconnect,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that cellular maintains autoconnect state around Suspend/Resume",
		Contacts:       []string{"chromeos-cellular-team@google.com", "danielwinkler@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_suspend", "cellular_run_isolated"},
		Fixture:        "cellular",
		Timeout:        4 * time.Minute,
		// TODO(b/217106877): Skip on herobrine as S/R is unstable
		HardwareDeps: hwdep.D(hwdep.SkipOnPlatform("herobrine")),
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "enabled",
			Val: autoconnectTestParams{
				autoconnectState: true,
			},
		}, {
			Name: "disabled",
			Val: autoconnectTestParams{
				autoconnectState: false,
			},
		}},
	})
}

func ShillSuspendResumeAutoconnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	params := s.Param().(autoconnectTestParams)

	expectedStates := map[bool]string{true: shillconst.ServiceStateOnline,
		false: shillconst.ServiceStateIdle}

	helper := s.FixtValue().(*cellular.FixtData).Helper

	//Chrome's eSIM cache may not exist because fixtures may delete Chrome's state.
	//Login to Chrome first and wait for Chrome to refresh its cache via Hermes.
	//This allows us to do suspend-resume-autoconnect testing without interference
	//from Chrome/Hermes.
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome (precondition): ", err)
	}
	defer cr.Close(cleanupCtx)

	if err := hermes.WaitForChromeESIMCache(ctx, 2*time.Second); err != nil {
		s.Log("Modem may got inhibited after resume")
	} else {
		if err := hermes.WaitForHermesIdle(ctx, 120*time.Second); err != nil {
			testing.ContextLog(ctx, "Could not confirm if Hermes is idle: ", err)
		}
	}

	// Disable Ethernet and/or WiFi if present and defer re-enabling.
	// Shill documentation shows that autoconnect will only be used if there
	// is no other service available, so it is necessary to only have
	// cellular available.
	if enableFunc, err := helper.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Unable to disable Ethernet (precondition): ", err)
	} else if enableFunc != nil {
		newCtx, cancel := ctxutil.Shorten(ctx, shill.EnableWaitTime)
		defer cancel()
		defer enableFunc(ctx)
		ctx = newCtx
	}
	if enableFunc, err := helper.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyWifi); err != nil {
		s.Fatal("Unable to disable Wifi (precondition): ", err)
	} else if enableFunc != nil {
		newCtx, cancel := ctxutil.Shorten(ctx, shill.EnableWaitTime)
		defer cancel()
		defer enableFunc(ctx)
		ctx = newCtx
	}

	// Get service to set autoconnect based on test parameters.
	if _, err := helper.SetServiceAutoConnect(ctx, params.autoconnectState); err != nil {
		s.Fatal("Failed to enable AutoConnect: ", err)
	}

	// Request suspend for 10 seconds.
	if err := power.SuspendAndResume(ctx, cr, 10*time.Second); err != nil {
		s.Fatal("Failed to perform system suspend (precondition): ", err)
	}

	// Try to re-login in case it is logged out after resume
	if _, err = cr.TestAPIConn(ctx); err != nil {
		s.Fatal("Failed to establish the Test API connection: ", err)
	}

	if err := helper.WaitForEnabledState(ctx, true); err != nil {
		s.Fatal("Cellular not enabled after resume")
	}

	service, err := helper.FindServiceForDevice(ctx)
	if err != nil {
		s.Fatal("Unable to find Cellular Service for Device: ", err)
	}

	// Ensure service's state matches expectations.
	if err := service.WaitForProperty(ctx, shillconst.ServicePropertyState, expectedStates[params.autoconnectState], 60*time.Second); err != nil {
		s.Fatal("Failed to get service state: ", err)
	}
}
