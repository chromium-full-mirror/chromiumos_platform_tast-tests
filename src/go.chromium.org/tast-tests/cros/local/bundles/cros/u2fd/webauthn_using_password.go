// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package u2fd

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/u2fd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebauthnUsingPassword,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that WebAuthn using password succeeds",
		Contacts: []string{
			"cros-hwsec@google.com",
			"hcyang@google.com",
		},
		BugComponent: "b:1188704",
		Attr:         []string{"group:hwsec", "hwsec_nightly", "group:u2fd"},
		SoftwareDeps: []string{"chrome", "tpm"},
		Data: []string{
			"webauthn.html",
			"bundle.js",
		},
		Params: []testing.Param{{
			Name:              "tpm",
			ExtraSoftwareDeps: []string{"no_gsc", "no_tpm_dynamic"},
			Fixture:           "chromeLoggedIn",
			Val:               browser.TypeAsh,
		}, {
			Name:              "tpm_lacros",
			ExtraSoftwareDeps: []string{"no_gsc", "lacros", "no_tpm_dynamic"},
			Fixture:           "lacros",
			Val:               browser.TypeLacros,
		}, {
			Name:              "tpm_dynamic",
			ExtraSoftwareDeps: []string{"tpm_dynamic"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
			Fixture:           "chromeLoggedIn",
			Val:               browser.TypeAsh,
		}, {
			Name:              "tpm_dynamic_lacros",
			ExtraSoftwareDeps: []string{"tpm_dynamic", "no_gsc", "lacros"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
			Fixture:           "lacros",
			Val:               browser.TypeLacros,
		}, {
			Name:              "gsc",
			ExtraSoftwareDeps: []string{"gsc"},
			Fixture:           "chromeLoggedIn",
			Val:               browser.TypeAsh,
		}, {
			Name:              "gsc_lacros",
			ExtraSoftwareDeps: []string{"gsc", "lacros"},
			Fixture:           "lacros",
			Val:               browser.TypeLacros,
		}},
		Timeout: 5 * time.Minute,
	})
}

func WebauthnUsingPassword(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := u2fd.NewWebAuthnHTTPServer(ctx, s.DataFileSystem())
	defer server.Close(cleanupCtx)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	bt := s.Param().(browser.Type)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "error")

	u2fDaemon, err := u2fd.NewU2fDaemon(ctx)
	if err != nil {
		s.Fatal("Failed to connect to u2fd: ", err)
	}
	if err = u2fDaemon.WaitUntilInitialized(ctx); err != nil {
		s.Fatal("Failed to wait until u2fd is initialized: ", err)
	}

	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, server.URL+"/webauthn.html")
	if err != nil {
		s.Fatalf("Failed to open the %v browser: %v", bt, err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	authCallback := func(ctx context.Context, ui *uiauto.Context) error {
		// Check if the UI is correct.
		if err := ui.Exists(nodewith.ClassName("LoginPasswordView"))(ctx); err != nil {
			return errors.Wrap(err, "failed to find the password input field")
		}
		// Type password into ChromeOS WebAuthn dialog.
		if err := keyboard.Type(ctx, chrome.DefaultPass+"\n"); err != nil {
			return errors.Wrap(err, "failed to type password into ChromeOS auth dialog")
		}
		return nil
	}

	if err := u2fd.WebAuthnInLocalSite(ctx, conn, tconn, authCallback); err != nil {
		s.Fatal("Failed to perform WebAuthn: ", err)
	}
}
