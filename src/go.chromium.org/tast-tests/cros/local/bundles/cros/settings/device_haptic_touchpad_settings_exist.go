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
		Func:         DeviceHapticTouchpadSettingsExist,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test to verify haptic touchpad settings exist",
		Contacts: []string{
			"cros-peripherals@google.com",
			"wangdanny@google.com",
			"dpad@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Touchpad
		BugComponent: "b:1131849",
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.HapticTouchpad()),
		Timeout:      time.Minute,
	})
}

// DeviceHapticTouchpadSettingsExist tests if haptic touchpad settings show up
// when a haptic touchpad is connected.
func DeviceHapticTouchpadSettingsExist(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

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

	// Find Touchpad row and click it.
	if err := ui.DoDefault(constants.TouchpadRow)(ctx); err != nil {
		s.Fatal("Failed to click touchpad row: ", err)
	}

	// Verify if haptic settings show up.
	touchpadSlider := nodewith.NameContaining("Click strength").Role(role.Slider)
	hapticFeedbackText := nodewith.Name("Haptic feedback").Role(role.StaticText)
	if err := uiauto.Combine("Verify if haptic touchpad settings show up",
		ui.WaitUntilExists(touchpadSlider),
		ui.WaitUntilExists(hapticFeedbackText),
	)(ctx); err != nil {
		s.Fatal("Failed to verify haptic touchpad settings show up: ", err)
	}
}
