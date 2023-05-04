// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceTrackpointEnabled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify trackpoint row exists when trackpoint enabled",
		Contacts: []string{
			"cros-peripherals@google.com",
			"yyhyyh@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Trackpoint
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalTrackpoint()),
	})
}

// DeviceTrackpointEnabled verifies the trackpoint row exists in device page
// when the chromebook has trackpoint enabled.
func DeviceTrackpointEnabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Open OS Settings app.
	if err := apps.Launch(ctx, tconn, apps.Settings.ID); err != nil {
		s.Fatal("Failed to launch Settings app: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
		s.Fatal("Failed to display settings app in shelf after launch: ", err)
	}

	// Find Device row and click it.
	deviceRow := nodewith.Name("Device").Role(role.Link)
	if err := ui.DoDefault(deviceRow)(ctx); err != nil {
		s.Fatal("Failed to click device row: ", err)
	}

	// Find Trackpoint row and click it.
	trackpointRow := nodewith.Name("TrackPoint").Role(role.GenericContainer)
	if err := ui.DoDefault(trackpointRow)(ctx); err != nil {
		s.Fatal("Failed to click trackpoint row: ", err)
	}

	// Find Built-in trackpoint title.
	trackpointTitle := nodewith.Name("Built-in TrackPoint").Role(role.Heading)
	if err := ui.WaitUntilExists(trackpointTitle)(ctx); err != nil {
		s.Fatal("Failed to verify built-in trackpoint title exist: ", err)
	}
}
