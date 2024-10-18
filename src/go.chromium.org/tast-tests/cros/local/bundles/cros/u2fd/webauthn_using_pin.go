// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package u2fd

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/u2fd/util"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/u2fd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WebauthnUsingPIN,
		Desc: "Checks that WebAuthn using PIN succeeds",
		Contacts: []string{
			"cros-hwsec@google.com",
			"hcyang@google.com",
			"martinkr@chromium.org",
		},
		BugComponent: "b:1188704",
		Attr:         []string{"group:hwsec", "hwsec_nightly", "group:u2fd"},
		SoftwareDeps: []string{"chrome", "gsc"},
		Timeout:      5 * time.Minute,
		Data: []string{
			"webauthn.html",
			"bundle.js",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PinUnlockAutosubmitEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.QuickUnlockModeAllowlist{}, pci.VerifiedFunctionalityOS),
		},
		Fixture: fixture.ChromePolicyLoggedIn,
	})
}

func WebauthnUsingPIN(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := u2fd.NewWebAuthnHTTPServer(ctx, s.DataFileSystem())
	defer server.Close(cleanupCtx)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "error")

	u2fDaemon, err := u2fd.NewU2fDaemon(ctx)
	if err != nil {
		s.Fatal("Failed to connect to u2fd: ", err)
	}
	if err = u2fDaemon.WaitUntilInitialized(ctx); err != nil {
		s.Fatal("Failed to wait until u2fd is initialized: ", err)
	}

	const (
		username   = fixtures.Username
		password   = fixtures.Password
		PIN        = "123456"
		autosubmit = false
	)

	pinPolicies := []policy.Policy{
		&policy.QuickUnlockModeAllowlist{Val: []string{"PIN"}},
		&policy.PinUnlockAutosubmitEnabled{Val: autosubmit}}

	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	if err := policyutil.ServeAndVerify(ctx, fdms, cr, pinPolicies); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection")
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	if _, err = util.SetUpUserPIN(ctx, cr, keyboard, PIN, password, autosubmit); err != nil {
		s.Fatal("Failed to set up PIN: ", err)
	}

	authCallback := func(ctx context.Context, ui *uiauto.Context) error {

		var switchButton = nodewith.HasClass("PillButton").Name("Switch to PIN")

		var passwordField = nodewith.ClassName("AuthTextfield").Name("Password")

		var pinField = nodewith.ClassName("AuthTextfield").Name("PIN")

		if err := ui.Exists(passwordField)(ctx); err != nil {
			s.Fatal(err, "password field is not found")
		} else if err = ui.Exists(switchButton)(ctx); err != nil {
			s.Fatal(err, "switch button is not found")
		} else if err := ui.LeftClick(switchButton)(ctx); err != nil {
			s.Fatal(err, "switch button click failed")
		}

		// Wait for the field to be focused before entering the password.
		if err := ui.WithTimeout(6 * time.Second).WaitUntilExists(pinField)(ctx); err != nil {
			return errors.Wrap(err, "pin input is not visible")
		}

		// Type PIN into ChromeOS WebAuthn dialog. Optionally autosubmitted.
		pinString := PIN
		if !autosubmit {
			pinString += "\n"
		}
		if err := keyboard.Type(ctx, pinString); err != nil {
			return errors.Wrap(err, "failed to type PIN into ChromeOS auth dialog")
		}
		return nil
	}

	conn, err := cr.NewConn(ctx, server.URL+"/webauthn.html")
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer conn.Close()

	if err := u2fd.WebAuthnInLocalSite(ctx, conn, tconn, authCallback); err != nil {
		s.Fatal("Failed to perform WebAuthn: ", err)
	}
}
