// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// CellularProfileRefreshTimeout is the timeout to wait until the profile refresh is done, it could take a longer time.
	CellularProfileRefreshTimeout = 5 * time.Minute

	// WaitForConnectionTimeout is the timeout for waiting the connection to be completed.
	WaitForConnectionTimeout = 90 * time.Second
)

// ApnConfig is struct containing information about an APN.
type ApnConfig struct {
	Name               string
	Username           string
	Password           string
	AuthenticationType string
	IPType             string
	ApnType            ApnType
}

// ApnType specifies the type of data connection for mobile networks.
type ApnType int

// ApnIsDefault and ApnIsAttach indicates the APN type is using.
const (
	ApnIsDefault ApnType = 1 << iota
	ApnIsAttach
)

// CreateCustomAPN creates new APN and verifies it is shown in the APN list after.
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

// WaitUntilRefreshProfileCompletes will wait until the cellular refresh profile completes.
func WaitUntilRefreshProfileCompletes(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	refreshProfileText := nodewith.NameContaining("This may take a few minutes").Role(role.StaticText)
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(refreshProfileText)(ctx); err == nil {
		if err := ui.WithTimeout(CellularProfileRefreshTimeout).WaitUntilGone(refreshProfileText)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait until refresh profile complete")
		}
	}
	return nil
}

// OpenMobileDataSubpage navigates Settings app to mobile data subpage.
// Deprecated: Use `LaunchAtMobileData` instead.
func OpenMobileDataSubpage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*OSSettings, error) {
	ui := uiauto.New(tconn)

	InternetPage, err := LaunchAtPageURL(ctx, tconn, cr, "internet", ui.Exists(Internet))
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch settings page")
	}

	if err := uiauto.Combine("Go to mobile data page",
		ui.LeftClick(Internet),
		ui.LeftClick(MobileButton),
	)(ctx); err != nil {
		mobileDataLinkNode := nodewith.Name("Mobile data").Role(role.Heading)
		if err := InternetPage.NavigateToPageURL(ctx, cr, "networks?type=Cellular", ui.WaitUntilExists(mobileDataLinkNode)); err != nil {
			return nil, errors.Wrap(err, "failed to go to mobile data page")
		}
		return InternetPage, nil
	}
	return &OSSettings{tconn: tconn, ui: ui}, nil
}

// GoToActiveNetworkDetails will go to the network details page of the active cellular network.
// Deprecated: Use `(s *OSSettings) ToMobileNetworkDetailsPage` with node `ActiveCellularBtn` instead.
func GoToActiveNetworkDetails(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := WaitUntilRefreshProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	if err := ui.WithTimeout(WaitForConnectionTimeout).WaitUntilExists(ActiveCellularBtn)(ctx); err != nil {
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
// Deprecated: Use `(s *OSSettings) ToMobileNetworkDetailsPage` with node `NotActiveCellularBtn.First()` instead.
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
// Deprecated: Use `(s *OSSettings) ToApnPage` instead.
func GoToActiveNetworkApnSubpage(ctx context.Context, tconn *chrome.TestConn, isFromMobileDataSubpage bool) error {
	if isFromMobileDataSubpage {
		if err := GoToActiveNetworkDetails(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to go to active cellular network detail page view")
		}
	}

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Go to APN subpage",
		ui.WithTimeout(10*time.Second).WaitUntilExists(ApnSubpageButton.Focusable()),
		ui.DoDefault(ApnSubpageButton.Focusable()),
		ui.WaitUntilExists(nodewith.Name("Settings - Access point name (APN)").Role(role.RootWebArea)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to go to APN subpage")
	}
	return nil
}

// VerifyAPNStabilized verifies that the APN row reflects the |state| consistently.
// Deprecated: Use `(s *OSSettings) VerifyAPNStabilized` instead.
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
// Deprecated: Use `(s *OSSettings) ClickToConnectToApn` instead.
func GoConnectIfNotConnectedThenReturnApnSubpage(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	if err := ui.EnsureExistsFor(nodewith.NameContaining("Manage network APN settings").Role(role.Link), 5*time.Second)(ctx); err == nil {
		if err := ui.LeftClick(BackArrowBtn)(ctx); err != nil {
			return errors.Wrap(err, "failed to navigate back to mobile data subpage from APN subpage")
		}
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

// VerifyAPNSubpageConnectedApnUI verifies that the UI of the connected APN's row in the APN subpage is correct.
// Deprecated: Use `(s *OSSettings) VerifyApnConnected` instead.
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

// ClickAPNMoreActionsButtonOfType will click the 'More Actions' button associated to the APN with the current state |currentState|, and if it is an attach and/or default APN.
// Deprecated: Use `(s *OSSettings) ClickMoreActionButtonOfAnAPN` instead.
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

// VerifyAPNSubpageNotConnectedApnUI verifies the UI for APNs that are not in use in the revamped APN UI.
// Deprecated: Use `(s *OSSettings) VerifyApnNotConnected` instead.
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

// getAPNTypeString returns the type of the APN.
// Deprecated: Use `apnMoreActionButtonFinder` instead.
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
