// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rgbkbd

import (
	"context"
	"log"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/rgbkbd"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CapslockRgbStateUpdates,
		Desc: "Checks that toggling Capslock updates the RGB backlight",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"jimmyxgong@chromium.org",
		},
		// ChromeOS > Software > System Services > Peripherals > Keyboard
		BugComponent: "b:1131926",
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard(), hwdep.NoSplitModifierKeyboard()),
		SoftwareDeps: []string{"chrome"},
	})
}

// CapslockRgbStateUpdates verifies that enabling or disabling Capslock updates
// the RGB backlight.
func CapslockRgbStateUpdates(ctx context.Context, s *testing.State) {
	const (
		dbusName             = "org.chromium.Rgbkbd"
		dbusPath             = "/org/chromium/Rgbkbd"
		dbusInterface        = "org.chromium.Rgbkbd"
		individualKey uint32 = 1
		job                  = "rgbkbd"
	)

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

	err = rgbkbdService.SetTestingMode(ctx, individualKey)
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

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	err = rgbkbd.ResetRgbkbdState(ctx, rgbkbdService)
	if err != nil {
		s.Fatal("Failed to reset rgbkbd state: ", err)
	}

	// Enable and Disable Capslock.
	kb.Accel(ctx, "alt+search")
	kb.Accel(ctx, "alt+search")

	// TODO(b/241255465): Add tast test that verifies calls made at startup
	// (initial caps lock state, default keyboard backlight color) when RGB keyboard
	// is supported.
	content, err := os.ReadFile("/run/rgbkbd/log")
	if err != nil {
		log.Fatal(err)
	}

	expectedLogLines := []string{
		"RGB::SetKeyColor - 44,255,77,0",
		"RGB::SetKeyColor - 57,255,77,0",
		"RGB::SetKeyColor - 44,255,255,210",
		"RGB::SetKeyColor - 57,255,255,210"}

	logsMatch, err := rgbkbd.LastLogLinesMatch(expectedLogLines, string(content), s.OutDir())

	if err != nil {
		s.Fatal("Error attempting to compare logs: ", err)
	}

	if !logsMatch {
		s.Fatal("Logs do not match: ", err)
	}
}
