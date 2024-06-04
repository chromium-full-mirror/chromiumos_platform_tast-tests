// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           StatusUI,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify Wi-Fi status is correctly represented in the Settings and Quick Settings UI",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		// ChromeOS > Software > System Services > Connectivity > WiFi
		BugComponent: "b:1131912",
		Attr:         []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.AutomationService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
			wifiutil.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      5 * time.Minute,
	})
}

// StatusUI verifies Wi-Fi status is correctly represented in the Settings and Quick Settings UI.
func StatusUI(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	ap, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	cleanupAPCtx := ctx
	defer tf.DeconfigAP(cleanupAPCtx, ap)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	// cleanupCtx is the context with time reserved, used for cleaning up resources other than the AP.
	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	// Toggling the WiFi is one of the criteria of this test,
	// therefore, expecting WiFi to be turned off as preparation of upcoming tests.
	if err := wifiClient.SetWifiEnabled(ctx, false); err != nil {
		s.Fatal("Failed to disable Wi-Fi: ", err)
	}
	// Leaving the test with Wi-Fi enabled since Wi-Fi being enabled is the default "good" state for group:wificell.
	defer wifiClient.SetWifiEnabled(cleanupCtx, true)

	for testPage, test := range map[string]statusUITest{
		"quick settings detailed view": newQuickSettingsViewTest(rpcClient),
		"os settings wifi page":        newOsSettingsWifiPageTest(rpcClient),
		"os settings":                  newOsSettingsPageTest(rpcClient),
	} {
		for _, toggleState := range []bool{true, false} {
			s.Run(ctx, fmt.Sprintf("toggle Wi-Fi to be %t and check UI in %q", toggleState, testPage), func(ctx context.Context, s *testing.State) {
				s.Log("Opening the test page")
				err := test.openPage(ctx)
				if err != nil {
					s.Fatal("Failed to open test page: ", err)
				}

				enabled, err := wifiClient.GetWifiEnabled(ctx)
				if err != nil {
					s.Fatal("Failed to obtain Wi-Fi enabled state: ", err)
				}
				if toggleState != enabled {
					s.Log("Setting Wi-Fi enabled: ", toggleState)
					if err := test.toggleWifi(ctx); err != nil {
						s.Fatal("Failed to set Wi-Fi enabled: ", err)
					}
					if err := waitUntilWifiEnabled(ctx, wifiClient, toggleState); err != nil {
						s.Fatalf("Failed to ensure Wi-Fi enabled state is %t: %v", toggleState, err)
					}
				}

				// The text status and scanning indicator will last for only a few seconds,
				// need to check their existence right after Wi-Fi is toggled.
				if err := test.checkTextAndScanningIndicator(ctx, toggleState); err != nil {
					s.Fatal("Failed to check text and scanning indicator status: ", err)
				}

				cleanupCtx := ctx
				ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
				defer cancel()

				// Cycling the page to force the a11y tree is up to date,
				// especially for statuses that might not be able to update onto UI tree, like: checked status, toggle state etc.
				if err := test.closePage(ctx); err != nil {
					s.Fatal("Failed to close test page: ", err)
				}
				if err = test.openPage(ctx); err != nil {
					s.Fatal("Failed to open test page: ", err)
				}
				defer test.closePage(cleanupCtx)
				defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, fmt.Sprintf("%s_%t_check_ui_dump", testPage, toggleState))

				if err := test.checkToggleAndButtonState(ctx, toggleState); err != nil {
					s.Fatal("Failed to check Wi-Fi toggle and 'Add Wi-Fi' button state: ", err)
				}
				if toggleState {
					if err := test.checkNetworkList(ctx, ap.Config().SSID); err != nil {
						s.Fatal("Failed to check network list exists: ", err)
					}
				}
			})
		}
	}
}

type statusUITest interface {
	// openPage opens the test page which is used to check Wi-Fi status.
	openPage(ctx context.Context) error
	// closePage closes the test page.
	closePage(context.Context) error
	// toggleWifi toggles Wi-Fi from the test page.
	toggleWifi(ctx context.Context) error
	// checkTextAndScanningIndicator checks status text and the scanning indicator are match with the WiFi status.
	checkTextAndScanningIndicator(ctx context.Context, wifiTurnedOn bool) error
	// checkToggleAndButtonState checks the WiFi toggle and "Add Wi-Fi" button state.
	checkToggleAndButtonState(ctx context.Context, wifiTurnedOn bool) error
	// checkNetworkList checks the given ssid is in the network list when Wi-Fi is turned on.
	checkNetworkList(ctx context.Context, ssid string) error
}

type quickSettingsViewTest struct {
	quickSettingsSvc quicksettings.QuickSettingsServiceClient
	uiSvc            ui.AutomationServiceClient
	rootFinder       *ui.NodeHelper
}

func newQuickSettingsViewTest(rpcClient *rpc.Client) *quickSettingsViewTest {
	return &quickSettingsViewTest{
		quickSettingsSvc: quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn),
		uiSvc:            ui.NewAutomationServiceClient(rpcClient.Conn),
		rootFinder:       ui.Node().Ancestor(ui.Node().HasClass("QuickSettingsView").Finder()),
	}
}

func (q *quickSettingsViewTest) openPage(ctx context.Context) error {
	_, err := q.quickSettingsSvc.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{})
	return err
}

func (q *quickSettingsViewTest) closePage(ctx context.Context) error {
	_, err := q.quickSettingsSvc.Hide(ctx, &emptypb.Empty{})
	return err
}

func (q *quickSettingsViewTest) toggleWifi(ctx context.Context) error {
	_, err := q.uiSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: q.wifiToggleButton(any /* wifiToggleState */)})
	return err
}

func (q *quickSettingsViewTest) checkTextAndScanningIndicator(ctx context.Context, wifiTurnedOn bool) error {
	toggleBtn := q.wifiToggleButton(boolToToggleState(wifiTurnedOn))

	if !wifiTurnedOn {
		_, err := q.uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: toggleBtn})
		return err
	}

	req := &ui.WaitUntilExistsRequest{
		Finder: toggleBtn,
		// The following UI results will last for only a few seconds, longer timeout is redundant.
		Timeout: durationpb.New(3 * time.Second),
	}

	if _, err := q.uiSvc.WaitUntilExists(ctx, req); err != nil {
		return errors.Wrap(err, "failed to wait until 'Wi-Fi is turned on' label exists")
	}

	req.Finder = q.rootFinder.Name("Loading").HasClass("ProgressBar").Role(ui.Role_ROLE_PROGRESS_INDICATOR).Finder()
	if _, err := q.uiSvc.WaitUntilExists(ctx, req); err != nil {
		return errors.Wrap(err, "failed to wait until the loading indicator exists")
	}
	return nil
}

func (q *quickSettingsViewTest) checkToggleAndButtonState(ctx context.Context, wifiTurnedOn bool) error {
	if _, err := q.uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
		Finder: q.wifiToggleButton(boolToToggleState(wifiTurnedOn)),
	}); err != nil {
		return errors.Wrapf(err, "failed to wait until Wi-Fi toggle button state to be %t", wifiTurnedOn)
	}

	joinWifiButton := q.rootFinder.Name("Join Wi-Fi network").Role(ui.Role_ROLE_BUTTON).Finder()
	exists, err := wifiutil.StableCheckIsNodeFound(ctx, q.uiSvc, joinWifiButton)
	if err != nil {
		return errors.Wrap(err, `failed to check if "Join Wi-Fi" button exists`)
	}
	if exists != wifiTurnedOn {
		return errors.Errorf(`"Add Wi-Fi" button existing state: %t, expect: %t`, exists, wifiTurnedOn)
	}
	return nil
}

func (q *quickSettingsViewTest) checkNetworkList(ctx context.Context, ssid string) error {
	wifiItem := q.rootFinder.Name("Connect to " + ssid).HasClass("NetworkListNetworkItemView").Role(ui.Role_ROLE_BUTTON).Finder()
	_, err := q.uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: wifiItem})
	return err
}

type wifiToggleState string

const (
	turnedOn  wifiToggleState = "on"
	turnedOff wifiToggleState = "off"
	any       wifiToggleState = "(off|on)"
)

func boolToToggleState(val bool) wifiToggleState {
	if val {
		return turnedOn
	}
	return turnedOff
}

func (q *quickSettingsViewTest) wifiToggleButton(state wifiToggleState) *ui.Finder {
	name := fmt.Sprintf(`^Toggle Wi-Fi. Wi-Fi is turned %s\.$`, string(state))
	networkDetailedViewRevamp := q.rootFinder.HasClass("NetworkDetailedNetworkViewImpl").Finder()
	return ui.Node().HasClass("HoverHighlightView").NameRegex(name).Ancestor(networkDetailedViewRevamp).Finder()
}

type osSettingsWifiPageTest struct {
	settingsSvc ossettings.OsSettingsServiceClient
	uiSvc       ui.AutomationServiceClient
}

func newOsSettingsWifiPageTest(rpcClient *rpc.Client) *osSettingsWifiPageTest {
	return &osSettingsWifiPageTest{
		settingsSvc: ossettings.NewOsSettingsServiceClient(rpcClient.Conn),
		uiSvc:       ui.NewAutomationServiceClient(rpcClient.Conn),
	}
}

func (s *osSettingsWifiPageTest) openPage(ctx context.Context) error {
	_, err := s.settingsSvc.LaunchAtWifiPage(ctx, &emptypb.Empty{})
	return err
}

func (s *osSettingsWifiPageTest) closePage(ctx context.Context) error {
	_, err := s.settingsSvc.Close(ctx, &emptypb.Empty{})
	return err
}

func (s *osSettingsWifiPageTest) toggleWifi(ctx context.Context) error {
	_, err := s.uiSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: ui.Node().Name("Wi-Fi enable").Role(ui.Role_ROLE_TOGGLE_BUTTON).Finder()})
	return err
}

func (s *osSettingsWifiPageTest) checkTextAndScanningIndicator(ctx context.Context, wifiTurnedOn bool) error {
	expectedText := "On"
	if !wifiTurnedOn {
		expectedText = "Off"
	}

	// Checking the text.
	if _, err := s.settingsSvc.EvalJSWithShadowPiercer(ctx, &ossettings.EvalJSWithShadowPiercerRequest{
		Expression: checkTextJSExpr(queryElementJSExpr(".primary-toggle", ""), expectedText),
	}); err != nil {
		return errors.Wrap(err, "failed to check the status label of Wi-Fi")
	}

	// There is no scanning indicator when Wi-Fi is turned off.
	if !wifiTurnedOn {
		return nil
	}

	// Checking the scanning text exists.
	const searchingForNetworks = "Searching for networks…"
	if _, err := s.settingsSvc.EvalJSWithShadowPiercer(ctx, &ossettings.EvalJSWithShadowPiercerRequest{
		Expression: checkTextJSExpr(queryElementJSExpr("container", "#networkListDiv localized-link"), searchingForNetworks),
	}); err != nil {
		return errors.Wrap(err, "failed to check the existence of scanning text")
	}

	// Checking the scanning indicator exists.
	queryScanningSpinner := queryElementJSExpr(fmt.Sprintf("paper-spinner-lite[title=%q]", searchingForNetworks), "")
	expr := fmt.Sprintf(`
		%s;
		window.getComputedStyle(element).display !== "none";
	`, queryScanningSpinner)

	res, err := s.settingsSvc.EvalJSWithShadowPiercer(ctx, &ossettings.EvalJSWithShadowPiercerRequest{Expression: expr})
	if err != nil {
		return errors.Wrap(err, "failed to check if the scanning spinner exists")
	}
	rendered, ok := res.Kind.(*structpb.Value_BoolValue)
	if !ok {
		return errors.New("the response type is not as expected")
	}
	if !rendered.BoolValue {
		return errors.New("the scanning spinner didn't render as expected")
	}
	return nil
}

func (s *osSettingsWifiPageTest) checkToggleAndButtonState(ctx context.Context, wifiTurnedOn bool) error {
	if _, err := s.settingsSvc.WaitUntilToggleOption(ctx, &ossettings.WaitUntilToggleOptionRequest{
		ToggleOptionName: "Wi-Fi enable",
		Enabled:          wifiTurnedOn,
	}); err != nil {
		return errors.Wrap(err, "failed to check if Wi-Fi toggle state is expected")
	}

	addWifiBtn := ui.Node().Name("Add Wi-Fi…").Role(ui.Role_ROLE_BUTTON).Finder()
	exists, err := wifiutil.StableCheckIsNodeFound(ctx, s.uiSvc, addWifiBtn)
	if err != nil {
		return errors.Wrap(err, `failed to check if "Add Wi-Fi" button exists`)
	}
	if exists != wifiTurnedOn {
		return errors.Errorf(`"Add Wi-Fi" button existing state: %t, expect: %t`, exists, wifiTurnedOn)
	}
	return nil
}

func (s *osSettingsWifiPageTest) checkNetworkList(ctx context.Context, ssid string) error {
	nameRegex := fmt.Sprintf(`^Network \d+ of \d+, %s.*`, ssid)
	networkFinder := ui.Node().NameRegex(nameRegex).HasClass("layout").Role(ui.Role_ROLE_GENERIC_CONTAINER).Finder()
	_, err := s.uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: networkFinder})
	return err
}

type osSettingsPageTest struct {
	*osSettingsWifiPageTest
}

func newOsSettingsPageTest(rpcClient *rpc.Client) *osSettingsPageTest {
	return &osSettingsPageTest{newOsSettingsWifiPageTest(rpcClient)}
}

func (s *osSettingsPageTest) openPage(ctx context.Context) error {
	_, err := s.settingsSvc.LaunchAtInternet(ctx, &emptypb.Empty{})
	return err
}

func (s *osSettingsPageTest) checkTextAndScanningIndicator(ctx context.Context, wifiTurnedOn bool) error {
	// There are four known statuses: "Off", "Enabling", "No network" and "Not connected".
	// Expecting the UI shows from "No network" to "Not connected" when turning on the WiFi, and UI shows "Off" when WiFi is turned off.
	expectedTexts := []string{"Off"}
	if wifiTurnedOn {
		expectedTexts = []string{"No network", "Not connected"}
	}

	_, err := s.settingsSvc.EvalJSWithShadowPiercer(ctx, &ossettings.EvalJSWithShadowPiercerRequest{
		Expression: checkTextJSExpr(queryElementJSExpr("networkState", "network-summary-item#WiFi"), expectedTexts...),
	})
	// Check the status text only since the OS Settings page won't have a scanning indicator.
	return err
}

func (s *osSettingsPageTest) checkToggleAndButtonState(ctx context.Context, wifiTurnedOn bool) error {
	// Check sub-page arrow's existence based on Wi-Fi turned on or off.
	subPageArrow := ui.Node().Name("Wi-Fi").HasClass("subpage-arrow").Role(ui.Role_ROLE_BUTTON).Finder()
	exists, err := wifiutil.StableCheckIsNodeFound(ctx, s.uiSvc, subPageArrow)
	if err != nil {
		return errors.Wrap(err, `failed to check if "subpage-arrow" exists`)
	}
	if exists != wifiTurnedOn {
		return errors.Errorf(`"subpage-arrow" existing state: %t, expect: %t`, exists, wifiTurnedOn)
	}

	// Ensure the section is expanded for checking "Add Wi-Fi" button in the same page.
	panel := ui.Node().Name("Add network connection").Role(ui.Role_ROLE_BUTTON)
	if err := wifiutil.EnsureNodeExpanded(ctx, s.uiSvc, panel); err != nil {
		return errors.Wrap(err, `failed to expend the "Add network connection" section`)
	}

	return s.osSettingsWifiPageTest.checkToggleAndButtonState(ctx, wifiTurnedOn)
}

func (*osSettingsPageTest) checkNetworkList(ctx context.Context, ssid string) error {
	// There is no network list in the Settings page.
	return nil
}

func waitUntilWifiEnabled(ctx context.Context, wifiClient *wificell.WifiClient, expectEnabled bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		enabled, err := wifiClient.GetWifiEnabled(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to obtain Wi-Fi enabled state"))
		}
		if enabled != expectEnabled {
			return errors.Errorf("unexpected Wi-Fi enabled state, got %t, expect: %t", enabled, expectEnabled)
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second})
}

// queryElementJSExpr returns a javascript expression to query the target element.
func queryElementJSExpr(element, ancestor string) string {
	query := fmt.Sprintf(`let element = shadowPiercingQuery(%q);`, element)
	if ancestor != "" {
		query = fmt.Sprintf(`
			let ancestor = shadowPiercingQuery(%[1]q);
			if (ancestor == null) { throw new Error("Ancestor is not found"); }
			element = ancestor.shadowRoot.getElementById(%[2]q);
		`, ancestor, element)
	}
	return fmt.Sprintf(`
		%s
		if (element == null) { throw new Error("Element is not found"); }
	`, query)
}

// checkTextJSExpr generates a javascript expression to fetch the status text from the
// queried target element and check if the text is expected.
func checkTextJSExpr(queryElementExpr string, expectedText ...string) string {
	getStatusTextFunc := fmt.Sprintf(`
		function getStatusText() {
			%s
			return element.innerText;
		}
	`, queryElementExpr)

	timeout := 15 * time.Second
	interval := time.Second
	waitTextExpectedFunc := fmt.Sprintf(`
		function waitTextExpected(text) {
			return new Promise((resolve, reject) => {
				(function f(t = 0) {
					if (t > %[1]d) { reject(new Error("timeout")); }
					if (getStatusText() == text) { resolve(); }
					setTimeout(() => { f(t + %[2]d) }, %[2]d);
				})();
			});
		}
	`, timeout.Milliseconds(), interval.Milliseconds())

	awaitTextsExpr := ``
	for _, t := range expectedText {
		awaitTextsExpr += fmt.Sprintf(`
			await waitTextExpected(%[1]q)
			.catch((reject) => {
				throw new Error("failed to check if %[1]s exists: " + reject);
			});
		`, t)
	}

	return fmt.Sprintf(`
		%[1]s
		%[2]s
		(async () => {
			%[3]s
			return true;
		})()
	`, getStatusTextFunc, waitTextExpectedFunc, awaitTextsExpr)
}
