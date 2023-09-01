// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RenameESimProfileNickname,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Renames connected and disconnected eSIM profiles name via the UI",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_dual_active"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      5 * time.Minute,
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

	if _, err := modemmanager.NewModemWithSim(ctx); err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}

	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}

	connectedIccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get iccid: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}
	defer mdp.Close(ctx)

	hasConnectedProfile := false
	hasDisconnectedProfile := false
	for _, profilex := range profiles {
		propsx, err := dbusutil.NewDBusProperties(ctx, profilex.DBusObject)

		iccid, err := propsx.GetString(hermesconst.ProfilePropertyIccid)
		if err != nil {
			s.Fatal("Failed to read profile ICCID: ", err)
		}

		hasConnectedProfile = hasConnectedProfile || (iccid == connectedIccid)
		hasDisconnectedProfile = hasDisconnectedProfile || (iccid != connectedIccid)
	}

	if !hasConnectedProfile {
		s.Fatal("No connected eSIM profile")
	}

	if !hasDisconnectedProfile {
		s.Fatal("No disconnected eSIM profile")
	}

	if err := ossettings.GoToActiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to connected network details: ", err)
	}

	if err := testRenameProfile(ctx, tconn); err != nil {
		s.Fatal("Failed to rename profile: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// Go back to mobile data page
	if err := ui.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
		s.Fatal("Could not go back to mobile data page: ", err)
	}

	if err := ui.WaitUntilExists(ossettings.MobileDataToggle)(ctx); err != nil {
		s.Fatal("Did not navigate to mobile data page: ", err)
	}

	if err := ossettings.GoToFirstInactiveNetworkDetails(ctx, tconn); err != nil {
		s.Fatal("Failed to go to disconnected network details: ", err)
	}

	if err := testRenameProfile(ctx, tconn); err != nil {
		s.Fatal("Failed to rename profile: ", err)
	}
}

func testRenameProfile(ctx context.Context, tconn *chrome.TestConn) error {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kb.Close(ctx)

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
