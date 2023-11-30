// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/network/protoutil"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           EthernetIsPreferred,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verify connection preference is for Ethernet when both wired and wireless networks are available",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		Attr:         []string{"group:wificell", "wificell_e2e"},
		ServiceDeps: []string{
			wificell.WifiUIServiceName,
			wificell.OsSettingsServiceName,
			wificell.QuickSettingsServiceName,
			wificell.AutomationServiceName,
			wificell.BrowserChromeServiceName,
			wificell.ShillServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesWithUI),
		Timeout:      3 * time.Minute,
	})
}

// EthernetIsPreferred verifies connection preference is for Ethernet when both
// wired and wireless networks are available.
func EthernetIsPreferred(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	ap, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	cleanupCtx = ctx
	ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()
	defer tf.DeconfigAP(cleanupCtx, ap)

	if _, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap); err != nil {
		s.Fatal("Failed to connect to the AP: ", err)
	}

	if err := checkOSSettings(ctx, rpcClient, ap /* WiFi AP interface */); err != nil {
		s.Fatal(`Failed to verify networks are shown as "Connected" in OS-Settings: `, err)
	}
	if err := checkQuickSettings(ctx, rpcClient, "Ethernet" /* network name */); err != nil {
		s.Fatal(`Failed to verify networks are shown as "Connected" in Quick Settings: `, err)
	}
	if err := checkQuickSettings(ctx, rpcClient, ap.Config().SSID /* network name */); err != nil {
		s.Fatal(`Failed to verify networks are shown as "Connected" in Quick Settings: `, err)
	}

	// Getting the shill properties for later use.
	shillSvc := wifi.NewShillServiceClient(rpcClient.Conn)
	resp, err := shillSvc.GetProperties(ctx, &wifi.GetPropertiesRequest{
		PropertyNames: []string{
			shillconst.ManagerPropertyDefaultService,
			shillconst.ManagerPropertyDefaultTechnology,
		},
	})
	if err != nil {
		s.Fatal("Failed to get shill properties: ", err)
	}

	const expectedServiceName = "Ethernet"
	const expectedDefaultTechnology = "ethernet"

	for name, value := range resp.GetProps() {
		val, err := protoutil.FromShillVal(value)
		if err != nil {
			s.Fatal("Failed to decode shill value: ", err)
		}
		propertyValue, ok := val.(string)
		if !ok {
			s.Fatal("Failed to decode shill value: unexpected property value")
		}

		switch name {
		case shillconst.ManagerPropertyDefaultService:
			servicePath := propertyValue
			// Using the service path of the default service to retrieve and verify the name of this service.
			serviceName, err := getServiceName(ctx, rpcClient, servicePath)
			if err != nil {
				s.Fatal("Failed to get the name of default service: ", err)
			}
			if serviceName != expectedServiceName {
				s.Fatal("Failed to verify Ethernet is preferred: ", err)
			}
		case shillconst.ManagerPropertyDefaultTechnology:
			// Verifying the default technology is the expected one.
			if propertyValue != expectedDefaultTechnology {
				s.Fatalf("Failed to verify Ethernet is preferred: unexpected default technology; got: %q, want: %q", propertyValue, expectedDefaultTechnology)
			}
		default:
			s.Fatalf("Failed to verify Ethernet is preferred: unrecognized property %q", name)
		}
	}
}

// checkOSSettings performs UI verifications, checks Ethernet and the
// specified Wi-Fi network |ap| are shown as "Connected" in the OS-Settings.
func checkOSSettings(ctx context.Context, rpcClient *rpc.Client, ap *wificell.APIface) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
	if _, err := wifiSvc.WifiPageControl(ctx, &wifi.WifiPageControlRequest{
		Ssid:    ap.Config().SSID,
		Control: wifi.WifiPageControlRequest_WaitUntilConnected,
	}); err != nil {
		return errors.Wrap(err, `failed to verify Wi-Fi is shown as "Connected" in the OS-Settings`)
	}

	settingsSvc := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	if _, err := settingsSvc.OpenNetworkDetailPage(ctx, &ossettings.OpenNetworkDetailPageRequest{
		NetworkName: "Ethernet",
		NetworkType: ossettings.OpenNetworkDetailPageRequest_ETHERNET,
	}); err != nil {
		return errors.Wrap(err, "failed to open the detail page of Ethernet")
	}
	defer settingsSvc.Close(cleanupCtx, &emptypb.Empty{})

	// Using JS expression to narrow down the target node to under the
	// OS-Settings without specifying the root UI node of the OS-Settings.
	resp, err := settingsSvc.EvalJSWithShadowPiercer(ctx, &ossettings.EvalJSWithShadowPiercerRequest{
		Expression: `var node = shadowPiercingQuery("#networkState");
		if (node == undefined) {
			throw new Error("network state node not found");
		}
		node.innerText;`,
	})
	if err != nil {
		return errors.Wrap(err, "failed to acquire the connect state of Ethernet")
	}

	val := resp.GetStringValue()
	if !strings.Contains(val, "Connected") || strings.Contains(val, "Not Connected") {
		return errors.Wrapf(err, `failed to verify Ethernet is shown as "Connected"; got: %q, want: "Connected"`, val)
	}
	return nil
}

// checkQuickSettings performs UI verifications, checks the
// specified network is shown as "Connected" in the Quick Settings.
func checkQuickSettings(ctx context.Context, rpcClient *rpc.Client, networkName string) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	quicksettingsSvc := quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn)
	if _, err := quicksettingsSvc.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to launch Quick Settings at network detail view")
	}
	defer quicksettingsSvc.Hide(cleanupCtx, &emptypb.Empty{})

	uiSvc := ui.NewAutomationServiceClient(rpcClient.Conn)
	// The label indicates the connect state, could be "Connected" or "Connected, no internet".
	connectedLabel := ui.Node().Role(ui.Role_ROLE_STATIC_TEXT).HasClass("UnfocusableLabel").NameContaining("Connected")
	networkItem := ui.Node().Role(ui.Role_ROLE_BUTTON).HasClass("NetworkListNetworkItemView").NameContaining(networkName).Finder()
	// Finding the node falls under the specified network.
	target := connectedLabel.Ancestor(networkItem).Finder()

	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: target}); err != nil {
		return errors.Wrap(err, `failed to verify Ethernet is shown as "Connected" in Quick Settings`)
	}
	return nil
}

// getServiceName gets the name of the specified service |servicePath|.
func getServiceName(ctx context.Context, rpcClient *rpc.Client, servicePath string) (string, error) {
	shillSvc := wifi.NewShillServiceClient(rpcClient.Conn)
	resp, err := shillSvc.GetProperties(ctx, &wifi.GetPropertiesRequest{
		ServicePath:   &servicePath,
		PropertyNames: []string{shillconst.ServicePropertyName},
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get properties of service %q", servicePath)
	}

	val, err := protoutil.FromShillVal(resp.GetProps()[shillconst.ServicePropertyName])
	if err != nil {
		return "", errors.Wrap(err, "failed to decode shill value")
	}

	serviceName, ok := val.(string)
	if !ok {
		return "", errors.New("unexpected property value")
	}
	return serviceName, nil
}
