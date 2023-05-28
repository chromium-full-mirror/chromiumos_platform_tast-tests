// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/u2fd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type webauthnTestParam struct {
	fingerprintSupported bool
	browserType          browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebauthnFactors,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that WebAuthn options are enabled or disabled based on the policy value",
		Contacts: []string{
			"cros-hwsec@google.com",
			"hcyang@google.com", // Test author
		},
		BugComponent: "b:1188704",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "pinweaver"},
		Data: []string{
			"webauthn/webauthn.html",
			"webauthn/bundle.js",
		},
		Params: []testing.Param{
			{
				Val: webauthnTestParam{
					fingerprintSupported: false,
					browserType:          browser.TypeAsh,
				},
				Fixture: fixture.ChromePolicyLoggedIn,
			},
			{
				Name:              "fingerprint",
				ExtraHardwareDeps: hwdep.D(hwdep.Fingerprint()),
				Val: webauthnTestParam{
					fingerprintSupported: true,
					browserType:          browser.TypeAsh,
				},
				Fixture: fixture.ChromePolicyLoggedIn,
			},
			{
				Name: "lacros",
				Val: webauthnTestParam{
					fingerprintSupported: false,
					browserType:          browser.TypeLacros,
				},
				Fixture: fixture.LacrosPolicyLoggedIn,
			},
			{
				Name:              "fingerprint_lacros",
				ExtraHardwareDeps: hwdep.D(hwdep.Fingerprint()),
				Val: webauthnTestParam{
					fingerprintSupported: true,
					browserType:          browser.TypeLacros,
				},
				Fixture: fixture.LacrosPolicyLoggedIn,
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.WebAuthnFactors{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickUnlockModeAllowlist{}, pci.VerifiedValue),
		},
	})
}

// WebauthnFactors sets up multiple policies, but only tests behavior that it controls.
// It tests "setup" and "webauthn", but not "quick_unlock" or other auth usages. So it will include
// just enough test cases to verify:
// 1. WebAuthnFactors enabled will enable "setup" the auth method and using it for "webauthn",
// even if all other policies disable it.
//
// 2. WebAuthnFactors disabled will disable using the auth method for "webauthn" even if all other
// policies enabled it, but will not disable "setup" for that auth method.
func WebauthnFactors(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server := u2fd.NewWebAuthnHTTPServer(ctx, s.DataFileSystem())
	defer server.Close(cleanupCtx)

	type testCase struct {
		name            string
		webAuthnFactors policy.WebAuthnFactors
		// Since this policy have similar set of entries and controls whether an auth method can be set with
		// WebAuthnFactors together, we want test cases that verify behaviors are correct when both
		// policies are set and have different values. See comments in testCases.
		quickUnlockModeAllowlist policy.QuickUnlockModeAllowlist
	}

	const PIN = "123456"

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	testCases := []testCase{
		{
			name:                     "unset",
			webAuthnFactors:          policy.WebAuthnFactors{Stat: policy.StatusUnset},
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Stat: policy.StatusUnset},
		},
		{
			name:                     "empty",
			webAuthnFactors:          policy.WebAuthnFactors{Val: []string{}},
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Stat: policy.StatusUnset},
		},
		// QuickUnlockModeAllowlist set to empty list shouldn't affect set and webauthn capabilities.
		{
			name:                     "all",
			webAuthnFactors:          policy.WebAuthnFactors{Val: []string{"all"}},
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Val: []string{}},
		},
		{
			name:                     "pin",
			webAuthnFactors:          policy.WebAuthnFactors{Val: []string{"PIN"}},
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Stat: policy.StatusUnset},
		},
		{
			name:                     "webauthn_empty_quick_unlock_all",
			webAuthnFactors:          policy.WebAuthnFactors{Val: []string{}},
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Val: []string{"all"}},
		},
	}

	fingerprintSupported := s.Param().(webauthnTestParam).fingerprintSupported

	if fingerprintSupported {
		testCases = append(testCases, testCase{
			name:                     "fingerprint",
			webAuthnFactors:          policy.WebAuthnFactors{Val: []string{"FINGERPRINT"}},
			quickUnlockModeAllowlist: policy.QuickUnlockModeAllowlist{Stat: policy.StatusUnset},
		},
		)
	}

	for _, param := range testCases {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name+".txt")

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			policies := []policy.Policy{
				&param.quickUnlockModeAllowlist,
				&param.webAuthnFactors,
			}

			// Update policies.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Open the Lockscreen page where we can set a PIN.
			conn, err := apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/osPrivacy/lockScreen")
			if err != nil {
				s.Fatal("Failed to connect to the settings page: ", err)
			}
			defer conn.Close()

			ui := uiauto.New(tconn)

			// Find and enter the password in the pop up window.
			if err := ui.LeftClick(nodewith.Name("Password").Role(role.TextField))(ctx); err != nil {
				s.Fatal("Failed to find the password field: ", err)
			}
			if err := kb.Type(ctx, fixtures.Password+"\n"); err != nil {
				s.Fatal("Failed to type password: ", err)
			}

			// Find node info for the radio button group node.
			rgNode, err := ui.Info(ctx, nodewith.Role(role.RadioGroup))
			if err != nil {
				s.Fatal("Failed to find radio group: ", err)
			}

			pinCapabilities := getExpectedWebAuthnCapabilities(&param.quickUnlockModeAllowlist, &param.webAuthnFactors, "PIN")

			var wantRestriction restriction.Restriction
			if pinCapabilities.set {
				wantRestriction = restriction.None
			} else {
				wantRestriction = restriction.Disabled
			}

			// Check that the radio button group has the expected restriction.
			if rgNode.Restriction != wantRestriction {
				s.Errorf("Unexpected radio button group state: got %v, want %v", rgNode.Restriction, wantRestriction)
			}

			if fingerprintSupported {
				fingerprintCapabilities := getExpectedWebAuthnCapabilities(&param.quickUnlockModeAllowlist, &param.webAuthnFactors, "FINGERPRINT")
				found, err := ui.IsNodeFound(ctx, nodewith.Name("Edit Fingerprints").Role(role.StaticText))
				if err != nil {
					s.Fatal("Failed to find Edit Fingerprints node: ", err)
				}
				if found != fingerprintCapabilities.set {
					s.Errorf("Failed checking if fingerprint can be set: got %v, want %v", found, fingerprintCapabilities.set)
				}
			}

			// If PIN can be set, we set up a PIN and see if the lock screen UI corresponds to PIN's unlock capability.
			if pinCapabilities.set {
				if err := uiauto.Combine("switch to PIN or password and wait for PIN dialog",
					// Find and click on radio button "PIN or password".
					ui.LeftClick(nodewith.Name("PIN or password").Role(role.RadioButton)),
					// Find and click on "Set up PIN" button.
					ui.LeftClick(nodewith.Name("Set up PIN").Role(role.Button)),
					// Wait for the PIN pop up window to appear.
					ui.WaitUntilExists(nodewith.Name("Enter your PIN").Role(role.StaticText)),
				)(ctx); err != nil {
					s.Fatal("Failed to open PIN dialog: ", err)
				}

				// Enter the PIN.
				if err := kb.Type(ctx, PIN); err != nil {
					s.Fatal("Failed to type PIN: ", err)
				}

				continueButton := nodewith.Name("Continue").Role(role.Button)

				// Find the Continue button node.
				if err := ui.WaitUntilExists(continueButton)(ctx); err != nil {
					s.Fatal("Failed to find the Continue button: ", err)
				}

				if err := ui.LeftClick(continueButton)(ctx); err != nil {
					s.Fatal("Failed to click the Continue button: ", err)
				}

				if err := ui.WaitUntilExists(nodewith.Name("Confirm your PIN").Role(role.StaticText))(ctx); err != nil {
					s.Fatal("Failed to find the PIN confirmation dialog: ", err)
				}

				// Enter the PIN.
				if err := kb.Type(ctx, PIN); err != nil {
					s.Fatal("Failed to type PIN: ", err)
				}

				confirmButton := nodewith.Name("Confirm").Role(role.Button)

				if err := ui.LeftClick(confirmButton)(ctx); err != nil {
					s.Fatal("Failed to click the Confirm button: ", err)
				}

				// Don't lock the screen before the add PIN operation ended.
				if err := ui.WaitUntilGone(nodewith.Name("Confirm your PIN").Role(role.StaticText))(ctx); err != nil {
					s.Fatal("Failed to wait for PIN confirmation dialog to disappear: ", err)
				}

				conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, s.Param().(webauthnTestParam).browserType, server.URL+"/webauthn/webauthn.html")
				if err != nil {
					s.Fatal("Failed to open the browser: ", err)
				}
				defer closeBrowser(cleanupCtx)
				defer conn.Close()

				if err := verifyInSessionAuthDialog(ctx, conn, tconn, pinCapabilities.webAuthn); err != nil {
					s.Fatal("Failed to verify in session auth dialog: ", err)
				}

				// Delete the PIN so upcoming tests don't get affected.
				if err := ui.DoDefault(nodewith.Name("Password only").Role(role.RadioButton))(ctx); err != nil {
					s.Fatal("Failed to delete PIN: ", err)
				}
			}
		})
	}
}

type webAuthnCapabilities struct {
	// Whether the auth method is allowed to be set in OS Settings.
	set bool
	// Whether the auth method is allowed to be used for WebAuthn.
	webAuthn bool
}

func getExpectedWebAuthnCapabilities(quickUnlockModeAllowlist *policy.QuickUnlockModeAllowlist, webauthnFactors *policy.WebAuthnFactors, authMethod string) webAuthnCapabilities {
	set, webAuthn := false, false
	if quickUnlockModeAllowlist.Stat != policy.StatusUnset {
		for _, entry := range quickUnlockModeAllowlist.Val {
			if entry == authMethod || entry == "all" {
				set = true
				// If WebAuthnFactors is unset, the pref value will be inherited from QuickUnlockModeAllowlist.
				if webauthnFactors.Stat == policy.StatusUnset {
					webAuthn = true
				}
			}
		}
	}
	if webauthnFactors.Stat != policy.StatusUnset {
		for _, entry := range webauthnFactors.Val {
			if entry == authMethod || entry == "all" {
				set = true
				webAuthn = true
			}
		}
	}
	return webAuthnCapabilities{
		set,
		webAuthn,
	}
}

func verifyInSessionAuthDialog(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, pinEnabled bool) error {
	u2fd.InitiateMakeCredentialInLocalSite(ctx, conn, u2fd.WebAuthnRegistrationConfig{Uv: "preferred"})

	if err := u2fd.ChoosePlatformAuthenticator(ctx, tconn); err != nil {
		return err
	}
	if err := u2fd.WaitForWebAuthnDialog(ctx, tconn); err != nil {
		return err
	}

	ui := uiauto.New(tconn)

	err := ui.Exists(nodewith.ClassName("LoginPinView"))(ctx)
	if (err == nil) != pinEnabled {
		return errors.Errorf("PIN pad existence not expected: want %v, get %v", pinEnabled, err == nil)
	}

	return nil
}

// randomUsername generates a random username of length 20.
func randomUsername() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"

	ret := make([]byte, 20)
	for i := range ret {
		ret[i] = letters[rand.Intn(len(letters))]
	}

	return string(ret)
}
