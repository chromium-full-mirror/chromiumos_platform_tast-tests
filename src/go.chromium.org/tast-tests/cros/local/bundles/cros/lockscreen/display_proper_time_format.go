// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lockscreen

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisplayProperTimeFormat,
		Desc:         "Test display proper time format on the lock screen",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"cros-lurs@google.com",
			"emaamari@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:hw_agnostic"},
	})
}

func DisplayProperTimeFormat(ctx context.Context, s *testing.State) {
	for _, param := range []struct {
		name            string
		use24HourFormat bool
	}{
		{
			name:            "24_hour_format",
			use24HourFormat: false,
		},
		{
			name:            "12_hour_format",
			use24HourFormat: true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()

			cr, err := chrome.New(ctx)
			if err != nil {
				s.Fatal("Chrome login failed: ", err)
			}
			defer cr.Close(ctx)

			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to connect to test API: ", err)
			}
			defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

			// Toggle time format setting.
			if err := tconn.Call(ctx, nil,
				`tast.promisify(chrome.settingsPrivate.setPref)`, "settings.clock.use_24hour_clock", param.use24HourFormat); err != nil {
				s.Fatal("Failed to set settings.clock.use_24hour_clock: ", err)
			}

			// Lock the screen
			if err := lockscreen.Lock(ctx, tconn); err != nil {
				s.Fatal("Failed to lock the screen: ", err)
			}
			if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
				s.Fatalf("Waiting for the screen to be locked failed: %v (last status %+v)", err, st)
			}
			// Unlock the screen to ensure subsequent tests aren't affected by the screen remaining locked.
			// TODO(b/187794615): Remove once chrome.go has a way to clean up the lock screen state.
			defer func() {
				if err := lockscreen.Unlock(ctx, tconn); err != nil {
					s.Fatal("Failed to unlock the screen: ", err)
				}
			}()

			// Ensure the status area is visible.
			ui := uiauto.New(tconn)
			statusArea := nodewith.ClassName(ash.StatusAreaClassName)
			if err := ui.WaitUntilExists(statusArea)(ctx); err != nil {
				s.Fatal("Failed to find status area widget: ", err)
			}

			// Verify time format.
			// Example content of the date tray: "Calendar view, Monday, November 13, 2023, 1:38 PM".
			TimeView := nodewith.ClassName("DateTray").First()
			info, err := ui.Info(ctx, TimeView)
			if err != nil {
				s.Fatal("Failed to get node info for the time view: ", err)
			}
			if is24HourFormat(info.Name) != param.use24HourFormat {
				s.Fatal("Wrong date time format: ", info.Name)
			}
		})
	}
}

func is24HourFormat(timeString string) bool {
	// 12-hour format contains either AM or PM.
	return !strings.Contains(timeString, "AM") && !strings.Contains(timeString, "PM")
}
