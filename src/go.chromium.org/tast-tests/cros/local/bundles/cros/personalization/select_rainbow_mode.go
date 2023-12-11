// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"io/ioutil"
	"log"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast-tests/cros/local/rgbkbd"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SelectRainbowMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that rainbow mode updates the correct number of keys for each device",
		Contacts: []string{
			"cros-peripherals@google.com",
			"michaelcheco@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
	})
}

// RainbowMode constants defined in: src/platform2/rgbkbd/constants.h.
// RgbKeyboardCapabilities enum defined in: src/platform2/system_api/dbus/rgbkbd/dbus-constants.h.

// SelectRainbowMode checks that selecting the "Rainbow" keyboard backlight color  updates the correct number of keys for each device.
func SelectRainbowMode(ctx context.Context, s *testing.State) {
	const (
		dbusName                 = "org.chromium.Rgbkbd"
		individualKey     uint32 = 1
		fourZoneFortyLed  uint32 = 2
		fourZoneTwelveLed uint32 = 3
		fourZoneFourLed   uint32 = 4
		job                      = "rgbkbd"
	)

	for _, tc := range []struct {
		name           string
		capability     uint32
		expectedLogLen int
	}{
		{
			name:           "Vell",
			capability:     individualKey,
			expectedLogLen: 77,
		},
		{
			name:           "Taniks",
			capability:     fourZoneFortyLed,
			expectedLogLen: 40,
		},
		{
			name:           "Osiris",
			capability:     fourZoneTwelveLed,
			expectedLogLen: 12,
		},
		{
			name:           "Mithrax",
			capability:     fourZoneFourLed,
			expectedLogLen: 4,
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()

			s.Logf("Restarting %s job and waiting for %s service", job, dbusName)
			if err := upstart.RestartJob(ctx, job); err != nil {
				s.Fatalf("Failed to start %s: %v", job, err)
			}

			rgbkbdService, err := rgbkbd.NewRgbkbd(ctx)
			if err != nil {
				s.Fatalf("Failed to connect to %s: %v", dbusName, err)
			}

			err = rgbkbdService.SetTestingMode(ctx, tc.capability)
			if err != nil {
				s.Fatal("Failed to set testing mode: ", err)
			}

			cr, err := chrome.New(ctx, chrome.EnableFeatures("RgbKeyboard"))
			if err != nil {
				s.Fatal("Failed to start Chrome: ", err)
			}
			defer cr.Close(cleanupCtx)

			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to connect Test API: ", err)
			}
			defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
			// The test has a dependency of network speed, so we give uiauto.Context ample
			// time to wait for nodes to load.
			ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

			if err := uiauto.Combine("open Personalization Hub and verify Keyboard settings available",
				personalization.OpenPersonalizationHub(ui),
				ui.WaitUntilExists(nodewith.Role(role.StaticText).NameContaining("Keyboard backlight")))(ctx); err != nil {
				s.Fatal("Failed to show Keyboard settings: ", err)
			}
			if err := selectRainbowMode(ui)(ctx); err != nil {
				s.Fatal("Failed to select backlight color rainbow: ", err)
			}

			content, err := ioutil.ReadFile("/run/rgbkbd/log")
			if err != nil {
				log.Fatal(err)
			}

			count, err := rgbkbd.RainbowModeCount(string(content))
			if err != nil {
				s.Fatal("Failed to get rainbow mode count: ", err)
			}
			if count != tc.expectedLogLen {
				s.Fatalf("Unexpected # of calls to SetKeyColor ... got %d, want %d", tc.expectedLogLen, count)
			}
		})
	}
}

func selectRainbowMode(ui *uiauto.Context) uiauto.Action {
	rainbowColor := "Rainbow"
	rainbowColorOption := nodewith.Role(role.RadioButton).Name(rainbowColor)

	return uiauto.Combine("validate the selected backlight color",
		ui.MakeVisible(rainbowColorOption),
		ui.WaitUntilExists(rainbowColorOption.Attribute("checked", "false")),
		ui.LeftClick(rainbowColorOption),
		ui.WaitUntilExists(rainbowColorOption.Attribute("checked", "true")))
}
