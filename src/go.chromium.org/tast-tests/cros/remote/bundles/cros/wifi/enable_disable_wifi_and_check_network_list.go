// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
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
		Func:         EnableDisableWifiAndCheckNetworkList,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that Disable/Enable option works correctly through the Network drop down menu and also WiFi setting page",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"chromeos-connectivity-cienet-external@google.com",
			"cienet-development@googlegroups.com",
			"bossan.fang@cienet.com",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wifiutil.FaillogServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
			"tast.cros.ui.AutomationService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixt",
	})
}

// EnableDisableWifiAndCheckNetworkList verifies that Disable/Enable option works correctly through the Network drop down menu and also WiFi setting page.
func EnableDisableWifiAndCheckNetworkList(ctx context.Context, s *testing.State) {
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
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	// Ensure WiFi is enabled.
	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiClient.SetWifiEnabled(ctx, true); err != nil {
		s.Fatal("Failed to enable WiFi: ", err)
	}
	// Leaving the test with WiFi enabled since WiFi being enabled is the default "good" state for group:wificell.
	defer wifiClient.SetWifiEnabled(cleanupCtx, true)

	trayArea := &quicksettingsUtil{rpcClient: tf.DUTRPC(wificell.DefaultDUT)}
	settings := &ossettingsUtil{rpcClient: tf.DUTRPC(wificell.DefaultDUT)}

	ssidMatched := func(s string) bool { return s == ap.Config().SSID }
	configuredWifiFound := func(items []string) bool { return slices.IndexFunc(items, ssidMatched) != -1 }
	wifiListEmpty := func(items []string) bool { return len(items) == 0 }

	for _, test := range []struct {
		description           string
		toggleWifi            action.Action
		openWifiPage          action.Action
		availableWifiNetworks func(ctx context.Context) ([]string, error)
		verify                func([]string) bool
	}{
		{
			description:           "disable WiFi from quick settings and verify no WiFi network showed up",
			toggleWifi:            trayArea.toggleWifi(false /* expectedState */),
			openWifiPage:          trayArea.openWifiPage,
			availableWifiNetworks: trayArea.availableWifiNetworks,
			verify:                wifiListEmpty,
		}, {
			description:           "enable WiFi from quick settings and verify the configured network appears on the list of networks",
			toggleWifi:            trayArea.toggleWifi(true /* expectedState */),
			openWifiPage:          trayArea.openWifiPage,
			availableWifiNetworks: trayArea.availableWifiNetworks,
			verify:                configuredWifiFound,
		}, {
			description:           "enable WiFi from WiFi page within OS Settings and verify the configured network appears on the list of networks",
			toggleWifi:            settings.toggleWifi(true /* expectedState */),
			openWifiPage:          settings.openWifiPage,
			availableWifiNetworks: settings.availableWifiNetworks,
			verify:                configuredWifiFound,
		}, {
			description:           "disable WiFi from WiFi page within OS Settings and verify no WiFi network showed up",
			toggleWifi:            settings.toggleWifi(false /* expectedState */),
			openWifiPage:          settings.openWifiPage,
			availableWifiNetworks: settings.availableWifiNetworks,
			verify:                wifiListEmpty,
		},
	} {
		s.Run(ctx, test.description, func(ctx context.Context, s *testing.State) {
			if err := test.openWifiPage(ctx); err != nil {
				s.Fatal("Failed to open WiFi page: ", err)
			}

			if err := test.toggleWifi(ctx); err != nil {
				s.Fatal("Failed to toggle WiFi: ", err)
			}

			// Retry on checking the available WiFi networks as it takes a moment for scan and update the UI.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				networkList, err := test.availableWifiNetworks(ctx)
				if err != nil {
					return testing.PollBreak(errors.Wrap(err, "failed to retrieve the available WiFi network list"))
				}

				if !test.verify(networkList) {
					return errors.New("available WiFi network list is not as expected")
				}
				return nil
			}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 5 * time.Second}); err != nil {
				s.Fatal("Failed to verify the available WiFi networks: ", err)
			}
		})
	}
}

type quicksettingsUtil struct{ rpcClient *rpc.Client }

func (q *quicksettingsUtil) toggleWifi(expectedState bool) action.Action {
	return func(ctx context.Context) error {
		svc := quicksettings.NewQuickSettingsServiceClient(q.rpcClient.Conn)
		_, err := svc.ToggleOption(ctx, &quicksettings.ToggleOptionRequest{
			ToggleButton: quicksettings.ToggleOptionRequest_Wifi,
			Enabled:      expectedState,
		})
		return err
	}
}

func (q *quicksettingsUtil) openWifiPage(ctx context.Context) error {
	svc := quicksettings.NewQuickSettingsServiceClient(q.rpcClient.Conn)
	if _, err := svc.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to navigate to the detailed Network within QuickSettings")
	}
	return nil
}

func (q *quicksettingsUtil) availableWifiNetworks(ctx context.Context) ([]string, error) {
	svc := quicksettings.NewQuickSettingsServiceClient(q.rpcClient.Conn)
	resp, err := svc.AvailableWifiNetworks(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve the available WiFi network from Quick Settings")
	}
	return resp.GetSsids(), nil
}

type ossettingsUtil struct{ rpcClient *rpc.Client }

func (o *ossettingsUtil) toggleWifi(expectedState bool) action.Action {
	return func(ctx context.Context) error {
		svc := ossettings.NewOsSettingsServiceClient(o.rpcClient.Conn)
		_, err := svc.SetToggleOption(ctx, &ossettings.SetToggleOptionRequest{
			ToggleOptionName: "Wi-Fi enable",
			Enabled:          expectedState,
		})
		return err
	}
}

func (o *ossettingsUtil) openWifiPage(ctx context.Context) error {
	svc := ossettings.NewOsSettingsServiceClient(o.rpcClient.Conn)
	if _, err := svc.LaunchAtWifiPage(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to navigate to WiFi page within OS Settings")
	}
	return nil
}

func (o *ossettingsUtil) availableWifiNetworks(ctx context.Context) ([]string, error) {
	svc := ossettings.NewOsSettingsServiceClient(o.rpcClient.Conn)
	resp, err := svc.AvailableWifiNetworks(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve the available WiFi network from OS Settings")
	}
	return resp.GetSsids(), nil
}
