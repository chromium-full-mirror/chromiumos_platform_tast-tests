// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package instanttether

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/instanttether"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Basic,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks basic Instant Tether functionality",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"hansenmichael@google.com",
		},
		// ChromeOS > Software > System Services > Cross Device > Instant Tethering
		BugComponent: "b:1131910",
		Attr:         []string{"group:cross-device", "cross-device_instanttether", "cross-device_cellular"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				// Test connecting to Instant Tether exclusively through OS settings.
				Val:     false,
				Fixture: "crossdeviceOnboardedAllFeatures",
			},
			{
				// Test connecting to Instant Tether with the "Wi-Fi available via phone"
				// notification that appears when tethering is available and CrOS has no
				// active network connections.
				Name:    "connect_with_notification",
				Fixture: "crossdeviceOnboardedAllFeatures",
				Val:     true,
			},

			// Floss-enabled duplicates
			{
				// Test connecting to Instant Tether exclusively through OS settings.
				Name:      "floss",
				Fixture:   "crossdeviceOnboardedAllFeaturesFloss",
				Val:       false,
				ExtraAttr: []string{"cross-device_floss"},
			},
			{
				// Test connecting to Instant Tether with the "Wi-Fi available via phone"
				// notification that appears when tethering is available and CrOS has no
				// active network connections.
				Name:      "connect_with_notification_floss",
				Fixture:   "crossdeviceOnboardedAllFeaturesFloss",
				Val:       true,
				ExtraAttr: []string{"cross-device_floss"},
			},
		},
		Timeout: 5 * time.Minute,
	})
}

// Basic tests that Instant Tether can be enabled and the Chromebook can use the Android phone's mobile data connection.
// The test flow is slightly complicated due to the ADB-over-WiFi setup required by local tests in the lab.
// The key steps are:
// 1. Setup Instant Tether while all network connections (ethernet and wifi) are active.
//
// 2. Accept first-use dialogs/notifications on CrOS and Android if they appear.
//   - Thanks to 1, we can still use ADB-over-wifi to control the UI on the Android side.
//
// 3. Disconnect other networks to isolate the tethered connection and ensure it works.
//   - The default subtest disconnects ethernet and wifi and proceeds with verification.
//   - The connect_with_notification subtest disconnects ethernet, wifi, and also Instant Tether.
//     This triggers a "Wi-Fi available via phone" notification to appear, which can be used
//     to start the tethered connection with the phone. The subtest accepts that notification
//     to reconnect tethering, and then proceeds with verification.
func Basic(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*crossdevice.FixtData).Chrome
	tconn := s.FixtValue().(*crossdevice.FixtData).TestConn
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice

	// Clear all notifications so we can easily surface the Instant Tether one.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to remove all notifications before starting the test: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*shill.EnableWaitTime+10*time.Second)
	defer cancel()

	tethered := false
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, instanttether.TetherURL, func(context.Context) error { return nil })
	if err != nil {
		s.Fatal("Failed to launch OS settings to the tethered networks page: ", err)
	}

	// Defer disconnecting from the tethered network to clean up.
	disconnectBtn := nodewith.NameRegex(regexp.MustCompile("(?i)disconnect")).Role(role.Button)
	defer func() {
		if tethered {
			if err := settings.LeftClick(disconnectBtn)(cleanupCtx); err != nil {
				s.Log("Failed to click Instant Tether disconnect button: ", err)
			}
		}
	}()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Ensure that the Instant Tethering/Hotspot page is opened.
	if err := settings.WaitUntilExists(nodewith.Role("button").Name("Instant hotspot"))(ctx); err != nil {
		s.Fatal("Failed to find Instant Tethering/Hotspot page: ", err)
	}

	// Initiate Instant Tethering while WiFi is still connected
	// so we still have ADB access to accept the mobile data provisioning notification.
	if err := uiauto.Combine("connecting to the mobile network",
		settings.LeftClick(nodewith.Role(role.Button).NameRegex(regexp.MustCompile("(?i)details")).ClassName("subpage-arrow")),
		settings.LeftClick(nodewith.NameRegex(regexp.MustCompile("(?i)connect")).Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to connect via OS Settings 'Connect' button: ", err)
	}

	// Accept first-use onboarding dialog on CrOS.
	if err := instanttether.HandleFirstUseDialog(ctx, cr, settings); err != nil {
		s.Fatal("Failed to accept first-use dialog: ", err)
	}

	// Accept the Android prompt to allow tethering, which can appear even if the first-use dialog on CrOS does not.
	if err := androidDevice.AcceptTetherNotification(ctx); err != nil {
		s.Fatal("Failed to accept tethering notification on the phone: ", err)
	}

	// Instant Tethering is now active and the Android's data connection is available to CrOS.
	tethered = true

	// Ensure the CrOS UI updates to reflect the tethered network's status.
	if err := settings.WaitUntilExists(nodewith.Name("Disconnect").Role(role.Button))(ctx); err != nil {
		s.Fatal("Failed to find text confirming Instant Tethering is connected: ", err)
	}

	// CrOS is currently connected to ethernet, WiFi, and has access to Android's mobile data.
	// To test that the tethered network can actually be used to access the internet on CrOS,
	// disable all other sources of internet by turning off the ethernet adapter and disconnecting
	// from the available WiFi network. Then tethering will be the only active network source,
	// and we can ensure CrOS still has internet access.
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}
	ethEnableFunc, err := manager.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet)
	if err != nil {
		s.Fatal("Unable to disable ethernet: ", err)
	}
	defer ethEnableFunc(cleanupCtx)

	// If we want to test connecting Instant Tether with the notification that is triggered
	// when no networks are available, disconnect tethering here too so that notification will
	// appear and we can reconnect.
	useNotif := s.Param().(bool)
	if useNotif {
		if err := settings.LeftClick(disconnectBtn)(ctx); err != nil {
			s.Fatal("Failed to click Instant Tether disconnect button: ", err)
		}
	}

	// Just disconnect from the WiFi network instead of disabling it via shill (like we did for ethernet),
	// since the wifi adapter still needs to be on to use tethering.
	if err := crossdevice.DisconnectFromWifi(ctx); err != nil {
		s.Fatal("Failed to disconnect wifi: ", err)
	}
	defer crossdevice.ConnectToWifi(cleanupCtx)

	// At this point, CrOS is no longer connected to ethernet or WiFi.
	// In the default case, it is connected via Instant Tether. If testing
	// the notification, it is currently not connected to anything.
	if useNotif {
		// The notification informing the user that WiFi is available via phone
		// should now appear since there are no active network connections.
		if err := instanttether.ConnectUsingNotification(ctx, tconn); err != nil {
			s.Fatal("Failed to connect with notification: ", err)
		}
	}

	// See if we have internet access over the tethered connection.
	if err := testing.Poll(ctx, func(context.Context) error {
		if err := instanttether.NetworkAvailable(ctx, tconn, cr); err != nil {
			return errors.Wrap(err, "still waiting for network to be available")
		}
		return nil

	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Network not available after connecting with Instant Tethering")
	}
}
