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
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceMouseScrollAcceleration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test mouse scroll acceleration control exists",
		Contacts: []string{
			"cros-peripherals@google.com",
			"zhangwenyu@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Mouse
		BugComponent: "b:1131847",
		Fixture:      "chromeLoggedInWithInputDeviceSettingsSplit",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      time.Minute,
	})
}

// DeviceMouseScrollAcceleration tests mouse scroll acceleration enablement and scrolling speed slider.
func DeviceMouseScrollAcceleration(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

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

	// Set up mouse.
	mouse, err := input.Mouse(ctx)
	if err != nil {
		s.Fatal("Failed to create mouse: ", err)
	}
	defer mouse.Close(ctx)

	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Device)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)

	// Find Mouse row and click it.
	if err := ui.DoDefault(constants.MouseRow)(ctx); err != nil {
		s.Fatal("Failed to click mouse row: ", err)
	}

	// Verify if tast test mouse shows up.
	mouseHeading := nodewith.NameContaining("Tast virtual mouse").Role(role.Heading)
	if err := ui.WaitUntilExists(mouseHeading)(ctx); err != nil {
		s.Fatal("Failed to find Tast virtual mouse: ", err)
	}

	// Verify if enable scroll acceleration button exists.
	scrollAccelerationButton := nodewith.NameContaining("Scroll acceleration").Role(role.ToggleButton).First()
	if err := ui.WaitUntilExists(scrollAccelerationButton)(ctx); err != nil {
		s.Fatal("Failed to find scroll acceleration button: ", err)
	}

	// Verify if scrolling speed slider exists and is disabled.
	scrollingSpeedSlider := nodewith.NameContaining("Scrolling speed").Role(role.Slider).First()
	nodeInfo, err := ui.Info(ctx, scrollingSpeedSlider)
	if err != nil {
		s.Fatal("Failed to find scrolling speed slider: ", err)
	}

	_, disabled := nodeInfo.HTMLAttributes["disabled"]
	if !disabled {
		s.Fatal("Failed to find disabled scrolling speed slider")
	}

	// Turn off scroll acceleration should enable scrolling speed slider.
	if err := ui.LeftClick(scrollAccelerationButton)(ctx); err != nil {
		s.Fatal("Failed to click scroll acceleration button: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		nodeInfo, err = ui.Info(ctx, scrollingSpeedSlider)
		_, disabled = nodeInfo.HTMLAttributes["disabled"]
		if disabled {
			return errors.New("failed to find enabled scrolling speed slider")
		}

		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Second}); err != nil {
		s.Fatal("Failed to find enabled scrolling speed slider: ", err)
	}
}
