// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
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
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_roaming", "cellular_unstable"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "cellular",
		Timeout:      2 * time.Minute,
	})
}

func CellularRoamingPerProfileOrSim(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	ctxForCleanUp := ctx

	cleanup, err := helper.InitDeviceProperty(ctx, shillconst.DevicePropertyCellularPolicyAllowRoaming, false)
	if err != nil {
		s.Fatal("Could not set PolicyAllowRoaming to false: ", err)
	}
	defer cleanup(ctxForCleanUp)
	cleanup2, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, false)
	if err != nil {
		s.Fatal("Could not set AutoConnect to false: ", err)
	}
	defer cleanup2(ctxForCleanUp)

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
	firstCellularRow := cellularRow.First()
	firstCellularRowBtn := nodewith.HasClass("subpage-arrow").Role(role.Button).Ancestor(firstCellularRow).Focusable()

	if err := uiauto.Combine("Go to details page of first cellular network",
		mdp.WaitUntilExists(firstCellularRowBtn),
		mdp.LeftClick(firstCellularRowBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to go to details page of first cellular network: ", err)
	}

	if err := enableRoamingAndConnect(ctx, tconn, cr, mdp, firstCellularRowBtn); err != nil {
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
	)(ctx); err == nil {
		if err := enableRoamingAndConnect(ctx, tconn, cr, mdp, secondCellularRowBtn); err != nil {
			s.Fatal("Failed to connect to second network: ", err)
		}

		if err := mdp.LeftClick(ossettings.BackArrowBtn)(ctx); err != nil {
			s.Fatal("Failed to go back to mobile data page: ", err)
		}

		if err := ossettings.WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
			s.Fatal("Failed to wait until refresh profile complete: ", err)
		}

		if err := uiauto.Combine("Reconnect with first cellular row",
			mdp.WaitUntilExists(firstCellularRow.Focusable()),
			mdp.LeftClick(firstCellularRow),
		)(ctx); err != nil {
			s.Fatal("Failed to make click on first cellular row: ", err)
		}
	}
}

func enableRoamingAndConnect(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, mdp *ossettings.OSSettings, entryBtn *nodewith.Finder) error {
	roamingToggleLabel := "Allow mobile data roaming"
	ui := uiauto.New(tconn)

	// If auto-connect is on, the network could already be connected. Auto-connect will be disabled and
	// roaming will also be disabled. Once roaming is disabled, the network should disconnect on its own.
	if err := ui.EnsureGoneFor(ossettings.DisconnectedStatus, 5*time.Second)(ctx); err == nil {
		if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.AutoconnectToggle)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for auto connect toggle to show")
		}
		if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(ossettings.RoamingToggle)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for roaming toggle to show")
		}
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

	if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(entryBtn)(ctx); err == nil {
		if err := ui.LeftClick(entryBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to navigate to cellular details page")
		}
	}
	if err := uiauto.Combine("Ensure connecting label gone",
		ui.WithTimeout(15*time.Second).WaitUntilEnabled(ossettings.RoamingToggle),
		ui.WithTimeout(15*time.Second).WaitUntilGone(ossettings.ConnectingStatus),
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
		if err := ui.WithTimeout(45 * time.Second).WaitUntilExists(ossettings.ConnectedStatus)(ctx); err != nil {
			return errors.Wrap(err, "failed to verify connected")
		}
	}

	return nil
}
