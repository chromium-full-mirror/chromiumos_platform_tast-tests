// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ToggleCellularFromOSSettings,
		Desc: "Checks that Cellular can be enabled and disabled from the OS Settings",
		Contacts: []string{
			"cros-device-enablement@google.com",
		},
		BugComponent:   "b:1131774", // ChromeOS > Software > Fundamentals > Device Enablement > Connectivity > Cellular
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		SoftwareDeps:   []string{"chrome"},
		Attr:           []string{"group:cellular", "cellular_sim_active", "cellular_carrier_agnostic"},
		Fixture:        "cellularE2ELocal",
	})
}

// ToggleCellularFromOSSettings tests that a user can successfully toggle the
// Cellular state using the Cellular toggle on the OS Settings page.
func ToggleCellularFromOSSettings(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Disable auto-connect so that enabling cellular data will not automatically trigger a connection, reducing instability caused by connection attempts.
	cleanup, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, false)
	if err != nil {
		s.Fatal("Failed to initialize autoconnect to false: ", err)
	}
	defer cleanup(cleanupCtx)

	app, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS Settings: ", err)
	}

	defer app.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	if err := app.SetToggleOption(cr, "Mobile data enable", true)(ctx); err != nil {
		s.Fatal("Failed to enable mobile data in UI: ", err)
	}
	if err := helper.WaitForEnabledState(ctx, true); err != nil {
		s.Fatal("Failed to enable Cellular state: ", err)
	}

	state := false
	const iterations = 5
	for i := 0; i < iterations; i++ {
		s.Logf("Toggling Cellular (iteration %d of %d)", i+1, iterations)

		if err := app.SetToggleOption(cr, "Mobile data enable", state)(ctx); err != nil {
			s.Fatal("Failed to enable mobile data in UI: ", err)
		}
		if err := helper.WaitForEnabledState(ctx, state); err != nil {
			s.Fatal("Failed to toggle Cellular state: ", err)
		}
		state = !state
	}
}
