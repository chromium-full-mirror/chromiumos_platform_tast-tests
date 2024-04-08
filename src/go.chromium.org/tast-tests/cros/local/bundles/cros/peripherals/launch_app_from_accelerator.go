// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package peripherals

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// accelTestParams contains all the data needed to run a single test iteration.
type accelTestParams struct {
	app         apps.App
	keystroke   string
	featureFlag string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchAppFromAccelerator,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Peripherals app can be found and launched with an accelerator",
		Contacts: []string{
			"cros-peripherals@google.com",
			"michaelcheco@google.com",
			"jimmyxgong@google.com",
			"ashleydp@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals
		BugComponent: "b:1150827",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name: "diagnostics",
				Val: accelTestParams{
					app:       apps.Diagnostics,
					keystroke: "Ctrl+Search+Esc",
				},
			},
		},
	})
}

// LaunchAppFromAccelerator verifies launching an app with an accelerator.
func LaunchAppFromAccelerator(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	params := s.Param().(accelTestParams)

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx) // Close our own chrome instance.

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Error creating keyboard instance: ", err)
	}
	defer kb.Close(ctx)

	s.Logf("Sending keystroke: %s", params.keystroke)
	err = kb.Accel(ctx, params.keystroke)
	if err != nil {
		s.Fatal("Failed to search and launch app: ", err)
	}

	err = ash.WaitForApp(ctx, tconn, params.app.ID, time.Minute)
	if err != nil {
		s.Fatal("Could not find app in shelf after launch: ", err)
	}
}
