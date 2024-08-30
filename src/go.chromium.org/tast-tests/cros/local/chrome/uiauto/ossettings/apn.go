// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/networkui/netconfig"
	"go.chromium.org/tast/core/ctxutil"
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

// LaunchAtMobileData navigates Settings app to mobile data sub-page.
func LaunchAtMobileData(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*OSSettings, error) {
	return LaunchAtPageURL(ctx, tconn, cr, "networks?type=Cellular", New(tconn).EnsureAtMobileDataPage())
}

// WaitForRefreshCellularProfile waits until the cellular is no longer inhibited and refresh profile completes.
func (s *OSSettings) WaitForRefreshCellularProfile(cr *chrome.Chrome) uiauto.Action {
	return func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		netConn, err := netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
		if err != nil {
			return errors.Wrap(err, "failed to get network Mojo Object")
		}
		defer netConn.Close(cleanupCtx)

		if err := netConn.WaitForCellularDeviceUninhibited(ctx); err != nil {
			return errors.Wrap(err, "failed to get uninhibited cellular device")
		}

		return WaitUntilRefreshCellularProfileCompletes(ctx, s.tconn)
	}
}

// WaitUntilRefreshCellularProfileCompletes will wait until the cellular refresh profile completes
// if there is a text indicating that the page is still loading.
// Note: This function is usually called before interacting with a newly updated cellular network.
func WaitUntilRefreshCellularProfileCompletes(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)
	refreshProfileText := nodewith.NameContaining("This may take a few minutes").Role(role.StaticText)
	// `IfSuccessThen` is required because refreshProfileText won't always show up:
	// If refreshProfileText exists, it indicates that the cellular profile is still loading, so wait until the text is gone.
	// On the other hand, the text doesn't exists indicating the profile is completely loaded, no needs to wait any longer.
	return uiauto.IfSuccessThen(
		ui.WaitUntilExists(refreshProfileText),
		ui.WithTimeout(CellularProfileRefreshTimeout).WaitUntilGone(refreshProfileText),
	)(ctx)
}

// EnsureAtMobileDataPage ensures the settings app stays on mobile data page.
func (s *OSSettings) EnsureAtMobileDataPage() uiauto.Action {
	return s.WaitUntilExists(MobileDataPageHeading)
}

// EnsureAtNetworkDetailsPage ensures the settings app stays on the network details page.
func (s *OSSettings) EnsureAtNetworkDetailsPage(cr *chrome.Chrome) uiauto.Action {
	// The title of network details page follows the name of the network, checking the url of the page instead.
	return func(ctx context.Context) error {
		conn, err := s.ChromeConn(ctx, cr)
		if err != nil {
			return errors.Wrap(err, "failed to get Chrome session")
		}

		if err := conn.Call(ctx, nil, `() => {
			if (location.pathname != "/networkDetail") {
				throw new Error("current page is not network detail page")
			}
		}`); err != nil {
			return errors.Wrap(err, "failed to check the current page")
		}
		return nil
	}
}

// EnsureAtApnPage ensures the settings app stays on the APN page.
func (s *OSSettings) EnsureAtApnPage() uiauto.Action {
	return s.WaitUntilExists(ApnPageRootWebArea)
}

// NavigateToMobileNetworkDetailsPage navigates the settings app to the details page of a mobile network.
// Note that settings app must be on the "Mobile data" page or an error will be thrown.
func (s *OSSettings) NavigateToMobileNetworkDetailsPage(cr *chrome.Chrome, NetworkButtonFinder *nodewith.Finder) uiauto.Action {
	return uiauto.Combine("navigates to network details page",
		// To navigate to the network details page, the settings app must be on the "Mobile data" page.
		s.EnsureAtMobileDataPage(),
		s.WaitForRefreshCellularProfile(cr),
		// An active network could take a longer time to be active.
		s.WithTimeout(WaitForConnectionTimeout).WaitUntilExists(NetworkButtonFinder),
		s.DoDefault(NetworkButtonFinder),
		s.EnsureAtNetworkDetailsPage(cr),
	)
}

// NavigateToApnPage navigates the settings app to the APN page.
// Note that settings app must be on the network details page or an error will be thrown.
func (s *OSSettings) NavigateToApnPage(cr *chrome.Chrome) uiauto.Action {
	return uiauto.Combine("navigates to APN page",
		s.EnsureAtNetworkDetailsPage(cr),
		s.ui.RetryUntil(
			s.DoDefault(ApnSubpageButton),
			s.WithTimeout(3*time.Second).WaitUntilExists(ApnPageRootWebArea),
		),
	)
}

// MaybeConnectToApn clicks the "Connect" button to connect to an APN.
// Note that settings app must be on the network details page or an error will be thrown.
func (s *OSSettings) MaybeConnectToApn(cr *chrome.Chrome) uiauto.Action {
	return uiauto.Combine("clicks connect button if it exists",
		s.EnsureAtNetworkDetailsPage(cr),
		// The action will be skipped if the APN is not available to connect to,
		// such as the APN is already connected, or still connecting.
		uiauto.IfSuccessThen(
			s.WaitUntilExists(ConnectButton),
			s.DoDefault(ConnectButton),
		),
	)
}

// CreateCustomAPN creates new APN and verifies it is shown in the APN list after.
func (s *OSSettings) CreateCustomAPN(ctx context.Context, apn *ApnConfig) error {
	if err := s.EnsureAtApnPage()(ctx); err != nil {
		return errors.Wrap(err, "failed to ensure the current page")
	}

	if err := s.OpenNewAPNDialogAndPopulateFields(ctx, apn); err != nil {
		return errors.Wrap(err, "failed to open new APN dialog and populate fields")
	}

	if err := uiauto.Combine("Add and verify APN added",
		s.DoDefault(nodewith.Name("Add").Role(role.Button)),
		s.ui.WaitUntilExists(nodewith.NameContaining(apn.Name).First()),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to add custom APN and verify it shows in the APN list")
	}

	return nil
}

// VerifyApnConnected verifies that APN is connected through the APN page.
// Note that settings app must be on the APN page or an error will be thrown.
// The source is usually retrieved from `(*cellular.Helper) GetCellularLastGoodAPN`.
// Example: https://source.chromium.org/chromiumos/chromiumos/codesearch/+/32fc89be14d73a593e7be1cfdf79751913dcb631:src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/bundles/cros/cellular/migrate_invalid_apn.go;l=116-127.
func (s *OSSettings) VerifyApnConnected(cr *chrome.Chrome, apnName, source string) uiauto.Action {
	return func(ctx context.Context) error {
		if err := s.EnsureAtApnPage()(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure the page is at APN page")
		}

		const expr = `(()=> {
			let nodes = shadowPiercingQueryAll('apn-list-item div#labelWrapper');
			for (const node of nodes) {
				if (node.innerText.includes("Connected")) {
					return node.innerText;
				}
			}
			throw new Error("No connected APN node found.");
		})()`
		var connectedNodeInnerText string
		if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, &connectedNodeInnerText); err != nil {
			return errors.Wrap(err, "failed to find connected APN row text")
		}

		if !strings.Contains(strings.ToUpper(connectedNodeInnerText), strings.ToUpper(apnName)) {
			return errors.Errorf("failed to show APN name %q in connected APN row text; shows %q instead", apnName, connectedNodeInnerText)
		}

		// The APN will display "Automatically detected" if it is provided by "modb" or "modem".
		if (source == "modb" || source == "modem") && !strings.Contains(connectedNodeInnerText, "Automatically detected") {
			return errors.New("failed to show Automatically detected for database provided APN in connected APN row text")
		}

		return nil
	}
}

// VerifyApnNotConnected verifies the UI for APNs that are not in use in the revamped APN UI.
// Note that settings app must be on the APN page or an error will be thrown.
func (s *OSSettings) VerifyApnNotConnected(cr *chrome.Chrome, apnName string) uiauto.Action {
	return func(ctx context.Context) error {
		if err := s.EnsureAtApnPage()(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure the page is at APN page")
		}

		expr := fmt.Sprintf(`(() => {
			let nodes = shadowPiercingQueryAll(
			'apn-list-item div#labelWrapper');
			for (const node of nodes) {
				if (!node.innerText.includes("Connected")) {
					if (node.querySelector('#apnName').innerText == %q) {
						return
					}
				}
			}
			throw new Error("Not connected APN is not found");
			})()`, apnName)
		if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, nil); err != nil {
			return errors.Wrap(err, "failed to find not connected APN rows")
		}
		return nil
	}
}

// VerifyApnStabilized verifies that the APN row reflects the |apnState| consistently.
// Note that settings app must be on the APN page or an error will be thrown.
func (s *OSSettings) VerifyApnStabilized(apn *ApnConfig, apnState ApnState) uiauto.Action {
	return uiauto.Combine("verify APN is stabilized",
		s.EnsureAtApnPage(),
		s.ui.WithInterval(time.Second).Retry(5, s.ui.EnsureExistsFor(apnMoreActionButtonFinder(apn, apnState).FinalAncestor(WindowFinder), 2*time.Second)),
	)
}

// ClickMoreActionButtonOfAnAPN clicks the more action button of an APN.
func (s *OSSettings) ClickMoreActionButtonOfAnAPN(apn *ApnConfig, apnState ApnState) uiauto.Action {
	moreActionBtn := apnMoreActionButtonFinder(apn, apnState)
	menuItems := map[ApnState]*nodewith.Finder{
		ApnEnabled:   DisableBtn,
		ApnDisabled:  EnableBtn,
		ApnConnected: DisableBtn,
	}

	return uiauto.Combine(fmt.Sprintf("click more action of %q", apn.Name),
		s.EnsureAtApnPage(),
		// More actions button may be temporarily disabled if cellular is connecting or disconnecting.
		s.WithTimeout(WaitForConnectionTimeout).WaitUntilExists(moreActionBtn.Focusable()),
		s.DoDefault(moreActionBtn),
		s.WaitUntilExists(menuItems[apnState]),
	)
}

func apnMoreActionButtonFinder(apn *ApnConfig, apnState ApnState) *nodewith.Finder {
	stateName := fmt.Sprintf(`APN is %s`, regexp.QuoteMeta(strings.ToLower(string(apnState))))
	if apnState == ApnEnabled {
		stateName = fmt.Sprintf(`APN is (%s|%s)`, strings.ToLower(string(ApnEnabled)), strings.ToLower(string(ApnConnected)))
	}

	names := map[ApnType]string{
		ApnIsDefault:               "default",
		ApnIsAttach:                "attach",
		ApnIsDefault | ApnIsAttach: "default and attach",
	}
	typeName := fmt.Sprintf(`APN is type %s`, regexp.QuoteMeta(names[apn.ApnType]))

	// Looking for a button with name like:
	//	1 of 1, internet. APN is automatically detected. APN is connected. APN is type default.
	//
	// In which, "internet" is the name of the APN; "APN is connected" is its state; "APN is type default" is its type; "APN is automatically detected" is displayed if the APN is automatically detected.
	apnNameRegex := regexp.MustCompile(fmt.Sprintf(`^[0-9]+ of [0-9]+, %s\.( APN is automatically detected\.)? %s\. %s\.`, regexp.QuoteMeta(apn.Name), stateName, typeName))
	return nodewith.NameRegex(apnNameRegex).Role(role.Button).HasClass("icon-more-vert")
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
		ui.WaitUntilExists(MobileButton),
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
// Deprecated: Use `(s *OSSettings) NavigateToMobileNetworkDetailsPage` with node `ActiveCellularBtn` instead.
func GoToActiveNetworkDetails(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
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
// Deprecated: Use `(s *OSSettings) NavigateToMobileNetworkDetailsPage` with node `NotActiveCellularBtn.First()` instead.
func GoToFirstInactiveNetworkDetails(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := WaitUntilRefreshCellularProfileCompletes(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to wait until refresh profile complete")
	}

	// TODO(b/343143720): Remove the this action once the issue is resolved.
	// The settings page should go into the network details page after clicking the sub-page arrow button of the network,
	// but sometimes the button is not working. Adding this action to observe where the node is.
	if err := ui.MouseMoveTo(NotActiveCellularBtn.First(), time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to move the mouse to the node")
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
// Deprecated: Use `(s *OSSettings) NavigateToApnPage` instead.
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
	const expr = `(()=> {
			let nodes = shadowPiercingQueryAll('apn-list-item div#labelWrapper');
			for (const node of nodes) {
				if (node.innerText.includes("Connected")) {
					return node.innerText;
				}
			}
			throw new Error("No connected APN node found.");
		})()`
	var connectedNodeInnterText string
	if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, &connectedNodeInnterText); err != nil {
		return errors.Wrap(err, "failed to find connected APN row text")
	}

	if !strings.Contains(strings.ToUpper(connectedNodeInnterText), strings.ToUpper(apn)) {
		return errors.Errorf("failed to show APN name %q in connected APN row text; shows %q instead", apn, connectedNodeInnterText)
	}

	// The APN will display "Automatically detected" if it is provided by "modb" or "modem".
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
	expr := fmt.Sprintf(`(() => {
		let nodes = shadowPiercingQueryAll(
		'apn-list-item div#labelWrapper');
		for (const node of nodes) {
			if (!node.innerText.includes("Connected")) {
				if (node.querySelector('#apnName').innerText == %q) {
					return
				}
			}
		}
		throw new Error("Not connected APN is not found");
		})()`, apn)
	if err := s.EvalJSWithShadowPiercer(ctx, cr, expr, nil); err != nil {
		return errors.Wrap(err, "failed to find not connected APN rows")
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
