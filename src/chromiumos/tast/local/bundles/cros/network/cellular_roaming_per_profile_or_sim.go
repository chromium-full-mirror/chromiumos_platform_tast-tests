// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/common/shillconst"
	"chromiumos/tast/local/cellular"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularRoamingPerProfileOrSim,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests connecting to two cellular networks that require roaming one after the next",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:cellular", "cellular_sim_roaming", "cellular_unstable"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      2 * time.Minute,
	})
}

func CellularRoamingPerProfileOrSim(ctx context.Context, s *testing.State) {
	helper, _, err := cellular.NewHelperWithSim(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper (precondition): ", err)
	}
	ctxForCleanUp := ctx

	cleanup, err := helper.InitDeviceProperty(ctx, shillconst.DevicePropertyCellularPolicyAllowRoaming, false)
	if err != nil {
		s.Fatal("Could not set PolicyAllowRoaming to false: ", err)
	}
	defer cleanup(ctxForCleanUp)

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data page")
	}

	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	cellularRow := nodewith.NameRegex(regexp.MustCompile(".*Connect")).HasClass("horizontal")

	// Finder for row to first cellular network
	firstCellularRowBtn := nodewith.HasClass("subpage-arrow").Role(role.Button).Ancestor(cellularRow.First()).Focusable()

	if err := uiauto.Combine("Go to details page of first cellular network",
		mdp.WaitUntilExists(firstCellularRowBtn),
		mdp.LeftClick(firstCellularRowBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to go to details page of first cellular network: ", err)
	}

	if err := enableRoamingAndConnect(ctx, tconn, cr, mdp); err != nil {
		s.Fatal("Failed to connect to first network: ", err)
	}

	if err := mdp.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
		s.Fatal("Failed to go back to mobile data page: ", err)
	}

	if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		s.Fatal("Failed to wait until refresh profile complete: ", err)
	}

	// Finder for row to second cellular network
	secondCellularRowBtn := nodewith.HasClass("subpage-arrow").Role(role.Button).Ancestor(cellularRow.Nth(1)).Focusable()

	if err := uiauto.Combine("Go to details page of second cellular network",
		mdp.WaitUntilExists(secondCellularRowBtn),
		mdp.LeftClick(secondCellularRowBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to go to details page of second cellular network: ", err)
	}

	if err := enableRoamingAndConnect(ctx, tconn, cr, mdp); err != nil {
		s.Fatal("Failed to connect to second network: ", err)
	}
}

func enableRoamingAndConnect(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, mdp *ossettings.OSSettings) error {
	roamingToggleLabel := "Allow mobile data roaming"
	ui := uiauto.New(tconn)

	// If auto-connect is on, the network could already be connected. Auto-connect will be disabled and
	// roaming will also be disabled. Once roaming is disabled, the network should disconnect on its own.
	if err := ui.EnsureGoneFor(ossettings.DisconnectedStatus, 5*time.Second)(ctx); err == nil {
		if err := mdp.SetToggleOption(cr, "Automatically connect to cellular network", false)(ctx); err != nil {
			return errors.Wrap(err, "failed to set auto-connect toggle option to false")
		}
		if err := mdp.SetToggleOption(cr, roamingToggleLabel, false)(ctx); err != nil {
			return errors.Wrap(err, "failed to set roaming toggle option to false")
		}
		if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.DisconnectedStatus)(ctx); err != nil {
			return errors.Wrap(err, "failed because SIM used in test setup does not require roaming to be connected")
		}
	}

	// Need to click connect the first time for network to become active.
	if err := uiauto.Combine("Connect to network",
		ui.WithTimeout(15*time.Second).WaitUntilEnabled(ossettings.ConnectButton),
		ui.LeftClick(ossettings.ConnectButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find Connect button")
	}

	if err := uiauto.Combine("Ensure connecting label gone",
		ui.WithTimeout(15*time.Second).WaitUntilEnabled(ossettings.RoamingToggle),
		ui.WaitUntilGone(ossettings.ConnectingStatus),
		ui.EnsureGoneFor(ossettings.ConnectingStatus, 10*time.Second),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to exit connecting state")
	}

	if err := mdp.SetToggleOption(cr, roamingToggleLabel, true)(ctx); err != nil {
		return errors.Wrap(err, "failed to set toggle option to true")
	}

	if err := ui.Exists(ossettings.ConnectedStatus)(ctx); err != nil {
		if err := ui.LeftClick(ossettings.ConnectButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to find Connect button")
		}
		if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err != nil {
			return errors.Wrap(err, "failed to verify connected")
		}
	}

	return nil
}
