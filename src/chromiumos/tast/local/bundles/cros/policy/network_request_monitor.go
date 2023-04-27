// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/bundles/cros/policy/spellcheckutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto/checked"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/restriction"
	"chromiumos/tast/local/policyutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkRequestMonitor,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that the optional services are not making any unwanted network requests when disabled",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"ramyagopalan@google.com",
			"shahinmd@google.com", // Test author
		},
		BugComponent: "b:1129862",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		Data: []string{"spell_checking.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SpellCheckServiceEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// getPolicyList returns the list of policies to be set at the beginning of the test.
func getPolicyList() []policy.Policy {
	return []policy.Policy{
		&policy.SpellCheckServiceEnabled{Val: false},
	}
}

// getAnnotationHashCodes returns a list of annotations that are not supposed to be found in the logs when the optional services are disabled.
func getAnnotationHashCodes() []string {
	return []string{
		"132553989", // spellcheck_lookup
	}
}

func NetworkRequestMonitor(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Setup and start webserver (implicitly provides data form above).
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Perform cleanup.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	// Update policies.
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, getPolicyList()); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	// Setup the browser for lacros tests after the policy was set.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_network_request_monitor")

	// Open the net-export page and start logging.
	if err := annotations.StartLogging(ctx, cr, br); err != nil {
		s.Fatal("Failed to start logging: ", err)
	}

	spellCheckParam := spellcheckutil.TestCase{
		Name:              "disallow",
		Value:             &policy.SpellCheckServiceEnabled{Val: false},
		WantRestriction:   restriction.Disabled,
		WantSettingsCheck: checked.False,
		// "" means that there is no checkmark.
		WantContextCheck:     "",
		ShouldFindAnnotation: false,
	}
	if err := spellcheckutil.TriggerSpellCheck(ctx, spellCheckParam, cr, server, br, tconn); err != nil {
		s.Fatal("Failed to trigger and verify spellcheck: ", err)
	}

	// Stop logging and verify annotations related to the optional services are not found in the logs.
	_, err = annotations.StopLoggingVerifyNoAnnotation(ctx, cr, br, getAnnotationHashCodes())
	if err != nil {
		s.Fatal("Failed to stop logging and verify logs: ", err)
	}
}
