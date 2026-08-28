// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/expandable"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SimLockSettingOnOff,
		Desc: "Checks that SIM Lock in Settings PIN locks and unlocks the SIM",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		SoftwareDeps:   []string{"chrome"},
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:           []string{"group:cellular", "cellular_sim_pinlock", "cellular_e2e"},
		Fixture:     "cellularSIMLockCleared",
		TestBedDeps: []string{"sim_state:WORKING"},
	})
}

func SimLockSettingOnOff(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 45*time.Second)
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
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}
	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	app, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer app.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "os_settings")

	currentPin, currentPuk, err := helper.GetPINAndPUKForICCID(ctx, iccid)
	if err != nil {
		s.Fatal("Could not get Pin and Puk: ", err)
	}
	if currentPin == "" {
		// Do graceful exit, not to run tests on unknown pin duts.
		s.Fatalf("Failed to find PIN code for ICCID : %s, skipping the test", iccid)
	}
	if currentPuk == "" {
		// Do graceful exit, not to run tests on unknown puk duts.
		s.Fatalf("Failed to find PUK code for ICCID : %s, skipping the test", iccid)
	}
	defer func(ctx context.Context) {
		if err := helper.ClearSIMLock(ctx, currentPin, currentPuk); err != nil {
			s.Fatal("Failed to clear PIN/PUK lock: ", err)
		}
		if errs := helper.ResetShill(ctx); errs != nil {
			s.Fatal("Failed to reset shill: ", errs)
		}
	}(cleanupCtx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	ui := uiauto.New(tconn).WithTimeout(120 * time.Second)
	if err := uiauto.Combine("Toggle on the SIM Lock setting",
		ui.LeftClick(ossettings.ActiveCellularBtn),
		ui.WithTimeout(90*time.Second).WaitUntilExists(ossettings.ConnectedStatus),
		expandable.EnsureExpandableSectionOpened(tconn, ossettings.CellularAdvanced),
		ui.LeftClick(ossettings.LockSimToggle),
		ui.WaitUntilExists(ossettings.EnterButton),
		kb.TypeAction(currentPin),
		ui.LeftClick(ossettings.EnterButton),

		// When the SIM Lock toggle is focusable and focused, the SIM Lock setting was successfully turned on.
		ui.WaitUntilExists(ossettings.LockSimToggle.Focusable().Focused()),
	)(ctx); err != nil {
		s.Fatal("Failed at ToggleOn in UI: ", err)
	}

	if !helper.IsSimLockEnabled(ctx) {
		s.Fatal("Failed to turn on PIN lock")
	}

	if err := uiauto.Combine("Toggle off the SIM Lock setting",
		ui.LeftClick(ossettings.LockSimToggle),
		ui.WaitUntilExists(ossettings.EnterButton),
		kb.TypeAction(currentPin),
		ui.LeftClick(ossettings.EnterButton),

		// When the SIM Lock toggle is focusable and focused, the SIM Lock setting was successfully turned off.
		ui.WaitUntilExists(ossettings.LockSimToggle.Focusable().Focused()),
	)(ctx); err != nil {
		s.Fatal("Failed at ToggleOff in UI: ", err)
	}
}
