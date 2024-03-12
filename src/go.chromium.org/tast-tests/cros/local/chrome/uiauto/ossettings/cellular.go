// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ApnConfig is struct containing information about an APN.
type ApnConfig struct {
	Name               string
	Username           string
	Password           string
	AuthenticationType string
	IPType             string
	IsAttach           bool
	IsDefault          bool
}

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

	var DetailButton = nodewith.NameContaining(name).HasClass("subpage-arrow").Role(role.Button)

	if err := ui.WithTimeout(2 * time.Minute).WaitUntilExists(DetailButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find cellular network with name: "+name)
	}

	if err := ui.LeftClick(DetailButton)(ctx); err != nil {
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

// GoToFirstInactiveNetworkDetails will go to the network details page of the first inactive cellular network.
func GoToFirstInactiveNetworkDetails(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	if err := ui.DoDefault(NotActiveCellularBtn.First())(ctx); err != nil {
		return errors.Wrap(err, "failed to click into inactive cellular networks detail view")
	}

	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(DisconnectedStatus)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify inactive cellular network in details settings page")
	}

	return nil
}

// GoToActiveNetworkApnSubpage will go to the APN subpage of the active cellular network.
func GoToActiveNetworkApnSubpage(ctx context.Context, tconn *chrome.TestConn, isFromMobileDataSubpage bool) error {
	if isFromMobileDataSubpage {
		if err := GoToActiveNetworkDetails(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to go to active cellular network detail page view")
		}
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Go to APN subpage",
		ui.WithTimeout(10*time.Second).WaitUntilExists(ApnSubpageButton.Focusable()),
		ui.LeftClick(ApnSubpageButton.Focusable()),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to go to APN subpage")
	}
	return nil
}

func getAPNTypeString(isAttach, isDefault bool) (string, error) {
	if isAttach && isDefault {
		return "APN is type default and attach.", nil
	} else if isAttach {
		return "APN is type attach.", nil
	} else if isDefault {
		return "APN is type default.", nil
	}

	return "", errors.New("Neither Attach nor Default APN")
}

// ClickAPNMoreActionsButtonOfType will click the 'More Actions' button associated to the APN with the current state |currentState|, and if it is an attach and/or default APN.
func ClickAPNMoreActionsButtonOfType(ctx context.Context, tconn *chrome.TestConn, apnName string, currentState ApnState, isAttach, isDefault bool) error {
	ui := uiauto.New(tconn)

	apnTypeString, err := getAPNTypeString(isAttach, isDefault)
	if err != nil {
		return errors.Wrap(err, "failed to get APN type string")
	}
	apnMoreActionBtn := nodewith.NameContaining(apnName).NameContaining(currentState.String()).NameContaining(apnTypeString).Role(role.Button).HasClass("icon-more-vert").First()

	// More actions button may be temporarily disabled if cellular is connecting or disconnecting.
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(apnMoreActionBtn.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "failed to show more actions button")
	}

	expectedMenuItemBtn := DisableBtn
	if currentState == ApnDisabled {
		expectedMenuItemBtn = EnableBtn
	}

	if err := ui.LeftClickUntil(apnMoreActionBtn, ui.Exists(expectedMenuItemBtn))(ctx); err != nil {
		return errors.Wrap(err, "failed to click more actions button")
	}
	return nil
}

// ClickAPNMoreActionsButton will click the 'More Actions' button associated to the APN.
func ClickAPNMoreActionsButton(ctx context.Context, tconn *chrome.TestConn, userFriendlyAPNName string) error {
	ui := uiauto.New(tconn)

	apnMoreActionBtn := nodewith.NameContaining(userFriendlyAPNName).Role(role.Button).HasClass("icon-more-vert").First()

	// More actions button may be temporarily disabled if cellular is connecting or disconnecting.
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(apnMoreActionBtn.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "failed to show more actions button")
	}

	if err := ui.LeftClickUntil(apnMoreActionBtn, ui.Exists(DetailsBtn))(ctx); err != nil {
		return errors.Wrap(err, "failed to click more actions button")
	}

	return nil
}

// CheckAutomaticallyDetectedAPNDetailesDialog willcheck that the APN details dialog is correct for automatically detected APN.
func CheckAutomaticallyDetectedAPNDetailesDialog(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(NameOfAPNInput)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for name of apn input")
	}

	if err := ui.CheckRestriction(NameOfAPNInput, restriction.Disabled)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify APN name input disabled")
	}

	if err := ui.CheckRestriction(UserNameOfAPNInput, restriction.Disabled)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify APN username input disabled")
	}

	if err := ui.CheckRestriction(PasswordOfAPNInput, restriction.Disabled)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify APN password input disabled")
	}

	if err := ui.LeftClick(APNAdvancedBtn)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on APN advanced settings button")
	}

	if err := ui.CheckRestriction(AuthenticationTypeDropdown, restriction.Disabled)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify Authentication type dropdown disabled")
	}

	if err := ui.WaitUntilExists(IPTypeDropdown)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for name of apn input")
	}

	if err := ui.CheckRestriction(IPTypeDropdown, restriction.Disabled)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify IP type dropdown disabled")
	}

	if err := ui.CheckRestriction(DefaultAPNCheckbox, restriction.Disabled)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify default checkbox disabled")
	}

	if err := ui.CheckRestriction(AttachAPNCheckbox, restriction.Disabled)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify attach checkbox disabled")
	}

	return nil
}

// ExpandPreRevampCellularNetworkDetails expands the "Network" section of a Cellular network's detail page.
func ExpandPreRevampCellularNetworkDetails(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(CellularNetwork.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "failed to show Network button")
	}

	if err := ui.LeftClick(CellularNetwork)(ctx); err != nil {
		return errors.Wrap(err, "failed to expand Network address settings")
	}

	return nil
}

// EnterPreRevampOtherAPNDetails enters the APN, username, password, and whether Attach APN is enabled in the old APN UI.
func EnterPreRevampOtherAPNDetails(ctx context.Context, tconn *chrome.TestConn, apn, username, password string, attach bool) error {
	ui := uiauto.New(tconn)

	if err := ui.WaitUntilExists(AccessPointNameInput)(ctx); err != nil {
		return errors.Wrap(err, "Access point name input doesn't exist")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kb.Close(ctx)

	m, err := input.Mouse(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get mouse")
	}
	defer m.Close(ctx)

	if err := ui.ScrollToVisible(AccessPointNameInput)(ctx); err != nil {
		return errors.Wrap(err, "did not scroll far enough down to apn input")
	}

	if err := ui.DoubleClick(AccessPointNameInput)(ctx); err != nil {
		return errors.Wrap(err, "could not click APN input")
	}

	maxApnLength := 255
	clearTextFieldViaClickingBackspace(kb, maxApnLength)

	if err := ui.LeftClick(AccessPointNameInput)(ctx); err != nil {
		return errors.Wrap(err, "could not click APN input 2")
	}

	if err := kb.Type(ctx, apn); err != nil {
		return errors.Wrap(err, "failed to type apn input")
	}

	if err := ui.ScrollToVisible(UsernameInput)(ctx); err != nil {
		return errors.Wrap(err, "did not scroll far enough down to username input")
	}

	if err := ui.DoubleClick(UsernameInput)(ctx); err != nil {
		return errors.Wrap(err, "could not click username input")
	}

	if err := kb.Type(ctx, username); err != nil {
		return errors.Wrap(err, "failed to type username")
	}

	if err := ui.ScrollToVisible(PasswordInput)(ctx); err != nil {
		return errors.Wrap(err, "did not scroll far enough down to password input")
	}

	if err := ui.DoubleClick(PasswordInput)(ctx); err != nil {
		return errors.Wrap(err, "could not click password input")
	}
	if err := kb.Type(ctx, password); err != nil {
		return errors.Wrap(err, "failed to type password")
	}

	if err := ui.ScrollToVisible(AttachAPNToggle)(ctx); err != nil {
		return errors.Wrap(err, "did not scroll far enough down to attach toggle")
	}

	settings := New(tconn)
	if toggleInfo, err := settings.Info(ctx, AttachAPNToggle); err != nil {
		return errors.Wrap(err, "failed to get toggle button info")
	} else if (toggleInfo.Checked == checked.True && attach == false) || (toggleInfo.Checked == checked.False && attach == true) {
		if ui.LeftClick(AttachAPNToggle)(ctx); err != nil {
			return errors.Wrap(err, "failed to click attach APN toggle")
		}
	}

	if err := ui.ScrollToVisible(SaveButton)(ctx); err != nil {
		return errors.Wrap(err, "did not scroll far enough down to Save button")
	}

	if err := ui.LeftClick(SaveButton.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "failed to click save button")
	}

	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(SaveButton.Focusable())(ctx); err != nil {
		return errors.Wrap(err, "Save button failed to become/remain focusable after modifying custom APN")
	}

	return nil
}

// VerifyPreRevampAPNSelected checks that the |apn| is selected in the pre-revamp APN UI.
func (s *OSSettings) VerifyPreRevampAPNSelected(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, apn string) error {
	if err := ExpandPreRevampCellularNetworkDetails(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to expand APN network details")
	}

	expr := `var node = shadowPiercingQuery('select#selectApn');
		if (node == undefined) {
			throw new Error("APN name not found");
		}
		node.innerText.split('\n')[node.selectedIndex];
		`
	var selectedApn string
	if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, &selectedApn); err != nil {
		return errors.Wrap(err, "failed to find APN dropdown in old UI")
	}

	if selectedApn != apn {
		return errors.Errorf("%q is not selected; %q is selected instead", apn, selectedApn)
	}

	return nil
}

// SelectPreRevampOtherAPN selects the "Other" old APN dropdown.
func SelectPreRevampOtherAPN(ctx context.Context, tconn *chrome.TestConn, apn string) error {
	ui := uiauto.New(tconn)

	if err := ExpandPreRevampCellularNetworkDetails(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to expand APN network details")
	}

	apnMenuItem := nodewith.NameContaining(apn).Role(role.ListBoxOption)

	if err := uiauto.Combine("Select other menu item",
		ui.WaitUntilExists(AccessPointDropdown.Focusable()),
		ui.ScrollToVisible(AccessPointDropdown),
		ui.LeftClick(AccessPointDropdown),
		ui.WaitUntilExists(apnMenuItem),
		ui.LeftClick(apnMenuItem),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to select other menu item")
	}

	return nil
}

// VerifyAPNSubpageNotConnectedApnUI verifies the UI for APNs that are not in use in the revamped APN UI.
func (s *OSSettings) VerifyAPNSubpageNotConnectedApnUI(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, apn string) error {
	expr := `var nodes = shadowPiercingQueryAll(
		'apn-list-item div#labelWrapper');
		var notConnectedAPNs = [];
		nodes.forEach(node => {
			if (!node.innerText.includes("Connected")) {
				notConnectedAPNs.push(node.querySelector('#apnName').innerText)
			}
		})
		if (connectedNode == undefined) {
			throw new Error("No connected APN node found.");
		}
		notConnectedAPNs.join(',');
		`
	var notConnectedAPNs string
	if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, &notConnectedAPNs); err != nil {
		return errors.Wrap(err, "failed to find not connected APN rows")
	}
	if !strings.Contains(notConnectedAPNs, apn) {
		return errors.New("failed to find not connected APN in list")
	}
	return nil
}

// VerifyAPNSubpageConnectedApnUI verifies that the UI of the connected APN's row in the APN subpage is correct.
func (s *OSSettings) VerifyAPNSubpageConnectedApnUI(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, apn, source string) error {
	expr := `var nodes = shadowPiercingQueryAll(
		'apn-list-item div#labelWrapper');
		var connectedNode = undefined;
		nodes.forEach(node => {
			if (node.innerText.includes("Connected")) {
				if (connectedNode === undefined) {
					connectedNode = node
				}
			}
		})
		if (connectedNode == undefined) {
			throw new Error("No connected APN node found.");
		}
		connectedNode.innerText;
		`
	var connectedNodeInnterText string
	if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, &connectedNodeInnterText); err != nil {
		return errors.Wrap(err, "failed to find connected APN row text")
	}

	if !strings.Contains(strings.ToUpper(connectedNodeInnterText), strings.ToUpper(apn)) {
		return errors.Errorf("failed to show APN name %q in connected APN row text; shows %q instead", apn, connectedNodeInnterText)
	}

	// If the APN is automatically detected, it is provided by the modb.
	if (source == "modb" || source == "modem") && !strings.Contains(connectedNodeInnterText, "Automatically detected") {
		return errors.New("failed to show Automatically detected for database provided APN in connected APN row text")
	}

	return nil
}

// VerifyOnlyThisAPNEnabled verifies that only the specified |apn| is enabled.
func VerifyOnlyThisAPNEnabled(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, apn string) error {
	ui := uiauto.New(tconn)

	moreActionsButtonOfAPNFinder := nodewith.NameContaining(apn).NameRegex(regexp.MustCompile("APN is (connected|enabled)")).Role(role.Button).HasClass("icon-more-vert")
	moreOptionsButtons, err := ui.NodesInfo(ctx, moreActionsButtonOfAPNFinder)
	if err != nil {
		return errors.Wrap(err, "failed to find more options button")
	}

	if len(moreOptionsButtons) == 0 {
		return errors.Wrap(err, "failed to find an enabled or connected APN")
	}
	if len(moreOptionsButtons) > 1 {
		return errors.Wrap(err, "failed to find exactly one enabled or connected APN")
	}

	return nil
}

// OpenDiscoverAPNDialogFromAPNSubpage opens the discover new APNs dialog when in the APN subpage
func OpenDiscoverAPNDialogFromAPNSubpage(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	if err := uiauto.Combine("Open discover APNs dialog",
		ui.LeftClick(MoreApnActionsTridot),
		ui.WithTimeout(10*time.Second).WaitUntilExists(DiscoverKnownApnsMenuBtn),
		ui.LeftClick(DiscoverKnownApnsMenuBtn),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open discover APNs dialog")
	}
	return nil
}

// SelectAPNFromDialog selects the provided |apnName| when the discover new APNs dialog is open.
func SelectAPNFromDialog(ctx context.Context, tconn *chrome.TestConn, apnName string) error {
	apnSelection := nodewith.NameContaining(apnName).Role(role.StaticText)

	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(apnSelection)(ctx); err != nil {
		return errors.Wrapf(err, "%q does does not show as a known APN", apnName)
	}

	if err := uiauto.Combine("Add known APN",
		ui.LeftClick(apnSelection),
		ui.LeftClick(UseThisApnBtn),
		ui.WithTimeout(10*time.Second).WaitUntilGone(UseThisApnBtn),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to  known APN %q", apnName)
	}

	if err := ui.WithTimeout(10 * time.Second).WaitUntilGone(nodewith.Name("Automatically detected").Role(role.StaticText))(ctx); err != nil {
		return errors.Wrap(err, "failed to remove automatically detected APN")
	}

	if err := ui.EnsureGoneFor(nodewith.Name("Automatically detected").Role(role.StaticText), 10*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to remove automatically detected APN")
	}

	return nil
}

// VerifyAPNMoreActionsMenuItemsPresent verifies the presence of more actions APN menu items.
func VerifyAPNMoreActionsMenuItemsPresent(ctx context.Context, tconn *chrome.TestConn, userFriendlyAPNName string, hasEnable, hasDisable, hasRemove bool) error {
	ui := uiauto.New(tconn)

	if err := ui.Exists(DetailsBtn)(ctx); err != nil {
		if err := ClickAPNMoreActionsButton(ctx, tconn, userFriendlyAPNName); err != nil {
			return errors.Wrap(err, "failed to click more actions button")
		}
		if err := ui.Exists(DetailsBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to find Details button")
		}
	}

	if hasDisable {
		if err := ui.Exists(DisableBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to find Disable menu item")
		}
	} else {
		if err := ui.Gone(DisableBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to verify lack of Disable menu item")
		}
	}

	if hasEnable {
		if err := ui.Exists(EnableBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to find Enable menu item")
		}
	} else {
		if err := ui.Gone(EnableBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to verify lack of Enable menu item")
		}
	}

	if hasRemove {
		if err := ui.Exists(RemoveBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to find Remove menu item")
		}
	} else {
		if err := ui.Gone(RemoveBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to verify lack of Remove menu item")
		}
	}

	return nil
}

// VerifyApnIsVisibleInSubtext will verify that the APN shows in the subtext of cellular details page.
func (s *OSSettings) VerifyApnIsVisibleInSubtext(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, apn string) error {
	ui := uiauto.New(tconn)

	expr := `var node = shadowPiercingQuery(
		'cr-link-row#apnSubpageButton div#subLabel');
		if (node == undefined) {
			throw new Error("APN name not found");
		}
		node.innerText;
		`
	var sublabelText string
	if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, &sublabelText); err != nil {
		return errors.Wrap(err, "failed to find APN in sublabel")
	}

	if !strings.Contains(strings.ToUpper(sublabelText), strings.ToUpper(apn)) {
		return errors.Errorf("failed to find APN name of %q in sublabel; shows %q instead", apn, sublabelText)
	}

	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(APNSubpageButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find APN in subtext")
	}
	return nil
}

// OpenNewAPNDialogAndPopulateFields opens the new APN dialog and populates its
// fields with |apn| configuration.
func (s *OSSettings) OpenNewAPNDialogAndPopulateFields(ctx context.Context, apn *ApnConfig) error {
	if err := s.ui.LeftClick(MoreApnActionsTridot)(ctx); err != nil {
		return errors.Wrap(err, "failed to click more actions tridot")
	}

	if err := uiauto.Combine("Add custom APN in new APN dialog",
		s.ui.WaitUntilExists(CreateNewApnMenuBtn),
		s.ui.LeftClick(CreateNewApnMenuBtn),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click create custom APN menu button")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kb.Close(ctx)
	if err := uiauto.Combine("Add custom APN in new APN dialog",
		s.ui.WaitUntilExists(NameOfAPNInput),
		kb.TypeAction(apn.Name),
		s.ui.LeftClick(UserNameOfAPNInput),
		kb.TypeAction(apn.Username),
		s.ui.LeftClick(PasswordOfAPNInput),
		kb.TypeAction(apn.Password),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to fill non-advanced field")
	}

	if err := uiauto.Combine("Show advanced settings",
		s.ui.LeftClick(APNAdvancedBtn),
		s.ui.WaitUntilExists(AuthenticationTypeDropdown),
		s.ui.WaitUntilExists(IPTypeDropdown),
		s.ui.WaitUntilExists(DefaultAPNCheckbox),
		s.ui.WaitUntilExists(AttachAPNCheckbox),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to show all advanced fields")
	}

	if len(apn.AuthenticationType) != 0 {
		authenticationTypeMenuItem := nodewith.Name(apn.AuthenticationType).Role(role.ListBoxOption)

		if err := uiauto.Combine("Select authentication menu item",
			s.ui.LeftClick(AuthenticationTypeDropdown),
			s.ui.WaitUntilExists(authenticationTypeMenuItem),
			s.ui.LeftClick(authenticationTypeMenuItem),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to select authentication menu item: %s", apn.AuthenticationType)
		}
	}

	if !apn.IsDefault {
		if err := s.ui.LeftClick(DefaultAPNCheckbox)(ctx); err != nil {
			return errors.Wrap(err, "failed to uncheck default checkbox")
		}
	}

	if apn.IsAttach {
		if err := s.ui.LeftClick(AttachAPNCheckbox)(ctx); err != nil {
			return errors.Wrap(err, "failed to check attach checkbox")
		}
	}

	if len(apn.IPType) != 0 {
		ipTypeMenuItem := nodewith.Name(apn.IPType).Role(role.ListBoxOption)

		if err := uiauto.Combine("Select IP menu item",
			s.ui.LeftClick(IPTypeDropdown),
			s.ui.WaitUntilExists(ipTypeMenuItem),
			s.ui.LeftClick(ipTypeMenuItem),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to select IP menu item: %s", apn.IPType)
		}
	}

	return nil
}

// CreateCustomAPN creates new APN and verify it is shown in the APN list after.
func (s *OSSettings) CreateCustomAPN(ctx context.Context, apn *ApnConfig) error {
	if err := s.OpenNewAPNDialogAndPopulateFields(ctx, apn); err != nil {
		return errors.Wrap(err, "failed to open new APN dialog and populate fields")
	}

	if err := uiauto.Combine("Add and verify APN added",
		s.ui.LeftClick(nodewith.Name("Add").Role(role.Button)),
		s.ui.WaitUntilExists(nodewith.NameContaining(apn.Name).First()),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to add custom APN and verify it shows in the APN list")
	}

	return nil
}

// GetUIStringForIPType returns the UI string that's displayed for the shill IP type
func GetUIStringForIPType(devicePropertyCellularAPNType string) string {
	if devicePropertyCellularAPNType == shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4 {
		return "IPv4"
	}

	if devicePropertyCellularAPNType == shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4v6 {
		return "IPv4/IPv6"
	}

	if devicePropertyCellularAPNType == shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv6 {
		return "IPv6"
	}

	return "Automatic"
}

// GetUIStringForAuthenticationType returns the UI string that's displayed for the shill authentication type
func GetUIStringForAuthenticationType(devicePropertyCellularAPNInfoApnAuthentication string) string {
	if devicePropertyCellularAPNInfoApnAuthentication == shillconst.DevicePropertyCellularAPNInfoApnAuthenticationChap {
		return "CHAP"
	}

	if devicePropertyCellularAPNInfoApnAuthentication == shillconst.DevicePropertyCellularAPNInfoApnAuthenticationPap {
		return "PAP"
	}

	return "Automatic"
}

// VerifyErrorToastMessageIsShowing will verify that the "Can't disable or remove this APN..." toast is showing
func (s *OSSettings) VerifyErrorToastMessageIsShowing(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) error {
	expr := `var node = shadowPiercingQuery(
		'cr-toast#errorToast span#errorToastMessage');
		if (node == undefined) {
			throw new Error("APN name not found");
		}
		node.innerText;
		`

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var errorMessage string
		if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, &errorMessage); err != nil {
			return errors.Wrap(err, "failed to find error message container")
		}

		if !strings.Contains(errorMessage, "Make sure enabled attach APNs are disabled or removed") {
			return testing.PollBreak(errors.Errorf("failed to show error toast; shows '%q' instead", errorMessage))
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: time.Second,
	}); err != nil {
		return err
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
	defer kb.Close(ctx)

	// TODO(b/281904820): Update to expect the |DialogEntryHeader| to exist after SM-DS Support launches.
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(DialogEntryHeader)(ctx); err == nil {
		var manualEntryLink = nodewith.Name("manually").Role(role.StaticText).Linked()
		if err := ui.LeftClick(manualEntryLink)(ctx); err != nil {
			return errors.Wrap(err, "failed to skip SM-DS discovery")
		}
	} else {
		// TODO(b/281904820): Remove the entire else block after SM-DS Support launches.
		var setupNewProfile = nodewith.NameContaining("Set up new profile").Role(role.Button).Focusable()
		// 2 minute is the timeout that we used in production code for loading pending profiles.
		if err := ui.WithTimeout(2 * time.Minute).WaitUntilExists(setupNewProfile)(ctx); err == nil {
			// There are pending profiles, opt to set up a new profile instead.
			if err := ui.LeftClick(setupNewProfile)(ctx); err != nil {
				return errors.Wrap(err, "failed to click set up new profile button")
			}
		}
	}

	var activationCodeInput = nodewith.NameRegex(regexp.MustCompile("Activation code")).Focusable().First()
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(activationCodeInput)(ctx); err != nil {
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
	var testProfileDetailButton = nodewith.HasClass("subpage-arrow").Role(role.Button).Ancestor(managedTestProfile.First())
	if err := ui.WithTimeout(time.Minute).WaitUntilExists(testProfileDetailButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the newly installed test profile")
	}

	if err := ui.LeftClick(testProfileDetailButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to left click Test Profile detail button")
	}
	return nil
}

// VerifyCelluarNetworkExistInList verifies thar the cellular network with |networkName| appears
// in the mobile network list.
func VerifyCelluarNetworkExistInList(ctx context.Context, tconn *chrome.TestConn, networkName string) error {
	if err := WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	network := nodewith.NameContaining(networkName).Role(role.GenericContainer).First()
	if err := ui.WaitUntilExists(network)(ctx); err != nil {
		return errors.Wrapf(err, "failed to find the %s network in the mobile network list", networkName)
	}

	return nil
}

func clearTextFieldViaClickingBackspace(kb *input.KeyboardEventWriter, times int) {
	var keySequence = []string{}
	for i := 1; i <= times; i++ {
		keySequence = append(keySequence, "Backspace")
	}
	kb.TypeSequenceAction(keySequence)
}

// VerifyAPNStabilized verifies that the APN row reflects the |state| consistently
func VerifyAPNStabilized(ctx context.Context, tconn *chrome.TestConn, name string, state ApnState, isAttach, isDefault bool) error {
	apnTypeString, err := getAPNTypeString(isAttach, isDefault)
	if err != nil {
		return errors.Wrap(err, "failed to get APN type string")
	}
	moreActionsButtonOfAPN := nodewith.NameContaining(name).NameContaining(state.String()).NameContaining(apnTypeString).Role(role.Button).HasClass("icon-more-vert").First()
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ui.EnsureExistsFor(moreActionsButtonOfAPN, 2*time.Second)(ctx); err != nil {
			return errors.Wrapf(err, "failed to display APN consistently with name: %s, state: %s, attach: %v, default: %v", name, state, isAttach, isDefault)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed polling for APN more actions button")
	}
	return nil
}

// GoConnectIfNotConnectedThenReturnApnSubpage navigates back from the APN subpage, connects if not connected, then returns to the APN subpage.
func GoConnectIfNotConnectedThenReturnApnSubpage(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	if err := ui.LeftClick(BackArrowBtn)(ctx); err != nil {
		return errors.Wrap(err, "failed to navigate back to mobile data subpage from APN subpage")
	}

	if err := ui.Exists(ConnectButton)(ctx); err == nil {
		if err := uiauto.Combine("Connect to network",
			ui.LeftClick(ConnectButton),
			ui.WithTimeout(10*time.Second).WaitUntilExists(ConnectedStatus),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to connect")
		}
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ui.EnsureExistsFor(ConnectedStatus, 2*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to display connected status consistently")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to stay connected")
	}

	if err := GoToActiveNetworkApnSubpage(ctx, tconn, false /*isFromMobileDataSubpage*/); err != nil {
		return errors.Wrap(err, "failed to go to apn subpage")
	}
	return nil
}
