// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	androidui "chromiumos/tast/common/android/ui"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/optin"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayStoreOmnibox,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Installs a TWA and WebAPK app via Omnibox in Play Store",
		Contacts:     []string{"chromeos-apps-foundation-core@google.com", "tsergeant@chromium.org"},
		BugComponent: "b:1203766",
		Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container", "chrome"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm", "chrome"},
		}},
		Timeout: 10 * time.Minute,
		Fixture: "arcBootedWithPlayStore",
	})
}

// Time to wait for UI elements to appear in Play Store and Chrome
const uiTimeout = 30 * time.Second

func PlayStoreOmnibox(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome

	// Setup Chrome Test API Connection
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	d := s.FixtValue().(*arc.PreData).UIDevice

	// Navigate to URL
	conn, err := cr.NewConn(ctx, "")
	if err != nil {
		s.Fatal("Failed to create renderer: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "play_store_omnibox")

	// Jitsi Meet is a PWA which has manifest entries to prefer installation of the app through Play Store.
	const (
		title     = "jitsi meet"
		publisher = "8x8, inc"
		url       = "https://meet.jit.si"
	)

	s.Logf("Launching %s from %s via omnibox", title, url)

	if err := conn.Navigate(ctx, url); err != nil {
		s.Fatalf("Failed to navigate to the url %s: %s", url, err)
	}

	// Locate and click on the omnibox install button.
	ui := uiauto.New(tconn)
	installButton := nodewith.ClassName("PwaInstallView").Role(role.Button)
	if err := ui.WithTimeout(uiTimeout).LeftClick(installButton)(ctx); err != nil {
		s.Fatalf("Failed to left click omnibox install button on %s. Error: %s", url, err)
	}

	if err := checkPlayStoreLaunched(ctx, d, title, publisher); err != nil {
		s.Fatalf("Failed to check if play store launched for %s: %s", title, err)
	}

	// Close Play Store.
	if err := optin.ClosePlayStore(ctx, tconn); err != nil {
		s.Fatal("Failed to close Play Store: ", err)
	}
}

// checkPlayStoreLaunched validates the Install button, app title and publisher are present.
func checkPlayStoreLaunched(ctx context.Context, d *androidui.Device, title, publisher string) error {
	// Check that the install button exists
	_, err := playstore.FindInstallButton(ctx, d, uiTimeout)
	if err != nil {
		return errors.Wrap(err, "failed finding install button")
	}

	// Check that the title exists
	appTitle := d.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)"+title), androidui.Enabled(true))
	if err := appTitle.WaitForExists(ctx, uiTimeout); err != nil {
		return errors.Wrapf(err, "failed finding %s text", title)
	}

	// Check that the publisher exists
	appPublisher := d.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)"+publisher), androidui.Enabled(true))
	if err := appPublisher.WaitForExists(ctx, uiTimeout); err != nil {
		return errors.Wrapf(err, "failed finding %s text", publisher)
	}

	return nil
}
