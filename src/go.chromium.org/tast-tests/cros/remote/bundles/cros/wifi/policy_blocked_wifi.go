// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
)

// QuickSettings indicates that quick setting needs to be used.
const quickSettings int = 0

// OsSettings indicates that OS setting needs to be used.
const osSettings int = 1

// ExpectDialog indicates that join WiFi dialog is expected to be opened during test.
const expectDialog bool = true
const canNotJoinWiFi string = "Failed to join WiFi: "
const canNotAddNewWiFi string = "Failed to add new WiFi network: "
const scanWaitingFailure string = "failed to wait for current scan to be done"
const connectionVerificationFailure string = "failed to verify that WiFi was connected"
const ssidDataRemovalFailure string = "failed to remove entries for SSID"
const tabPressFailure string = "failed to move to next UI element with Tab"

var (
	useDeviceGUID           = "device_network_config"
	notBlockedSSID          = ap.RandomSSID("TAST_TEST_NOT_BLOCKED_")
	blockedSSID             = ap.RandomSSID("TAST_TEST_BLOCKED_")
	blockedPreferredSSID    = ap.RandomSSID("TAST_TEST_BLOCKED_PREFERRED_")
	blockedSSIDHex          = strings.ToUpper(hex.EncodeToString([]byte(blockedSSID)))
	blockedPreferredSSIDHex = strings.ToUpper(hex.EncodeToString([]byte(blockedPreferredSSID)))
	testPass                = "some_password"
	globalNetworkConfig     = &policy.ONCGlobalNetworkConfiguration{
		AllowOnlyPolicyNetworksToAutoconnect: false,
		AllowOnlyPolicyNetworksToConnect:     false,
		BlockedHexSSIDs:                      []string{blockedSSIDHex, blockedPreferredSSIDHex},
	}
)

type policyBlockedWifiTestcase struct {
	devicePolicy *policy.DeviceOpenNetworkConfiguration
	apConfig     security.ConfigFactory
	wifiCred     wifiutil.Credential
}

type localContext struct {
	ctx       context.Context
	rpcClient *grpc.ClientConn
	wifiSvc   *wificell.WifiClient
	wifiCred  wifiutil.Credential
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PolicyBlockedWifi,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that DUT respects rules for SSIDs blocked by device policy",
		Contacts: []string{
			"chromeos-commercial-networking@google.com",
		},
		BugComponent: "b:1000044", // ChromeOS > Software > Commercial (Enterprise) > Commercial Networking
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// verify that managed users can access or are blocked from accessing the appropriate networks as per the configuration.
				// (COM_FOUND_CUJ5_TASK3_WF1).
				Value: "screenplay-55005668-0de3-4314-accc-baef199664ed",
			},
		},
		Attr:         []string{"group:wificell", "wificell_func", "wificell_unstable", "group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		// SigninProfileTestExtensionID is an id of the test extension which is
		// allowed for signin profile (see http://crrev.com/772709 for details).
		VarDeps: []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: append(
			wifiutil.JoinWifiServiceNames,
			"tast.cros.policy.PolicyService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wifi.ShillService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
		),
		Timeout:      10 * time.Minute,
		Fixture:      wificell.FixtureID(wificell.TFFeaturesEnroll),
		Requirements: []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		Params: []testing.Param{
			{
				// TODO(b/278192058): Extend test with  EAP-TLS and WPA-EAP WiFi if they have different behavior when blocked.
				// Verify that DUT can connect to an open AP.
				Name: "open",
				Val: policyBlockedWifiTestcase{
					apConfig: nil,
					wifiCred: &wifiutil.None{},
					devicePolicy: &policy.DeviceOpenNetworkConfiguration{
						Val: &policy.ONC{
							GlobalNetworkConfiguration: globalNetworkConfig,
							NetworkConfigurations: []*policy.ONCNetworkConfiguration{
								{
									GUID: useDeviceGUID,
									Name: "DeviceWideNetworkConfig",
									Type: "WiFi",
									WiFi: &policy.ONCWifi{
										AutoConnect: false,
										Security:    "None",
										SSID:        blockedPreferredSSID,
									},
								},
							},
						},
					},
				},
			},
			{
				// Verify that DUT can connect to a WPA-PSK AP.
				Name: "wpa_psk",
				Val: policyBlockedWifiTestcase{
					apConfig: wpa.NewConfigFactory(
						testPass,
						wpa.Mode(wpa.ModePureWPA2),
						wpa.Ciphers2(wpa.CipherCCMP),
					),
					wifiCred: &wifiutil.Psk{Password: testPass},
					devicePolicy: &policy.DeviceOpenNetworkConfiguration{
						Val: &policy.ONC{
							GlobalNetworkConfiguration: globalNetworkConfig,
							NetworkConfigurations: []*policy.ONCNetworkConfiguration{
								{
									GUID: useDeviceGUID,
									Name: "DeviceWideNetworkConfig",
									Type: "WiFi",
									WiFi: &policy.ONCWifi{
										AutoConnect: false,
										Security:    "WPA-PSK",
										SSID:        blockedPreferredSSID,
										Passphrase:  testPass,
									},
								},
							},
						},
					},
				},
			},
		},
	})
}

/*
PolicyBlockedWifi verifies if the DUT respects blocked WiFi networks from the enterprise provisioned per-device policies.

ONC spec: https://chromium.googlesource.com/chromium/src/+/main/components/onc/docs/onc_spec.md

Here are the full steps this test takes:
 1. Test will be executed for AP with WPA-PSK security and for AP without security.
 2. Start 3 APs, one will be blockedSSID by policy,
    second will be blockedPreferredSSID - blocked by policy and same time added as recommended by policy,
    third one will be notBlockedSSID at all.
 3. Provision the per-device policy to the DUT with BlockedSSIDHex and BlockedPreferredSSIDHex in blocked SSIDs.
 4. Wait for the DUT to connect to the AP provisioned with the device policy and check that before login to
    user's session all 3 AP can be connected using quick settings.
 5. Log in the user account, verify that only notBlockedSSID AP and blockedPreferredSSID can be connected using
    quick settings and OS settings.
    Verify that blockedSSID can not be connected and has special message when you open details view.
 6. Clean up test, including disconnect, remove SSID entry and deconfig APs.
*/
func PolicyBlockedWifi(ctx context.Context, s *testing.State) {
	params := s.Param().(policyBlockedWifiTestcase)
	testFixture := s.FixtValue().(*wificell.TestFixture)
	rpcClient := testFixture.DUTRPC(wificell.DefaultDUT)
	policyClient := ps.NewPolicyServiceClient(rpcClient.Conn)
	wifiSvc := testFixture.DUTWifiClient(wificell.DefaultDUT)
	localCtx := localContext{ctx, rpcClient.Conn, wifiSvc, params.wifiCred}

	// Configure 3 access points for blocked, non blocked, blocked+preferred SSIDs.
	nonBlockedApOptions := []ap.Option{ap.Mode(ap.Mode80211nMixed), ap.Channel(108), ap.HTCaps(ap.HTCapHT20), ap.SpectrumManagement(), ap.SSID(notBlockedSSID)}
	blockedApOptions := []ap.Option{ap.Mode(ap.Mode80211nPure), ap.Channel(1), ap.HTCaps(ap.HTCapHT20), ap.SSID(blockedSSID)}
	blockedPreferredApOptions := []ap.Option{ap.Mode(ap.Mode80211g), ap.Channel(1), ap.SSID(blockedPreferredSSID)}

	deconfigAP := func(ctx context.Context, ap *wificell.APIface) {
		if err := testFixture.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig access point, err: ", err)
		}
	}

	notBlockedAP, err := testFixture.ConfigureAP(ctx, nonBlockedApOptions, params.apConfig)
	if err != nil {
		s.Fatal("Failed to configure access point for non blocked SSID, err: ", err)
	}
	cleanupCtxNotBlockedAP := ctx
	defer deconfigAP(cleanupCtxNotBlockedAP, notBlockedAP)
	ctx, cancelNotBlockedAP := testFixture.ReserveForDeconfigAP(ctx, notBlockedAP)
	defer cancelNotBlockedAP()

	blockedAP, err := testFixture.ConfigureAP(ctx, blockedApOptions, params.apConfig)
	if err != nil {
		s.Fatal("Failed to configure access point for blocked SSID, err: ", err)
	}
	cleanupCtxBlockedAP := ctx
	defer deconfigAP(cleanupCtxBlockedAP, blockedAP)
	ctx, cancelBlockedAP := testFixture.ReserveForDeconfigAP(ctx, blockedAP)
	defer cancelBlockedAP()

	blockedAndPreferredAP, err := testFixture.ConfigureAP(ctx, blockedPreferredApOptions, params.apConfig)
	if err != nil {
		s.Fatal("Failed to configure access point for blocked+preferred SSID, err: ", err)
	}
	cleanupCtxBlockedAndPreferredAP := ctx
	defer deconfigAP(cleanupCtxBlockedAndPreferredAP, blockedAndPreferredAP)
	ctx, cancelBlockedAndPreferredAP := testFixture.ReserveForDeconfigAP(ctx, blockedAndPreferredAP)
	defer cancelBlockedAndPreferredAP()

	// Prepare policy for device.
	pb := policy.NewBlob()
	pb.AddPolicy(params.devicePolicy)
	pJSON, err := json.Marshal(pb)
	if err != nil {
		s.Fatal("Error while marshalling policies to JSON: ", err)
	}

	// Enroll device and apply policies from pJSON.
	// Isolate the step to leverage `defer` pattern.
	func() {
		if _, err := policyClient.EnrollUsingChrome(ctx, &ps.EnrollUsingChromeRequest{
			PolicyJson: pJSON,
			SkipLogin:  true,
		}); err != nil {
			s.Fatal("Failed to enroll using chrome: ", err)
		}
		policyClient.StopChromeAndFakeDMS(ctx, &empty.Empty{})
	}()

	func() {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		chromeService := ui.NewChromeServiceClient(rpcClient.Conn)
		startChromeRequest := func() *ui.NewRequest {
			req := &ui.NewRequest{
				LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
				SigninProfileTestExtensionId: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
				KeepState:                    true,
			}
			return req
		}
		if _, err := chromeService.New(ctx, startChromeRequest()); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer chromeService.Close(cleanupCtx, &emptypb.Empty{})

		ctx, cancel = testFixture.ReserveForDisconnect(ctx)
		defer cancel()

		// ------ Testing join WiFi with one click from the quick settings on the login screen.
		s.Log("Start testing WiFi connection from one click in quick settings before login")

		// Connect to blocked WiFi which is also defined in ONC policy.
		// This WiFi has password/certificate defined in policy, so we should be able
		// to join it without entering any password.
		if err := expectSuccJoinWiFi(blockedAndPreferredAP, localCtx, quickSettings, !expectDialog); err != nil {
			s.Fatal(canNotJoinWiFi, err)
		}

		// Connect and test connection to the notBlockedAP.
		if err := expectSuccJoinWiFi(notBlockedAP, localCtx, quickSettings, expectDialog); err != nil {
			s.Fatal(canNotJoinWiFi, err)
		}

		// Connect and test connection to the blockedAP.
		if err := expectSuccJoinWiFi(blockedAP, localCtx, quickSettings, expectDialog); err != nil {
			s.Fatal(canNotJoinWiFi, err)
		}
		// ------ End testing one click join on Login screen.

		// ------- Test join WiFi from "Add a new WiFi connection" in quick settings on the login screen.
		// For every access point we try to open "Add a new WiFi connection" from the quick
		// settings and manually put the name, security/password and then click Connect.
		s.Log("Start testing add new WiFi connection from the quick settings before login")

		// Connect and test connection to the notBlockedAP.
		if err := expectSuccAddAndJoinWiFiQuickSettings(notBlockedAP, localCtx); err != nil {
			s.Fatal(canNotAddNewWiFi, err)
		}

		// Connect and test connection to the blockedAP.
		if err := expectSuccAddAndJoinWiFiQuickSettings(blockedAP, localCtx); err != nil {
			s.Fatal(canNotAddNewWiFi, err)
		}

		// Connect and test connection to the blockedAndPreferredAP.
		// WiFi popup shows error, previous network is still connected, this is a bug.
		// TODO(b/278189326): Change this to joinAndTestWiFiFromQuickSettings() after bug is fixed.
		if err := expectFailAddAndJoinWiFiQuickSettings(blockedAndPreferredAP, localCtx); err != nil {
			s.Fatal("Failed to join not blocked WiFi from quick settings: ", err)
		}

		// Connect back to blockedAP, so can verify that it is disconnected after the login.
		if err := expectSuccAddAndJoinWiFiQuickSettings(blockedAP, localCtx); err != nil {
			s.Fatal(canNotAddNewWiFi, err)
		}
	}()

	func() {
		// Get chrome service and login to users session with default test user.
		// Policies were applied on device level, so they will be efficient for any logged in user.
		cleanupCtx := ctx
		chromeService := ui.NewChromeServiceClient(rpcClient.Conn)
		if _, err := chromeService.New(ctx, &ui.NewRequest{KeepState: true}); err != nil {
			s.Fatal("DUT: failed to start Chrome: ", err)
		}
		defer chromeService.Close(cleanupCtx, &emptypb.Empty{})

		// Check that blockedAP is disconnected after login.
		if err := expectWiFiNotConnected(ctx, wifiSvc, blockedAP.Config().SSID); err != nil {
			s.Fatal(err, "failed to verify that blocked WiFi was disconnected after login")
		}

		// --------- Testing join WiFi with one click from quick settings after login.
		s.Log("Start testing WiFi connection from one click settings after login")

		// Connect and test connection to the notBlockedAP.
		if err := expectSuccJoinWiFi(notBlockedAP, localCtx, quickSettings, expectDialog); err != nil {
			s.Fatal(canNotJoinWiFi, err)
		}

		// Connect to blocked WiFi which is also defined in ONC policy.
		// This WiFi has password/certificate defined in policy, so we should be able
		// to join this WiFi without entering any password.
		if err := expectSuccJoinWiFi(blockedAndPreferredAP, localCtx, quickSettings, !expectDialog); err != nil {
			s.Fatal(canNotJoinWiFi, err)
		}

		// Can not connect to blockedAP.
		if err := expectFailJoinWiFiFromOneClick(blockedAP, localCtx); err != nil {
			s.Fatal("Was expecting failure for joining WiFi, but received: ", err)
		}
		// --------- End join WiFi with one click from quick settings after login.

		// --------- Testing join WiFi with one click from OS settings after login.
		s.Log("Start testing WiFi connection from OS settings after login with one click")

		// Connect and test connection to the notBlockedAP.
		if err := expectSuccJoinWiFi(notBlockedAP, localCtx, osSettings, expectDialog); err != nil {
			s.Fatal(canNotJoinWiFi, err)
		}

		// Connect and test connection to the blockedAndPreferredAP.
		if err := expectSuccJoinWiFi(blockedAndPreferredAP, localCtx, osSettings, !expectDialog); err != nil {
			s.Fatal(canNotJoinWiFi, err)
		}

		// Connect and test connection to the blockedAP.
		if err := expectBlockedJoinWiFiOSSettings(ctx, rpcClient.Conn, blockedAP); err != nil {
			s.Fatal("Was expecting to find blocked WiFi, but received: ", err)
		}
		//--------- End join WiFi with one click from OS settings after login.

		//------- Test join WiFi from "Add a new WiFi connection" in quick settings after login.
		s.Log("Start testing WiFi connection from quick settings after login")

		// Connect and test connection to the notBlockedAP.
		if err := expectSuccAddAndJoinWiFiQuickSettings(notBlockedAP, localCtx); err != nil {
			s.Fatal("Failed to join not blocked WiFi from quick settings: ", err)
		}

		// Connect and test connection to the blockedAP.
		// WiFi popup expected to show error because network is blocked by policy.
		if err := expectFailAddAndJoinWiFiQuickSettings(blockedAP, localCtx); err != nil {
			s.Fatal("Failed to join not blocked WiFi from quick settings: ", err)
		}

		// Connect and test connection to the blockedAndPreferredAP.
		// WiFi popup shows error, this is a bug.
		// TODO(b/278189326): Change this to joinAndTestWiFiFromQuickSettings() after bug is fixed.
		if err := expectFailAddAndJoinWiFiQuickSettings(blockedAndPreferredAP, localCtx); err != nil {
			s.Fatal("Failed to join not blocked WiFi from quick settings: ", err)
		}
		// --------- End of test join WiFi from quick settings "Add a new WiFi connection" in quick settings after login.
	}()
}

// expectBlockedJoinWiFiOSSettings verifies that provided network is disabled by administrator.
func expectBlockedJoinWiFiOSSettings(ctx context.Context, conn *grpc.ClientConn, expectedAP *wificell.APIface) (retErr error) {
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	uiSvc := ui.NewAutomationServiceClient(conn)
	ssid := expectedAP.Config().SSID
	cleanupCtx := ctx
	cleanup, err := openNetworkSettingPage(ctx, conn, ssid)
	defer cleanup(cleanupCtx)
	if err != nil {
		return errors.Wrap(err, "failed to open network settings page")
	}

	networkStatusText := ui.Node().Name("Connecting to this network is disabled by your administrator").Role(ui.Role_ROLE_STATIC_TEXT).Finder()
	exists, err := wifiutil.StableCheckIsNodeFound(ctx, uiSvc, networkStatusText)
	if err != nil {
		return errors.Wrap(err, "error during finding text about disabled network")
	}
	if exists != true {
		return errors.New("failed to find text about disabled network")
	}

	return nil
}

// expectFailJoinWiFiFromOneClick verifies that tooltip about disable network is shown for blocked networks.
func expectFailJoinWiFiFromOneClick(expectedAP *wificell.APIface, localCtx localContext) (retErr error) {
	uiSvc := ui.NewAutomationServiceClient(localCtx.rpcClient)
	ssid := expectedAP.Config().SSID
	keyboard := inputs.NewKeyboardServiceClient(localCtx.rpcClient)
	ctx := localCtx.ctx
	wifiSvc := localCtx.wifiSvc

	cleanupCtx := ctx
	quickSettingsSvc := quicksettings.NewQuickSettingsServiceClient(localCtx.rpcClient)
	if _, err := quickSettingsSvc.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to navigate to network detailed view within the Quick Settings")
	}
	cleanup := func(ctx context.Context) { quickSettingsSvc.Hide(ctx, &emptypb.Empty{}) }
	defer cleanup(cleanupCtx)

	if _, err := wifiSvc.WaitScanIdle(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, scanWaitingFailure)
	}

	connectToSsidButton := ui.Node().Name(fmt.Sprintf("Connect to %s", ssid)).Role(ui.Role_ROLE_BUTTON).Finder()
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: connectToSsidButton}); err != nil {
		return errors.Wrap(err, "failed to ensure the SSID connection button is present")
	}
	if err := ensureUIElementVisible(ctx, uiSvc, keyboard, connectToSsidButton); err != nil {
		return errors.Wrap(err, "failed select required SSID")
	}

	if _, err := uiSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: connectToSsidButton}); err != nil {
		return errors.Wrap(err, "failed to click the network from detail list")
	}

	disableTooltip := ui.Node().NameContaining("This network is disabled by your administrator").Role(ui.Role_ROLE_TOOLTIP).Finder()
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: disableTooltip}); err != nil {
		return errors.Wrap(err, "error during searching disabled network tooltip")
	}

	if err := expectWiFiNotConnected(ctx, wifiSvc, ssid); err != nil {
		return errors.Wrap(err, "failed to verify that WiFi was connected")
	}
	return nil
}

// expectSuccAddAndJoinWiFiQuickSettings verifies that attempt to join WiFi from quick settings
// "Add WiFi network" was successful and cleans entered data from shill.
func expectSuccAddAndJoinWiFiQuickSettings(accessPoint *wificell.APIface, localCtx localContext) (retErr error) {
	ctx := localCtx.ctx
	wifiSvc := localCtx.wifiSvc

	cleanupCtx := ctx
	ssid := accessPoint.Config().SSID
	cleanup, err := wifiutil.JoinWifiFromQuickSettings(ctx, localCtx.rpcClient, ssid, localCtx.wifiCred)
	defer cleanup(cleanupCtx)
	if err != nil {
		return errors.Wrap(err, "failed open and fill WiFi data from quick settings")
	}

	if err := expectWiFiConnected(ctx, wifiSvc, ssid); err != nil {
		return errors.Wrap(err, connectionVerificationFailure)
	}

	req := &wifi.DeleteEntriesForSSIDRequest{Ssid: []byte(ssid)}
	if _, err := wifiSvc.DeleteEntriesForSSID(ctx, req); err != nil {
		return errors.Wrap(err, ssidDataRemovalFailure)
	}
	return nil
}

// expectFailAddAndJoinWiFiQuickSettings verifies that attempt to join WiFi failed and cleans entered data from shill.
func expectFailAddAndJoinWiFiQuickSettings(accessPoint *wificell.APIface, localCtx localContext) (retErr error) {
	ctx := localCtx.ctx
	wifiSvc := localCtx.wifiSvc

	cleanupCtx := ctx
	ssid := accessPoint.Config().SSID
	cleanup, err := wifiutil.JoinWifiFromQuickSettings(ctx, localCtx.rpcClient, ssid, localCtx.wifiCred)
	defer cleanup(cleanupCtx)
	if err != nil {
		return errors.Wrap(err, "failed open and fill WiFi data from quick settings")
	}

	if err := expectNetworkConfigErrorPrompted(ctx, localCtx.rpcClient); err != nil {
		return errors.Wrap(err, "failed to find the expected network configuration error message")
	}

	if err := expectWiFiNotConnected(ctx, wifiSvc, ssid); err != nil {
		return errors.Wrap(err, "failed to verify that WiFi was not connected")
	}

	req := &wifi.DeleteEntriesForSSIDRequest{Ssid: []byte(ssid)}
	if _, err := wifiSvc.DeleteEntriesForSSID(ctx, req); err != nil {
		return errors.Wrap(err, ssidDataRemovalFailure)
	}
	return nil
}

// expectNetworkConfigErrorPrompted verifies a message "Error configuring network" on the "Join WiFi Network" dialog.
func expectNetworkConfigErrorPrompted(ctx context.Context, conn *grpc.ClientConn) error {
	uiSvc := ui.NewAutomationServiceClient(conn)
	networkConfigErrorMsg := ui.Node().Name("Error configuring network").Role(ui.Role_ROLE_INLINE_TEXT_BOX).Ancestor(wifiutil.JoinWiFiNetworkDialogFinder).Finder()
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: networkConfigErrorMsg}); err != nil {
		return errors.Wrap(err, "failed to find the expected error message")
	}
	return nil
}

// expectWiFiConnected waits the specified network is connected.
func expectWiFiConnected(ctx context.Context, wifiSvc *wificell.WifiClient, ssid string) error {
	return wifiSvc.WaitForConnected(ctx, ssid, true /* expectedValue */)
}

// expectWiFiNotConnected waits the specified network is not connected.
func expectWiFiNotConnected(ctx context.Context, wifiSvc *wificell.WifiClient, ssid string) error {
	return wifiSvc.WaitForConnected(ctx, ssid, false /* expectedValue */)
}

// openNetworkSettingPage will open specific SSID page from the OS settings.
func openNetworkSettingPage(ctx context.Context, conn *grpc.ClientConn, ssid string) (func(context.Context), error) {
	osSettingsSvc := ossettings.NewOsSettingsServiceClient(conn)
	cleanup := func(ctx context.Context) { osSettingsSvc.Close(ctx, &emptypb.Empty{}) }

	uiSvc := ui.NewAutomationServiceClient(conn)
	// SSIDs positon might change due to signal strengths, so will re-try from
	// here if the page header is not correct.
	for i := 1; i <= 3; i++ {
		if _, err := osSettingsSvc.OpenNetworkDetailPage(ctx, &ossettings.OpenNetworkDetailPageRequest{
			NetworkName: ssid,
			NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
		}); err != nil {
			return cleanup, errors.Wrap(err, "error during opening networks setting page")
		}
		// Check that page header has required SSID.
		pageHeaderName := ui.Node().Name(ssid).Role(ui.Role_ROLE_STATIC_TEXT).Finder()
		correctPage, err := wifiutil.StableCheckIsNodeFound(ctx, uiSvc, pageHeaderName)
		if err != nil {
			return cleanup, errors.Wrap(err, "error during searching page header on network setting page")
		}
		if correctPage == true {
			return cleanup, nil
		}
	}
	return cleanup, errors.New("Expected page header was not found " + ssid)
}

// expectSuccJoinWiFi will joind WiFi, check that WiFi is connected and clean data afterwards.
func expectSuccJoinWiFi(expectedAP *wificell.APIface, localCtx localContext, settingType int, isCredentialsRequired bool) (retErr error) {
	ctx := localCtx.ctx
	wifiSvc := localCtx.wifiSvc

	if _, err := wifiSvc.WaitScanIdle(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, scanWaitingFailure)
	}

	cleanupCtx := ctx
	ssid := expectedAP.Config().SSID
	cleanup, err := joinWiFiWithOneClick(localCtx, ssid, testPass, settingType, isCredentialsRequired)
	defer cleanup(cleanupCtx)
	if err != nil {
		return errors.Wrap(err, "failed open and fill WiFi data from one click")
	}

	if err := expectWiFiConnected(ctx, wifiSvc, ssid); err != nil {
		return errors.Wrap(err, connectionVerificationFailure)
	}

	// Clean entered data/passwords for WiFi, except it is blockedPreferredSSID,
	// because cleaning data for blockedPreferredSSID will clean shill credentials
	// received from the ONC policy.
	if ssid != blockedPreferredSSID {
		req := &wifi.DeleteEntriesForSSIDRequest{Ssid: []byte(ssid)}
		if _, err := wifiSvc.DeleteEntriesForSSID(ctx, req); err != nil {
			return errors.Wrap(err, ssidDataRemovalFailure)
		}
	}
	return nil
}

// joinWiFiWithOneClick will joind WiFi network from quick settings or OS settings with one click.
func joinWiFiWithOneClick(localCtx localContext, ssid, password string, settingType int, isCredentialsRequired bool) (func(context.Context), error) {
	ctx := localCtx.ctx
	rpcClient := localCtx.rpcClient

	cleanup, err := openJoinWiFiDialogFromOneClick(ctx, rpcClient, settingType, ssid)
	if err != nil {
		faillog := ui.NewChromeUIServiceClient(rpcClient)
		faillog.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{
			FilePrefix: "network_details_page",
		})
		return cleanup, errors.Wrap(err, `failed to open "Join Wi-Fi network" dialog"`)
	}

	if _, ok := localCtx.wifiCred.(*wifiutil.None); ok || !isCredentialsRequired {
		return cleanup, nil
	}
	uiauto := ui.NewAutomationServiceClient(rpcClient)
	if _, err := uiauto.LeftClick(ctx, &ui.LeftClickRequest{Finder: wifiutil.JoinWiFiNetworkDialogFinder}); err != nil {
		return cleanup, errors.Wrap(err, "failed to click the join button")
	}

	if err := wifiutil.CompleteJoinWiFiDialog(ctx, rpcClient, ssid, localCtx.wifiCred, true /*skipSecurityComboBox*/); err != nil {
		return cleanup, errors.Wrap(err, "failed to join Wi-Fi")
	}

	return cleanup, nil
}

// openJoinWiFiDialogFromOneClick will select specific SSID from the WiFi list in
// OS settings or in Quick settings and press enter on it, which should lead
// to "join WiFi" dialog window.
func openJoinWiFiDialogFromOneClick(ctx context.Context, rpcClient *grpc.ClientConn, settings int, ssid string) (func(context.Context), error) {
	keyboard := inputs.NewKeyboardServiceClient(rpcClient)
	uiSvc := ui.NewAutomationServiceClient(rpcClient)

	var cleanup func(context.Context)
	var connectToSsidButton *ui.Finder

	if settings == quickSettings {
		settingsSvc := quicksettings.NewQuickSettingsServiceClient(rpcClient)
		if _, err := settingsSvc.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
			return func(ctx context.Context) { return }, errors.Wrap(err, "failed to navigate to network detailed view within the Quick Settings")
		}
		cleanup = func(ctx context.Context) { settingsSvc.Hide(ctx, &emptypb.Empty{}) }

		connectToSsidButton = ui.Node().Name(fmt.Sprintf("Connect to %s", ssid)).Role(ui.Role_ROLE_BUTTON).Finder()
	} else if settings == osSettings {
		settingsSvc := ossettings.NewOsSettingsServiceClient(rpcClient)
		if _, err := settingsSvc.LaunchAtWifiPage(ctx, &emptypb.Empty{}); err != nil {
			return func(ctx context.Context) { return }, errors.Wrap(err, "error during opening OS WiFi setting page")
		}
		cleanup = func(ctx context.Context) { settingsSvc.Close(ctx, &emptypb.Empty{}) }
		connectToSsidButton = &ui.Finder{
			NodeWiths: []*ui.NodeWith{
				{Value: &ui.NodeWith_NameContaining{NameContaining: fmt.Sprintf(`, %s,`, ssid)}},
				{Value: &ui.NodeWith_First{First: true}},
			},
		}
	}

	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: connectToSsidButton}); err != nil {
		return cleanup, errors.Wrap(err, "failed to ensure the SSID connection button is present")
	}

	if err := ensureUIElementVisible(ctx, uiSvc, keyboard, connectToSsidButton); err != nil {
		return cleanup, errors.Wrap(err, "failed select required SSID")
	}

	if _, err := uiSvc.EnsureFocused(ctx, &ui.EnsureFocusedRequest{Finder: connectToSsidButton}); err != nil {
		return cleanup, errors.Wrap(err, "required SSID was not found")
	}

	if _, err := keyboard.Accel(ctx, &inputs.AccelRequest{Key: "enter"}); err != nil {
		return cleanup, errors.Wrap(err, "failed to select required SSID")
	}

	return cleanup, nil
}

// ensureUIElementVisible moves UI element which is offscreen to visible by pressing
// Tab until UI element is visible. It is added for more stable tests in noisy environment
// when list of WiFis is very long.
func ensureUIElementVisible(ctx context.Context, uiSvc ui.AutomationServiceClient,
	keyboard inputs.KeyboardServiceClient, uiElement *ui.Finder) error {

	// Focus on current window/tab by pressing Tab once.
	if _, err := keyboard.Accel(ctx, &inputs.AccelRequest{Key: "tab"}); err != nil {
		return errors.Wrap(err, tabPressFailure)
	}
	isSsidFocusable, err := isUIElementVisible(ctx, uiSvc, uiElement)
	if err != nil {
		return errors.Wrap(err, "UI element can not be seen on screen")
	}

	// SSID is not visible on screen, click Tab until it become visible.
	if !isSsidFocusable {
		// Move from the top of menu to the WiFi list.
		for i := 1; i <= 6; i++ {
			if _, err := keyboard.Accel(ctx, &inputs.AccelRequest{Key: "tab"}); err != nil {
				return errors.Wrap(err, tabPressFailure)
			}
		}
		maxAttempForTabPress := 30
		for i := 1; i <= maxAttempForTabPress; i++ {
			isSsidFocusable, err := isUIElementVisible(ctx, uiSvc, uiElement)
			if err != nil {
				return errors.Wrap(err, "UI element can not be focused")
			}
			if isSsidFocusable {
				break
			} else {
				if _, err := keyboard.Accel(ctx, &inputs.AccelRequest{Key: "tab"}); err != nil {
					return errors.Wrap(err, tabPressFailure)
				}
			}
		}
	}

	return nil
}

// isUIElementVisible checks if UI element has state "offscreen" in their NodeInfo.
func isUIElementVisible(ctx context.Context, uiSvc ui.AutomationServiceClient, uiElement *ui.Finder) (bool, error) {
	uiElementInfo, err := wifiutil.StableNodeInfo(ctx, uiSvc, uiElement)
	if err != nil {
		return false, errors.Wrap(err, "failed to check UI element information")
	}
	val, isSsidOffScreen := uiElementInfo.NodeInfo.State["offscreen"]
	if isSsidOffScreen && val {
		return false, nil
	} else if isSsidOffScreen && !val {
		return false, errors.Errorf("the current UI element offscreen: %s, got offscreen: %t, expect offscreen: %t", uiElementInfo.NodeInfo.Restriction, isSsidOffScreen, true)
	} else {
		return true, nil
	}
}
