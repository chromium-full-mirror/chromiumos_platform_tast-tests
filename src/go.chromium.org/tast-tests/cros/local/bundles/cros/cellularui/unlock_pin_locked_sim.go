// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UnlockPinLockedSim,
		Desc: "Verifies that a PIN locked SIM can only be unlocked by the correct PIN, and then subsequently connected to",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		SoftwareDeps:   []string{"chrome"},
		// Run test only on cellular capable devices that only have one active SIM.
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:        []string{"group:cellular", "cellular_sim_pinlock", "cellular_e2e"},
		Fixture:     "cellularSIMLockCleared",
		Timeout:     8 * time.Minute,
		TestBedDeps: []string{"sim_state:WORKING"},
	})
}

func UnlockPinLockedSim(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	currentPin, currentPuk, err := helper.GetPINAndPUKForICCID(ctx, iccid)
	if err != nil {
		s.Fatal("Could not get PIN and PUK: ", err)
	}
	if currentPin == "" {
		// Do graceful exit, not to run tests on unknown PIN duts.
		s.Fatal("Unable to find PIN code for ICCID: ", iccid)
	}
	if currentPuk == "" {
		// Do graceful exit, not to run tests on unknown puk duts.
		s.Fatal("Unable to find PUK code for ICCID: ", iccid)
	}

	// Check if PIN enabled and locked/set.
	if helper.IsSimPinLocked(ctx) {
		s.Fatal("Precondition of an unlocked SIM was not met")
	}

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	s.Log("Attempting to enable sim lock with correct pin")
	if err = helper.Device.RequirePin(ctx, currentPin, true); err != nil {
		s.Fatal("Failed to enable pin, mostly default pin needs to set on dut: ", err)
	}

	defer func(ctx context.Context) {
		if err := helper.ClearSIMLock(ctx, currentPin, currentPuk); err != nil {
			s.Fatal("Failed to clear PIN/PUK lock: ", err)
		}
	}(cleanupCtx)

	// ResetModem needed for sim power reset to reflect locked type values.
	if _, err := helper.ResetModem(ctx); err != nil {
		s.Log("Failed to reset modem: ", err)
	}

	// Start a new chrome session
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	screenRecorder, err := uiauto.NewScreenRecorder(ctx, tconn, cr)
	if err != nil {
		s.Log("Failed to create screen recorder: ", err)
	} else {
		if screenRecorder.Start(ctx, tconn); err != nil {
			s.Log("Failed to start screen recorder: ", err)
		} else {
			defer screenRecorder.StopAndSaveOnError(cleanupCtx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
		}
	}

	// Opening the mobile data page via ossettings.OpenMobileDataSubpage() will not work if the device does not support multisim and the SIM is locked.
	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS settings: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "os_settings_ui_dump")

	ui := uiauto.New(tconn).WithTimeout(5 * time.Minute)

	goNavigateToMobileData := func() {
		mobileDataLinkNode := nodewith.Name("Mobile data").Role(role.Heading)
		if err := settings.NavigateToPageURL(ctx, cr, "networks?type=Cellular", ui.WaitUntilExists(mobileDataLinkNode)); err != nil {
			s.Fatal("Failed to navigate to mobile data page 1: ", err)
		}
	}

	goNavigateToMobileData()
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	if err := ossettings.WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil {
		s.Fatal("Could not get the Network name: ", err)
	}

	if err := ui.WaitUntilExists(ossettings.UnlockButton)(ctx); err != nil {
		// In some cases the unlock button is shown immediately and requires extra setup.
		// Clicking on the network row will prompt the network to be refreshed and would show the unlock button.
		networkRegex := fmt.Sprintf(`^Network \d+ of \d+, %s.*`, regexp.QuoteMeta(networkName))
		networkRow := nodewith.NameRegex(regexp.MustCompile(networkRegex)).ClassName("no-outline").First()
		if err := ui.LeftClick(networkRow)(ctx); err != nil {
			s.Fatal("Failed to click into network cellular row: ", err)
		}
		if err := ossettings.WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
			s.Fatal("Failed to wait until refresh profile complete: ", err)
		}
	}

	var incorrectPinSublabel = nodewith.NameContaining("Incorrect PIN").Role(role.StaticText)
	if err := uiauto.Combine("Incorrect PIN does not unlock the SIM",
		// The unlock button may remain restricted for a short periods even after the cellular profiles have refreshed.
		ui.WaitUntilEnabled(ossettings.UnlockButton),
		ui.LeftClick(ossettings.UnlockButton),
		ui.WaitUntilExists(ossettings.CancelButton),

		// Use a bad PIN.
		kb.TypeAction(currentPin+"0"),
		ui.LeftClick(ossettings.UnlockButton),
		ui.WaitUntilExists(incorrectPinSublabel),
	)(ctx); err != nil {
		s.Fatal("Unlock button can still be clicked when incorrect PIN was entered: ", err)
	}

	if err := uiauto.Combine("Correct PIN unlocks the SIM, and subsequently clicking on the network row initiates a connection",
		kb.TypeAction(currentPin),
		ui.LeftClick(ossettings.UnlockButton),
	)(ctx); err != nil {
		s.Fatal("Failed to unlock the SIM: ", err)
	}

	goNavigateToMobileData()
	var networkNameDetail = nodewith.NameContaining(networkName).Role(role.Button).ClassName("subpage-arrow").First()
	if err := uiauto.Combine("Verify network connected",
		ui.WaitUntilExists(networkNameDetail),
		ui.LeftClick(networkNameDetail),
		ui.WaitUntilExists(ossettings.ConnectedStatus),
	)(ctx); err != nil {
		s.Fatal("Failed to verify network connected via settings: ", err)
	}
}
