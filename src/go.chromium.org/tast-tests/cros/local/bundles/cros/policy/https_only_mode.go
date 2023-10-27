// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HTTPSOnlyMode,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that the HTTPSOnlyMode policy is properly applied",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"mpetrisor@google.com",
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome", "lacros"},
		Attr:         []string{"group:golden_tier"},
		Fixture:      fixture.LacrosPolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.HttpsOnlyMode{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func HTTPSOnlyMode(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Get .local domain alias of localhost.
	out, err := testexec.CommandContext(ctx, "avahi-resolve-address", "127.0.0.1").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("avahi-resolve-address failed: ", err)
	}

	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		s.Fatal("Malformed avahi output: ", string(out))
	}

	// Arbitrary port, does not matter.
	address := fmt.Sprintf("http://%s:%d", fields[1], 12345)

	for _, param := range []struct {
		name            string
		value           *policy.HttpsOnlyMode
		wantRestriction bool // whether the HTTPS only button is enabled
		setChecked      bool // whether to set the HTTPS only button to true
		httpAllowed     bool // whether HTTP connections are allowed
	}{
		{
			name:            "allowed",
			value:           &policy.HttpsOnlyMode{Val: "allowed"},
			wantRestriction: false,
			setChecked:      false,
			httpAllowed:     true,
		},
		{
			name:            "allowed_and_enabled",
			value:           &policy.HttpsOnlyMode{Val: "allowed"},
			wantRestriction: false,
			setChecked:      true,
			httpAllowed:     false,
		},
		{
			name:            "disallowed",
			value:           &policy.HttpsOnlyMode{Val: "disallowed"},
			wantRestriction: true,
			setChecked:      false,
			httpAllowed:     true,
		},
		{
			name:            "unset",
			value:           &policy.HttpsOnlyMode{Stat: policy.StatusUnset},
			wantRestriction: false,
			setChecked:      false,
			httpAllowed:     true,
		},
		{
			name:            "unset_enabled",
			value:           &policy.HttpsOnlyMode{Stat: policy.StatusUnset},
			wantRestriction: false,
			setChecked:      true,
			httpAllowed:     false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeLacros)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			conn, err := br.NewConn(ctx, "")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}

			// Open Security settings page.
			if err := conn.Navigate(ctx, "chrome://settings/security"); err != nil {
				s.Fatal("Failed to open Security settings page: ", err)
			}
			defer conn.Close()

			ui := uiauto.New(tconn)

			httpsOnlyButton := nodewith.Name("Always use secure connections").Role(role.ToggleButton)
			if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(httpsOnlyButton)(ctx); err != nil {
				s.Fatal("Failed to find HTTPS only toggle: ", err)
			}

			var info *uiauto.NodeInfo
			if info, err = ui.WithTimeout(5*time.Second).Info(ctx, httpsOnlyButton); err != nil {
				s.Fatal("Failed to get node info for HTTPS only toggle: ", err)
			}

			isRestricted := info.Restriction == restriction.Disabled
			if param.wantRestriction != isRestricted {
				b, _ := json.Marshal(info)
				s.Fatalf("Unexpected toggle button restricted state, got %s", b)
			}

			// Toggle HTTPS only mode button if not in desired state.
			isChecked := info.Checked == checked.True
			if param.setChecked != isChecked {
				var httpsOnlyChecked = ui.WaitUntilCheckedState(httpsOnlyButton, param.setChecked)
				// The UI might lag, keep trying until HTTPS only is toggled to the expected state.
				if err := ui.WithInterval(2*time.Second).DoDefaultUntil(httpsOnlyButton, httpsOnlyChecked)(ctx); err != nil {
					s.Fatal("Failed to toggle HTTPS only button: ", err)
				}
			}

			// Navigate to the HTTP-only site which does not support HTTPS connection upgrade.
			// We cannot use localhost or 127.0.0.1 as the policy does not apply to those hosts.
			if err := conn.Navigate(ctx, address); err != nil {
				s.Fatalf("Failed to navigate to %s: %v", address, err)
			}

			continueButton := nodewith.Name("Continue to site").Role(role.Button)

			// Check if Continue to site button exists which indicates that the HTTP connection warning is shown.
			if param.httpAllowed {
				if err := ui.EnsureGoneFor(continueButton, 10*time.Second)(ctx); err != nil {
					s.Error("Continue to site button is visible: ", err)
				}
			} else {
				if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(continueButton)(ctx); err != nil {
					s.Error("Continue to site button not visible: ", err)
				}
			}
		})
	}
}
