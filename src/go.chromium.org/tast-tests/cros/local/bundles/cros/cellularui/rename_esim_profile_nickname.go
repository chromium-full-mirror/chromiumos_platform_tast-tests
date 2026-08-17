// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellularui

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RenameESimProfileNickname,
		Desc: "Renames eSIM profiles name via the UI",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:cellular", "cellular_sim_dual_active"},
		SoftwareDeps:   []string{"chrome"},
		Fixture:        "cellular",
		Timeout:        6 * time.Minute,
	})
}

func RenameESimProfileNickname(ctx context.Context, s *testing.State) {
	euicc, _, err := hermes.GetEUICC(ctx, false)
	if err != nil {
		s.Fatal("Could not get Hermes euicc")
	}

	testing.ContextLog(ctx, "Looking for installed profile")
	profiles, err := euicc.InstalledProfiles(ctx, false)
	if err != nil {
		s.Fatal("Could not get Hermes installed profiles")
	}

	if len(profiles) < 2 {
		s.Fatal("There are less than 2 installed profiles")
	}

	if err := euicc.EnableAnyProfile(ctx); err != nil {
		s.Fatal("Could not enable any profiles: ", err)
	}

	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Failed to create modem: ", err)
	}
	if _, err := modem.SetPrimarySimSlot(ctx, 2); err != nil {
		testing.ContextLog(ctx, "Failed to set primary SIM slot to 2 (eSIM): ", err)
	}

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

	// TODO(b/358402911): Remove this function once we no longer need it for debugging.
	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn, cr)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "recording.webm"), s.HasError)

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ossettings")

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	for i := range profiles {
		if err := ossettings.WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
			s.Fatal("Failed to wait until refresh profile complete: ", err)
		}

		profileDetailBtn := nodewith.HasClass("subpage-arrow").Role(role.Button).Focusable().Nth(i)
		if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(profileDetailBtn)(ctx); err != nil {
			s.Fatalf("Failed to find eSIM profile %d in list: %v", i, err)
		}

		if err := ui.LeftClick(profileDetailBtn)(ctx); err != nil {
			s.Fatalf("Failed to click into eSIM profile %d details: %v", i, err)
		}

		// GoBigSleepLint: Give some time to modem and shill to stabilize.
		if err := testing.Sleep(ctx, 10*time.Second); err != nil {
			s.Fatal("Failed to wait for 10 seconds: ", err)
		}

		if err := testRenameProfile(ctx, tconn); err != nil {
			s.Fatalf("Failed to rename profile %d: %v", i, err)
		}

		// Go back to mobile data page.
		if err := ui.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
			s.Fatal("Could not go back to mobile data page: ", err)
		}

		if err := ui.WaitUntilExists(ossettings.MobileDataToggle)(ctx); err != nil {
			s.Fatal("Did not navigate to mobile data page: ", err)
		}
	}
}

func testRenameProfile(ctx context.Context, tconn *chrome.TestConn) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kb.Close(cleanupCtx)

	ui := uiauto.New(tconn).WithTimeout(5 * time.Minute)

	if err := clickRenameProfileButton(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to click rename profile button")
	}

	if err := uiauto.Combine("Change profile name",
		ui.WaitUntilExists(ossettings.CancelButton),
		kb.TypeAction("0"),
		ui.LeftClick(ossettings.RenameProfileDoneButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "could not change profile name")
	}

	if err := clickRenameProfileButton(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to click rename profile button")
	}

	if err := uiauto.Combine("Change profile name back",
		ui.WaitUntilExists(ossettings.CancelButton),
		kb.AccelAction("Backspace"),
		ui.LeftClick(ossettings.RenameProfileDoneButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "did not change profile name back to original")
	}

	return nil
}

func clickRenameProfileButton(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	// More actions button may be temporarily disabled if cellular is connecting or disconnecting.
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(ossettings.MoreActionsBtn.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "failed to show more actions button")
	}

	if err := ui.LeftClickUntil(ossettings.MoreActionsBtn, ui.Exists(ossettings.RenameProfileBtn))(ctx); err != nil {
		return errors.Wrap(err, "failed to click more actions button")
	}

	if err := ui.LeftClick(ossettings.RenameProfileBtn)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on rename profile button")
	}

	return nil
}
