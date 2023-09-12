// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/devicesettings/constants"
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

	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Find Trackpoint row and click it.
	if err := ui.DoDefault(constants.TrackPointRow)(ctx); err != nil {
		s.Fatal("Failed to click trackpoint row: ", err)
	}

	// Find Built-in trackpoint title.
	trackpointTitle := nodewith.Name("Built-in TrackPoint").Role(role.Heading)
	if err := ui.WaitUntilExists(trackpointTitle)(ctx); err != nil {
		s.Fatal("Failed to verify built-in trackpoint title exist: ", err)
	}
}
