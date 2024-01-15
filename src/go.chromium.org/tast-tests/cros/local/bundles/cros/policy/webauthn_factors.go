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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
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
		Timeout: 3 * time.Minute,
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
			settingsPage := policyutil.OSSettingsPageWithPassword(ctx, cr, "osPrivacy/lockScreen", fixtures.Password)

			ui := uiauto.New(tconn)

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

			pinCapabilities := getExpectedWebAuthnCapabilities(&param.quickUnlockModeAllowlist, &param.webAuthnFactors, "PIN")
			var wantRestriction restriction.Restriction
			if pinCapabilities.set {
				wantRestriction = restriction.None
			} else {
				wantRestriction = restriction.Disabled
			}

			// Check that the PIN button has the expected restriction.
			if err := settingsPage.
				SelectNode(ctx, nodewith.Name("Set up").Role(role.Button)).
				Restriction(wantRestriction).
				Verify(); err != nil {
				s.Fatal("Unexpected button state: want ", wantRestriction)
			}

			// If PIN can be set, we set up a PIN and see if the lock screen UI corresponds to PIN's unlock capability.
			if pinCapabilities.set {
				if err := settingsPage.SetUpPin(ctx, PIN, true /*completeSetup*/).Verify(); err != nil {
					s.Fatal("Failed to set up PIN: ", err)
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
				settingsPage = policyutil.OSSettingsPageWithPassword(ctx, cr, "osPrivacy/lockScreen", fixtures.Password)
				if err := uiauto.Combine("delete PIN",
					ui.DoDefault(nodewith.HasClass("icon-more-vert").Ancestor(ossettings.WindowFinder).Role(role.PopUpButton)),
					ui.DoDefault(nodewith.Name("Remove").Ancestor(ossettings.WindowFinder).Role(role.MenuItem)))(ctx); err != nil {
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
