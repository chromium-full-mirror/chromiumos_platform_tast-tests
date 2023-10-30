// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package instanttether

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/instanttether"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Quicksettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks enabling Instant Tether through Quick Settings",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > System Services > Cross Device > Instant Tethering
		BugComponent: "b:1131910",
		Attr:         []string{"group:cross-device", "cross-device_instanttether", "cross-device_cellular"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "crossdeviceOnboardedAllFeatures",
		Timeout:      3 * time.Minute,
	})
}

func Quicksettings(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(*crossdevice.FixtData).TestConn
	cr := s.FixtValue().(*crossdevice.FixtData).Chrome
	ui := uiauto.New(tconn)
	androidDevice := s.FixtValue().(*crossdevice.FixtData).AndroidDevice

	cleanupCtx := ctx

	ctx, cancel := ctxutil.Shorten(ctx, 2*shill.EnableWaitTime+10*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Open settings as we need to interact with this page throughout the test.
	settings, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, instanttether.TetherURL, func(context.Context) error { return nil })
	if err != nil {
		s.Fatal("Failed to open OS Settings to tethered networks page: ", err)
	}

	// Open Quick Settings network menu.
	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to open Network Quick Settings menu: ", err)
	}

	// Determine the device's name to find it in the Quick Settings panel.
	deviceInfo, err := androidDevice.GetAndroidAttributes(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve information about paired phone")
	}

	networkDetailedView, err := quicksettings.NetworkDetailedView(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get network detailed view: ", err)
	}

	// Model name is provided with "_" instead of spaces, so replace to match the UI label.
	deviceName := strings.Replace(deviceInfo.ModelName, "_", " ", -1)
	mobileNetworkView := nodewith.Role("button").NameRegex(regexp.MustCompile("(?i)connect to .*" + deviceName)).Ancestor(networkDetailedView)

	// Click on the button to connect to the mobile device.
	if err := ui.LeftClick(mobileNetworkView)(ctx); err != nil {
		s.Fatal("Failed to click on the instant tether device in Quick Settings menu: ", err)
	}

	// Accept first-use onboarding dialog on CrOS.
	if err := instanttether.HandleFirstUseDialog(ctx, cr, settings); err != nil {
		s.Fatal("Failed to accept first-use dialog: ", err)
	}

	// Accept the Android prompt to allow tethering, which can appear even if the first-use dialog on CrOS does not.
	if err := androidDevice.AcceptTetherNotification(ctx); err != nil {
		s.Fatal("Failed to accept tethering notification on the phone: ", err)
	}

	// Open Quick Settings network menu.
	if err := quicksettings.NavigateToNetworkDetailedView(ctx, tconn); err != nil {
		s.Fatal("Failed to open Network Quick Settings menu: ", err)
	}

	// Ensure a connection has been established.
	detailsBtn := nodewith.Role("button").NameRegex(regexp.MustCompile("(?i)open settings for .*" + deviceName)).Ancestor(networkDetailedView)

	// Since the test has no exposure to the contents of the NetworkListNetworkItemView, we check that the button's label has
	// changed to indicate that it has intiated a network connection.
	if err := ui.WaitUntilExists(detailsBtn)(ctx); err != nil {
		s.Fatal("Failed to find network detail button confirming Instant Tethering is connected: ", err)
	}

	// Because Quick Settings does not show when the connection is established, and we don't
	// want to wait an arbitrary time, we'll check OS Settings for tethering confirmation.

	// Ensure the CrOS UI updates to reflect the tethered network's status.
	if err := settings.WaitUntilExists(nodewith.NameRegex(regexp.MustCompile(`(?i)instant tethering network, signal strength \d+%`)))(ctx); err != nil {
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

	// Just disconnect from the WiFi network instead of disabling it via shill (like we did for ethernet),
	// since the wifi adapter still needs to be on to use tethering.
	if err := crossdevice.DisconnectFromWifi(ctx); err != nil {
		s.Fatal("Failed to disconnect wifi: ", err)
	}
	defer crossdevice.ConnectToWifi(cleanupCtx)

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
