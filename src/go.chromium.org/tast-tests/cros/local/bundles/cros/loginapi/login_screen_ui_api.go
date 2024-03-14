// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package loginapi

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LoginScreenUIAPI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test chrome.loginScreenUi Extension API",
		Contacts: []string{
			"chromeos-commercial-identity@google.com",
			"mpetrisor@chromium.org",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Identity > Imprivata
		BugComponent: "b:1253162",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"reboot", "chrome", "vpd"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceLoginScreenExtensions{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func LoginScreenUIAPI(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	defer func(ctx context.Context) {
		// Use cr as a reference to close the last started Chrome instance.
		if err := cr.Close(ctx); err != nil {
			s.Error("Failed to close Chrome connection: ", err)
		}
	}(ctx)

	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	policies := []policy.Policy{
		&policy.DeviceLoginScreenExtensions{
			Val: []string{mgs.LoginScreenExtensionID},
		},
	}

	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	// Close the previous Chrome instance.
	if err := cr.Close(ctx); err != nil {
		s.Fatal("Failed to close Chrome connection: ", err)
	}

	// Restart Chrome, forcing Devtools to be available on the login screen.
	cr, err = chrome.New(ctx,
		chrome.NoLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
		chrome.ExtraArgs("--force-devtools-available"))
	if err != nil {
		s.Fatal("Chrome restart failed: ", err)
	}

	loginScreenBGURL := chrome.ExtensionBackgroundPageURL(mgs.LoginScreenExtensionID)
	bgConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(loginScreenBGURL))
	if err != nil {
		s.Fatal("Failed to connect to login screen background page: ", err)
	}
	defer bgConn.Close()

	// Show window.html.
	// The file window.html is bundled with the extension.
	if err := bgConn.Eval(ctx, `new Promise((resolve, reject) => {
		chrome.loginScreenUi.show({url: "window.html"}, () => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, nil); err != nil {
		s.Fatal("Failed to show window: ", err)
	}

	windowURL := fmt.Sprintf("chrome-extension://%s/window.html", mgs.LoginScreenExtensionID)
	windowConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(windowURL))
	if err != nil {
		s.Fatal("Failed to connect to window: ", err)
	}
	defer windowConn.Close()

	windowCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Check that the window title is correct.
	// WaitForExpr has to be used since the window title is not updated immediately.
	expectedWindowTitle := "Login screen APIs test extension"
	expr := fmt.Sprintf(`document.querySelector('title').innerText === '%s'`, expectedWindowTitle)
	if err := windowConn.WaitForExpr(windowCtx, expr); err != nil {
		s.Error("Window title does not match: ", err)
	}

	// Close the window.
	if err := bgConn.Eval(ctx, `new Promise((resolve, reject) => {
		chrome.loginScreenUi.close(() => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, nil); err != nil {
		s.Fatal("Failed to close window: ", err)
	}

	// Check that the window is closed.
	available, err := cr.IsTargetAvailable(ctx, chrome.MatchTargetURL(windowURL))
	if err != nil {
		s.Fatal("Failed to get targets: ", err)
	}
	if available {
		s.Error("Window was not closed")
	}
}
