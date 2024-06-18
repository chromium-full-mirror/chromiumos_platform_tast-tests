// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type networkScanParam struct {
	test  func(conn *grpc.ClientConn, req *ui.NewRequest) networkScanTest
	uiReq *ui.NewRequest
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkScan,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the WiFi UIs will scan for networks periodically",
		Contacts: []string{
			//"cros-connectivity@google.com",
			//"chromeos-connectivity-engprod@google.com",
			"vivian.chen@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Attr:           []string{"group:wificell", "wificell_e2e_unstable"},
		TestBedDeps:    []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking, tbdep.BluetoothStateNormal},
		Vars:           []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wifiutil.FaillogServiceName,
			wificell.BrowserChromeServiceName,
			wificell.AutomationServiceName,
			wificell.QuickSettingsServiceName,
			wificell.OsSettingsServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Params: []testing.Param{
			{
				Name: "login",
				Val: networkScanParam{
					test:  newUserLoginTest,
					uiReq: &ui.NewRequest{},
				},
			}, {
				Name: "oobe",
				Val: networkScanParam{
					test: newOobeLoginTest,
					uiReq: &ui.NewRequest{
						LoginMode: ui.LoginMode_LOGIN_MODE_NO_LOGIN,
					},
				},
			},
		},
	})
}

// NetworkScan verifies the network panel will always be populated with the available networks
// since it triggers a network scan every 5 seconds.
func NetworkScan(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	param := s.Param().(networkScanParam)

	if param.uiReq.LoginMode == ui.LoginMode_LOGIN_MODE_NO_LOGIN {
		param.uiReq.SigninProfileTestExtensionId = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	}
	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := cr.New(ctx, param.uiReq); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	// Configuring 1 AP to simulate the presence of Wi-Fi network within range.
	ap, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	cleanupCtx = ctx
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()
	defer tf.DeconfigAP(cleanupCtx, ap)

	// Finding the AP before opening the UI since the target AP's presence is a part of criteria of scanning validation and we want the target AP to be presented at the beginning.
	props := map[string]interface{}{
		shillconst.ServicePropertyType: shillconst.TypeWifi,
		shillconst.ServicePropertyName: ap.Config().SSID,
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err = tf.WifiClient().GetServicePath(ctx, props)
		return err
	}, &testing.PollOptions{Timeout: shillconst.DefaultTimeout, Interval: time.Second}); err != nil {
		s.Fatalf("Failed to find wifi service %q: %v", ap.Config().SSID, err)
	}

	test := param.test(rpcClient.Conn, param.uiReq)
	if err := test.openUI(ctx); err != nil {
		s.Fatal("Failed to open page: ", err)
	}
	defer test.closeUI(cleanupCtx)
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, "ui_dump")

	const (
		// Custom timeout watchdog for testing.Poll, to continuously monitor for at least a full minute.
		checkingDuration = time.Minute

		// Scans typically take 2 to 3 seconds.
		scanDuration      = 3 * time.Second
		scanInterval      = 5 * time.Second
		expectedScanCount = int(checkingDuration / (scanDuration + scanInterval))
	)

	scanCount := 0
	start := time.Now()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if time.Since(start) >= checkingDuration {
			return nil
		}
		if err := test.isScanning(ctx); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to ensure the searching networks text is presented"))
		}
		s.Log("UI is scanning")

		if err := test.hasScanningFinished(ctx); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to ensure the network scanning is finished"))
		}
		scanCount++
		s.Logf("UI has finished scanning (%d/%d)", scanCount, expectedScanCount)

		if err := test.isNetworkListPopulated(ctx, ap.Config().SSID); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to ensure the network is presented"))
		}
		s.Log("The network list is populated")

		return errors.Errorf("still observing for the WiFi UI for %.1fs", (checkingDuration - time.Since(start)).Seconds())
	}, nil); err != nil {
		s.Fatal("Failed to observe the WiFi UI: ", err)
	}

	if scanCount < expectedScanCount {
		s.Errorf("Failed to verify WiFi scans are executed properly: expecting at least %d scans within %v; got %d scans", expectedScanCount, checkingDuration, scanCount)
	}
}

type networkScanTest interface {
	// openUI opens the network settings page.
	openUI(ctx context.Context) error
	// closeUI closes the network settings page.
	closeUI(ctx context.Context) error
	// isScanning checks if the network is scanning in the the WiFi UI.
	isScanning(ctx context.Context) error
	// hasScanningFinished checks if the network scanning is finished in the the WiFi UI.
	hasScanningFinished(ctx context.Context) error
	// isNetworkListPopulated checks if the network list is populated with available networks.
	isNetworkListPopulated(ctx context.Context, expectedNetwork string) error
}

type oobeLoginTest struct {
	uiSvc            ui.AutomationServiceClient
	quickSettingsSvc quicksettings.QuickSettingsServiceClient
}

func newOobeLoginTest(conn *grpc.ClientConn, req *ui.NewRequest) networkScanTest {
	return &oobeLoginTest{
		uiSvc:            ui.NewAutomationServiceClient(conn),
		quickSettingsSvc: quicksettings.NewQuickSettingsServiceClient(conn),
	}
}

func (o *oobeLoginTest) openUI(ctx context.Context) error {
	if _, err := o.quickSettingsSvc.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to navigate to network detailed view within the Quick Settings")
	}
	return nil
}

func (o *oobeLoginTest) closeUI(ctx context.Context) error {
	_, err := o.quickSettingsSvc.Hide(ctx, &emptypb.Empty{})
	return err
}

func (o *oobeLoginTest) progressBar() *ui.Finder {
	quickSettingsView := ui.Node().HasClass("QuickSettingsView").Finder()
	progressBar := ui.Node().Ancestor(quickSettingsView).Name("Loading").HasClass("ProgressBar").Role(ui.Role_ROLE_PROGRESS_INDICATOR).Finder()
	return progressBar
}

func (o *oobeLoginTest) isScanning(ctx context.Context) error {
	_, err := o.uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: o.progressBar()})
	return err
}

func (o *oobeLoginTest) hasScanningFinished(ctx context.Context) error {
	_, err := o.uiSvc.WaitUntilGone(ctx, &ui.WaitUntilGoneRequest{Finder: o.progressBar()})
	return err
}

func (o *oobeLoginTest) isNetworkListPopulated(ctx context.Context, expectedNetworks string) error {
	networkList, err := o.quickSettingsSvc.AvailableWifiNetworks(ctx, &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to retrieve the available WiFi network from Quick Settings")
	}
	ssids := networkList.GetSsids()
	if len(ssids) == 0 {
		return errors.New("no available WiFi network are listed in the network list")
	}
	if !slices.Contains(ssids, expectedNetworks) {
		return errors.Errorf("the network %q should be listed in the network list", expectedNetworks)
	}
	return nil
}

type userLoginTest struct {
	uiSvc         ui.AutomationServiceClient
	ossettingsSvc ossettings.OsSettingsServiceClient
}

func newUserLoginTest(conn *grpc.ClientConn, req *ui.NewRequest) networkScanTest {
	return &userLoginTest{
		uiSvc:         ui.NewAutomationServiceClient(conn),
		ossettingsSvc: ossettings.NewOsSettingsServiceClient(conn),
	}
}

func (u *userLoginTest) openUI(ctx context.Context) error {
	if _, err := u.ossettingsSvc.LaunchAtWifiPage(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to launch OS-settings at WiFi page")
	}
	return nil
}

func (u *userLoginTest) closeUI(ctx context.Context) error {
	_, err := u.ossettingsSvc.Close(ctx, &emptypb.Empty{})
	return err
}

func (u *userLoginTest) searchingNetworkText() *ui.Finder {
	settingsWindow := ui.Node().Name("Settings - Wi-Fi").Role(ui.Role_ROLE_WINDOW).Nth(0).Finder()
	searchingNetworksText := ui.Node().Ancestor(settingsWindow).Name("Searching for networks…").Role(ui.Role_ROLE_GENERIC_CONTAINER).Finder()
	return searchingNetworksText
}

func (u *userLoginTest) isScanning(ctx context.Context) error {
	_, err := u.uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: u.searchingNetworkText()})
	return err
}

func (u *userLoginTest) hasScanningFinished(ctx context.Context) error {
	_, err := u.uiSvc.WaitUntilGone(ctx, &ui.WaitUntilGoneRequest{Finder: u.searchingNetworkText()})
	return err
}

func (u *userLoginTest) isNetworkListPopulated(ctx context.Context, expectedNetwork string) error {
	networkList, err := u.ossettingsSvc.AvailableWifiNetworks(ctx, &emptypb.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to retrieve the available WiFi network from OS Settings")
	}
	ssids := networkList.GetSsids()
	if len(ssids) == 0 {
		return errors.New("no available WiFi network are listed in the network list")
	}
	if !slices.Contains(ssids, expectedNetwork) {
		return errors.Errorf("the network %q should be listed in the network list", expectedNetwork)
	}
	return nil
}
