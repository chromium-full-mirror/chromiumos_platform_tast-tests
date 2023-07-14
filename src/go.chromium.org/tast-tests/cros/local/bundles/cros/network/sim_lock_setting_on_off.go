// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SimLockSettingOnOff,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that SIM Lock in Settings PIN locks and unlocks the SIM",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		SoftwareDeps: []string{"chrome"},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_pinlock", "cellular_e2e"},
		Fixture:      "cellular",
		Vars:         []string{"autotest_host_info_labels"},
	})
}

func SimLockSettingOnOff(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if _, err := modemmanager.NewModemWithSim(ctx); err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	// Gather Shill Device sim properties.
	labels, err := cellular.GetLabelsAsStringArray(ctx, s.Var, "autotest_host_info_labels")
	if err != nil {
		s.Fatal("Failed to read autotest_host_info_labels: ", err)
	}

	// Get cellular helper, used to determine if SIM is actually PIN locked/unlocked.
	helper, err := cellular.NewHelperWithLabels(ctx, labels)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	// Enable and get service to set autoconnect based on test parameters.
	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Failed to enable modem")
	}

	app, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	defer app.Close(ctx)

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

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

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		helper.ClearSIMLock(ctx, currentPin, currentPuk)
	}(cleanupCtx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Toggle on the SIM Lock setting",
		ui.LeftClick(ossettings.ActiveCellularBtn),
		ui.WaitUntilExists(ossettings.ConnectedStatus),
		ui.WithTimeout(90*time.Second).LeftClick(ossettings.CellularAdvanced),
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
