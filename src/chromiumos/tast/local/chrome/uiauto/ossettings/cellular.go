// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
)

// WaitUntilRefreshProfileCompletes will wait until the cellular refresh profile completes.
func WaitUntilRefreshProfileCompletes(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(5 * time.Minute)
	refreshProfileText := nodewith.NameContaining("This may take a few minutes").Role(role.StaticText)
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(refreshProfileText)(ctx); err == nil {
		if err := ui.WithTimeout(5 * time.Minute).WaitUntilGone(refreshProfileText)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait until refresh profile complete")

		}
	}
	return nil
}

// GoToCellularNetworkDetailPageWithNickName will go to the cellular details page with
// network name of |name| by clicking its the subpage arrow.
func GoToCellularNetworkDetailPageWithNickName(ctx context.Context, tconn *chrome.TestConn, name string) error {
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	var DetailButton = nodewith.NameContaining(name).ClassName("subpage-arrow").Role(role.Button)

	if err := ui.WithTimeout(2 * time.Minute).WaitUntilExists(DetailButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find cellular network with name: "+name)
	}

	if err := ui.LeftClick(DetailButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click into cellular networks detail view with name: "+name)
	}

	return nil
}

// GoToNetworkWithNickName will go to the network details page of the cellular network with the name of |name|.
func GoToNetworkWithNickName(ctx context.Context, tconn *chrome.TestConn, name string) error {
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	var NetworkButton = nodewith.NameContaining(name).Role(role.Button)

	if err := ui.WithTimeout(90 * time.Second).WaitUntilExists(NetworkButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find cellular network with name: "+name)
	}

	if err := ui.LeftClick(NetworkButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click into cellular networks detail view with name: "+name)
	}

	return nil
}

// GoToActiveNetworkDetails will go to the network details page of the active cellular network.
func GoToActiveNetworkDetails(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	if err := ui.WithTimeout(90 * time.Second).WaitUntilExists(ActiveCellularBtn)(ctx); err != nil {
		return errors.Wrap(err, "failed to find active cellular network")
	}

	activeCellularRowNodes, err := ui.NodesInfo(ctx, ActiveCellularRows)
	if err != nil {
		return errors.Wrap(err, "failed to find node info of active cellular network")
	}
	if len(activeCellularRowNodes) > 1 {
		return errors.Wrap(err, "more than one active network displayed as active")
	}

	if err := ui.LeftClick(ActiveCellularBtn)(ctx); err != nil {
		return errors.Wrap(err, "failed to click into active cellular networks detail view")
	}

	if err := uiauto.IfFailThen(ui.WithTimeout(10*time.Second).WaitUntilExists(ConnectedStatus), ui.WithTimeout(10*time.Second).WaitUntilExists(SignInToNetwork))(ctx); err != nil {
		return errors.Wrap(err, "failed to verify active cellular network in details settings page")
	}

	return nil
}

// VerifyAutoconnectStateOfActiveNetwork verifies that the autoconnect toggle of the active network matches the |enabled| state.
func VerifyAutoconnectStateOfActiveNetwork(ctx context.Context, tconn *chrome.TestConn, enabled bool) error {
	if err := GoToActiveNetworkDetails(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to go to active cellular network detail page view")
	}
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Verify network autoconnect",
		ui.WaitUntilExists(AutoconnectToggle),
		ui.WaitUntilCheckedState(AutoconnectToggle, enabled),
		ui.LeftClick(BackArrowBtn),
		ui.WaitUntilExists(ActiveCellularBtn),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify autoconnect state in network details setting page")
	}
	return nil
}

// VerifyNetworkIsActive verifies that the network with |activeIccid| is the primary active network.
func VerifyNetworkIsActive(ctx context.Context, tconn *chrome.TestConn, activeIccid string) error {
	if err := GoToActiveNetworkDetails(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to go to active cellular network detail page view")
	}

	ui := uiauto.New(tconn).WithTimeout(90 * time.Second)
	displayedIccid := nodewith.NameContaining(activeIccid).Role(role.StaticText)
	if err := uiauto.Combine("Verify network connected",
		ui.WithTimeout(30*time.Second).LeftClick(CellularAdvanced),
		ui.WithTimeout(30*time.Second).WaitUntilExists(displayedIccid),
		ui.LeftClick(BackArrowBtn),
		ui.WaitUntilExists(ActiveCellularBtn),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify network iccid in network details setting page")
	}
	return nil
}

// AddESimWithActivationCode will input an eSIM activation code, assuming the eSIM setup dialog has been launched.
func AddESimWithActivationCode(ctx context.Context, tconn *chrome.TestConn, activationCode string) error {
	if err := WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	ui := uiauto.New(tconn).WithTimeout(1 * time.Minute)

	if err := ui.LeftClick(AddCellularButton.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "failed to click the Add Cellular Button")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kb.Close()

	var setupNewProfile = nodewith.NameContaining("Set up new profile").Role(role.Button).Focusable()
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(setupNewProfile)(ctx); err == nil {
		// There are pending profiles, opt to set up a new profile instead.
		if err := ui.LeftClick(setupNewProfile)(ctx); err != nil {
			return errors.Wrap(err, "failed to click set up new profile button")
		}
	}

	var activationCodeInput = nodewith.NameRegex(regexp.MustCompile("Activation code")).Focusable().First()
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(activationCodeInput)(ctx); err != nil {
		return errors.Wrap(err, "failed to find activation code input field")
	}

	if err := ui.LeftClick(activationCodeInput)(ctx); err != nil {
		return errors.Wrap(err, "failed to find activation code input field")
	}

	if err := kb.Type(ctx, "LPA:"+activationCode); err != nil {
		return errors.Wrap(err, "could not type activation code")
	}

	if err := ui.LeftClick(NextButton.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "could not click Next button")
	}

	return nil
}

// VerifyTestESimProfile verifies that the test profile exists and navigates to the test profile.
func VerifyTestESimProfile(ctx context.Context, tconn *chrome.TestConn) error {
	if err := WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	ui := uiauto.New(tconn).WithTimeout(3 * time.Second)

	managedTestProfile := nodewith.NameRegex(regexp.MustCompile("^Network [0-9] of [0-9],.*"))
	// testProfileDetailButton is the finder for the "Test Profile" detail subpage arrow button in the mobile data page UI.
	var testProfileDetailButton = nodewith.ClassName("subpage-arrow").Role(role.Button).Ancestor(managedTestProfile.First())
	if err := ui.WithTimeout(time.Minute).WaitUntilExists(testProfileDetailButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the newly installed test profile")
	}

	if err := ui.LeftClick(testProfileDetailButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to left click Test Profile detail button")
	}
	return nil
}
