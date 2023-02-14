// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package u2fd

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/u2fd/util"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/u2fd"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebauthnUsingPassword,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that WebAuthn using password succeeds",
		Contacts: []string{
			"cros-hwsec@chromium.org",
			"hcyang@google.com",
		},
		BugComponent: "b:1188704",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			"webauthn.html",
			"bundle.js",
		},
		Params: []testing.Param{{
			Name:              "tpm",
			ExtraSoftwareDeps: []string{"tpm", "no_gsc"},
			ExtraAttr:         []string{"informational"},
			Fixture:           "chromeLoggedIn",
			Val:               browser.TypeAsh,
		}, {
			Name:              "tpm_lacros",
			ExtraSoftwareDeps: []string{"tpm", "no_gsc", "lacros"},
			ExtraAttr:         []string{"informational"},
			Fixture:           "lacros",
			Val:               browser.TypeLacros,
		}, {
			Name:              "gsc",
			ExtraSoftwareDeps: []string{"gsc"},
			ExtraAttr:         []string{"informational"},
			Fixture:           "chromeLoggedIn",
			Val:               browser.TypeAsh,
		}, {
			Name:              "gsc_lacros",
			ExtraSoftwareDeps: []string{"gsc", "lacros"},
			ExtraAttr:         []string{"informational"},
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

	u2fDaemon, err := util.NewU2fDaemon(ctx)
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
	defer keyboard.Close()

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
