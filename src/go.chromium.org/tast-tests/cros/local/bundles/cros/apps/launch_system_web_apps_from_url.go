// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchSystemWebAppsFromURL,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that System Web Apps can launch through their URL",
		Contacts: []string{
			"cros-apps-platform-core@google.com",
		},
		BugComponent: "b:1168727",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

// LaunchSystemWebAppsFromURL tries to navigate to System Web Apps from their chrome:// URL.
// This test launches several SWAs to trigger different launch paths.
func LaunchSystemWebAppsFromURL(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	systemWebApps, err := apps.ListRegisteredSystemWebApps(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get list of SWAs: ", err)
	}

	// Get a handle to the input keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard handle: ", err)
	}
	defer kb.Close(ctx)

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	testAppInternalNames := map[string]struct{}{
		// Link capture, stand alone app.
		"OSSettings": {},
		// Link capture, File Handling (origin trial) app.
		"Media": {},
		// Tabbed app, non-link capture.
		"Crosh": {},
	}

	for _, systemWebApp := range systemWebApps {
		if _, exists := testAppInternalNames[systemWebApp.InternalName]; exists {
			s.Run(ctx, systemWebApp.Name, func(ctx context.Context, s *testing.State) {
				startURL := systemWebApp.StartURL
				s.Log("Navigating to ", startURL)
				if err := verifyAndLaunchSystemWebAppFromURL(ctx, cr, tconn, kb, s.OutDir(), systemWebApp.Name, startURL); err != nil {
					s.Fatalf("Failed navigating to %q: %v", startURL, err)
				}
			})
		}
	}
}

// verifyAndLaunchSystemWebAppFromURL types the URL into the Chrome omnibox and verifies the SWA page loads.
func verifyAndLaunchSystemWebAppFromURL(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, outDir, appName, appURL string) (retErr error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Open up an empty Chrome browser window.
	conn, err := cr.NewConn(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to open a new renderer")
	}
	defer conn.Close()

	// Take a screenshot of the display before closing the SWA window (if it exists).
	var connTarget *chrome.Conn
	defer func() {
		if retErr != nil {
			screenshotFile := filepath.Join(outDir, appName+"_failed.png")
			if err := screenshot.Capture(ctx, screenshotFile); err != nil {
				testing.ContextLog(ctx, "Failed to take screenshot: ", err)
			}
		}

		if connTarget != nil {
			connTarget.Close()
		}
	}()

	ui := uiauto.New(tconn)
	omniboxFinder := browserui.AddressBarFinder
	if err := uiauto.Combine("open target "+appURL,
		ui.LeftClick(omniboxFinder),
		ui.RetrySilently(len("about:blank"), func(ctx context.Context) error {
			expectedValue := ""
			if err := uiauto.Combine("delete all text",
				keyboard.AccelAction("ctrl+a"),
				keyboard.AccelAction("Backspace"))(ctx); err != nil {
				return err
			}

			if nodeInfo, err := ui.Info(ctx, omniboxFinder); err != nil {
				return err
			} else if nodeInfo.Value != expectedValue {
				return errors.Errorf("failed to clean the address bar: got: %s; want: %s", nodeInfo.Value, expectedValue)
			}
			return nil
		}),
		keyboard.TypeAction(appURL),
		keyboard.AccelAction("Enter"))(ctxWithTimeout); err != nil {
		return err
	}

	connTarget, err = cr.NewConnForTarget(ctxWithTimeout, chrome.MatchTargetURLPrefix(appURL))
	if err != nil {
		return errors.Wrap(err, "failed getting connection to new target")
	}

	if connTarget.WaitForExpr(ctxWithTimeout, "document.readyState === 'complete'"); err != nil {
		return errors.Wrap(err, "failed waiting for URL to load")
	}

	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to close app window")
	}

	return nil
}
