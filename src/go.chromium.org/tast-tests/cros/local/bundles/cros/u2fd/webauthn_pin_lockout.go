// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
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
		Func: WebauthnPINLockout,
		Desc: "Checks that WebAuthn PIN lockouts after too many attempts and fallbacks to password",
		Contacts: []string{
			"cros-hwsec@google.com",
			"hcyang@google.com",
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

func WebauthnPINLockout(ctx context.Context, s *testing.State) {
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
		username           = fixtures.Username
		password           = fixtures.Password
		PIN                = "123456"
		IncorrectPIN       = "000000"
		autosubmit         = false
		pinLockoutAttempts = 4
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

		// Check if the UI is correct.
		var switchButton = nodewith.HasClass("PillButton").Name("Switch to PIN")

		var passwordField = nodewith.ClassName("AuthTextfield").Name("Password")

		var pinField = nodewith.ClassName("AuthTextfield").Name("PIN")

		if err := ui.Exists(passwordField)(ctx); err != nil {
			s.Fatal(err, "password input field is not found")
		} else if err = ui.Exists(switchButton)(ctx); err != nil {
			s.Fatal(err, "switchButton is not found")
		} else if err := ui.LeftClick(switchButton)(ctx); err != nil {
			s.Fatal(err, "switchButton click failed")
		}

		// Wait for the field to be focused before entering the password.
		if err := ui.WithTimeout(6 * time.Second).WaitUntilExists(pinField)(ctx); err != nil {
			return errors.Wrap(err, "pin input is not found")
		}

		// Type incorrect PIN into ChromeOS WebAuthn dialog. Optionally autosubmitted.
		pinString := IncorrectPIN
		if !autosubmit {
			pinString += "\n"
		}
		for i := 1; i <= pinLockoutAttempts; i++ {
			if err := keyboard.Type(ctx, pinString); err != nil {
				return errors.Wrap(err, "failed to type PIN into ChromeOS auth dialog")
			}
			if i != pinLockoutAttempts {
				node := nodewith.ClassName("Label").Name("Incorrect PIN")
				if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(node)(ctx); err != nil {
					return errors.Wrap(err, "failed to wait for Incorrect PIN message")
				}

				// After the first three failed attempts the pin should be still available
				if err := ui.Exists(pinField)(ctx); err != nil {
					s.Fatal(err, "PIN input field is not found")
				}
			} else {
				node := nodewith.ClassName("Label").NameStartingWith("Too many PIN attempts.")
				if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(node)(ctx); err != nil {
					return errors.Wrap(err, "failed to wait for Incorrect PIN message")
				}

				// After the fourth attempt the pin should be locked out for 30 sec,
				// and automatically the password field should be visible.
				if err := ui.WithTimeout(6 * time.Second).WaitUntilExists(passwordField)(ctx); err != nil {
					return errors.Wrap(err, "PIN input field is not visible")
				}
			}
		}

		// Cancel the WebAuthn dialog.
		node := nodewith.Role(role.Button).ClassName("IconButton").Name("Close dialog")
		if err = ui.DoDefault(node)(ctx); err != nil {
			return errors.Wrap(err, "failed to cancel the WebAuthn dialog")
		}

		// An error dialog will appear, press "Try again".
		node = nodewith.Role(role.Button).Name("Try again")
		if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(node)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for the retry button")
		}
		// The retry button shouldn't be continuously clicked with an interval too
		// short, as it restarts the processing on each click.
		if err = ui.WithInterval(2*time.Second).DoDefaultUntil(node, ui.Gone(node))(ctx); err != nil {
			return errors.Wrap(err, "failed to press the retry button")
		}

		// Choose platform authenticator again.
		platformAuthenticatorButton := nodewith.Role(role.Button).Name("This device")
		if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(platformAuthenticatorButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to select platform authenticator from transport selection sheet")
		}
		if err = ui.DoDefault(platformAuthenticatorButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to click button for platform authenticator")
		}

		// Wait for ChromeOS WebAuthn dialog.
		dialog := nodewith.ClassName("AuthDialogWidget")
		if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(dialog)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for the ChromeOS dialog")
		}

		// Password input field should exist, PIN shouldn't because it's locked out.
		if err := ui.Exists(passwordField)(ctx); err != nil {
			return errors.Wrap(err, "failed to find the password input field")
		}
		if err := ui.Exists(switchButton)(ctx); err == nil {
			return errors.Wrap(err, "the switch button shouldn't be exist")
		}

		// Type password into ChromeOS WebAuthn dialog.
		if err := keyboard.Type(ctx, password+"\n"); err != nil {
			return errors.Wrap(err, "failed to type password into ChromeOS auth dialog")
		}
		return nil
	}

	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, server.URL+"/webauthn.html")
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	if err := u2fd.WebAuthnInLocalSite(ctx, conn, tconn, authCallback); err != nil {
		s.Fatal("Failed to perform WebAuthn: ", err)
	}
}
