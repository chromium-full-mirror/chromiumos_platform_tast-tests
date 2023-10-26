// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"net"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IPSettingsView,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that user is able to view the current ip/gateway/subnet mask/MAC address of the device",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wifiutil.FaillogServiceName,
			"tast.cros.ui.AutomationService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.browser.ChromeService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
	})
}

// IPSettingsView verifies that user is able to view the current ip/gateway/subnet mask/MAC address of the device.
func IPSettingsView(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	ap, err := tf.DefaultOpenNetworkAP(ctx)
	if err != nil {
		s.Fatal("Failed to configure AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	if _, err = tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap); err != nil {
		s.Fatal("Failed to connect to WiFi: ", err)
	}
	defer tf.CleanDisconnectDUTFromWifi(ctx, wificell.DefaultDUT)
	ctx, cancel = tf.ReserveForDisconnect(ctx)
	defer cancel()

	settingsSvc := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	if _, err := settingsSvc.OpenNetworkDetailPage(ctx, &ossettings.OpenNetworkDetailPageRequest{
		NetworkName: ap.Config().SSID,
		NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
	}); err != nil {
		s.Fatal("Failed to open network page: ", err)
	}
	defer settingsSvc.Close(cleanupCtx, &emptypb.Empty{})
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, "ui_dump")

	// The root node helper of the settings window.
	var settingsNodeHelper = ui.Node().Ancestor(ui.Node().NameRegex(`^Settings`).Role(ui.Role_ROLE_WINDOW).HasClass("BrowserFrame").Finder())

	// Open the IP settings view.
	uiSvc := ui.NewAutomationServiceClient(rpcClient.Conn)
	if _, err := uiSvc.LeftClick(ctx, &ui.LeftClickRequest{
		Finder: settingsNodeHelper.Name("Show network address settings").Role(ui.Role_ROLE_BUTTON).Finder(),
	}); err != nil {
		s.Fatal("Failed open the IP settings view: ", err)
	}

	// TODO(b/263311524): Update this test to check that the routing prefix (subnet mask),
	// gateway, and MAC address match the values reported by Shill.
	if err := checkIPSettings(ctx, tf, uiSvc, settingsNodeHelper); err != nil {
		s.Fatal("Failed to check the basic IP settings: ", err)
	}
}

func checkIPSettings(ctx context.Context, tf *wificell.TestFixture, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) error {
	for _, section := range []struct {
		name            string
		contentVerifier func(ctx context.Context, tf *wificell.TestFixture, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) error
	}{
		{"IP address", checkIPv4Content(1 /* order */)},
		{"Subnet mask", checkIPContent(2 /* order */)},
		{"Gateway", checkIPContent(3 /* order */)},
		{"MAC address", checkMACAddressContent()},
	} {
		if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
			Finder: root.Name(section.name).Role(ui.Role_ROLE_STATIC_TEXT).Finder(),
		}); err != nil {
			return errors.Wrapf(err, "failed to wait for section to exist: %q", section.name)
		}

		// Make the section is observable and can be captured by screenshot.
		if _, err := uiSvc.MakeVisible(ctx, &ui.MakeVisibleRequest{
			Finder: root.Name(section.name).Role(ui.Role_ROLE_STATIC_TEXT).Finder(),
		}); err != nil {
			return errors.Wrapf(err, "failed to make section visible: %q", section.name)
		}

		if err := section.contentVerifier(ctx, tf, uiSvc, root); err != nil {
			return errors.Wrap(err, "failed to verify IP settings contents")
		}
	}

	return nil
}

// fetchIPContent returns the IP address that UI provided.
// The |order| parameter is provided since all of the IP text boxes are identical on the UI tree
// and are only distinguished by their order.
func fetchIPContent(order int32) func(ctx context.Context, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) (string, error) {
	return func(ctx context.Context, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) (string, error) {
		// This finder uses a regex that will match any IPv4 address, e.g. 127.0.0.1, and is required
		// since the network details page has very little other information to leverage when searching.
		textBox := ui.Node().NameRegex(`^([0-9]{1,3}\.){3}[0-9]{1,3}$`).Role(ui.Role_ROLE_INLINE_TEXT_BOX).Nth(order).Finder()

		resp, err := uiSvc.Info(ctx, &ui.InfoRequest{Finder: textBox})
		if err != nil {
			return "", errors.Wrap(err, "failed to fetch IP content")
		}
		return resp.NodeInfo.Name, nil
	}
}

// checkIPContent checks a IP text content is a valid IP address.
// The |order| parameter is provided since all of the IP text boxes are identical on the UI tree
// and are only distinguished by their order.
func checkIPContent(order int32) func(ctx context.Context, tf *wificell.TestFixture, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) error {
	return func(ctx context.Context, tf *wificell.TestFixture, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) error {
		ip, err := fetchIPContent(order)(ctx, uiSvc, root)
		if err != nil {
			return err
		}

		// Validate the content text.
		if address := net.ParseIP(ip); address == nil {
			return errors.Errorf("failed to to parse content %s", ip)
		}
		return nil
	}
}

// checkIPv4Content checks a IP text content is a valid IP address and is the same IPv4 address reported by router.
// The |order| parameter is provided since all of the IP text boxes are identical on the UI tree
// and are only distinguished by their order.
func checkIPv4Content(order int32) func(ctx context.Context, tf *wificell.TestFixture, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) error {
	return func(ctx context.Context, tf *wificell.TestFixture, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) error {
		ip, err := fetchIPContent(order)(ctx, uiSvc, root)
		if err != nil {
			return err
		}

		ipFromRouter, err := tf.DUTIPv4Addrs(ctx, wificell.DefaultDUT)
		if err != nil {
			return errors.Wrap(err, "failed to get IP address reported by router")
		} else if len(ipFromRouter) == 0 {
			return errors.New("invalid IP address")
		}

		// Validate the IP matches with the one reported by router.
		if ip != ipFromRouter[0].String() {
			return errors.Errorf("IP address shown in UI does not match router, found: %s, expected: %s", ip, ipFromRouter[0].String())
		}
		return nil
	}
}

func checkMACAddressContent() func(ctx context.Context, tf *wificell.TestFixture, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) error {
	return func(ctx context.Context, tf *wificell.TestFixture, uiSvc ui.AutomationServiceClient, root *ui.NodeHelper) error {
		// TODO(b/264833164): Make these fields more identifiable and update this test.
		// The node has no unique identifier and is hard to be located/distinguished from the UI tree.
		// Finding the node directly by its text content since the MAC address has a certain pattern.
		// The expression guarantees at least six groups of two hexadecimal digits; there won't be a collision with the IP address.
		contentFinder := root.NameRegex(`^([0-9A-F]{2}[:-]){5}[0-9A-F]{2}`).Nth(1).Finder()

		resp, err := uiSvc.Info(ctx, &ui.InfoRequest{Finder: contentFinder})
		if err != nil {
			return errors.Wrap(err, "failed to get node info")
		}

		// Validate the content text.
		if _, err = net.ParseMAC(resp.NodeInfo.Name); err != nil {
			return errors.Wrap(err, "failed to parse the MAC address")
		}

		return nil
	}
}
