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
	"chromiumos/tast/local/bundles/cros/policy/quickanswersutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/quickanswers"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         QuickAnswersUnitConversionEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test QuickAnswersUnitConversionEnabled policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr:         []string{"group:commercial_limited"},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{"quick_answers.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           fixture.LacrosPolicyLoggedIn,
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// QuickAnswersUnitConversionEnabled tests that Quick Answers unit conversion
// can be enabled and disabled via policy.
func QuickAnswersUnitConversionEnabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := quickanswers.SetPrefValue(ctx, tconn, "settings.quick_answers.enabled", true); err != nil {
		s.Fatal("Failed to enable Quick Answers: ", err)
	}

	for _, param := range []quickanswersutil.UnitConversionTestCase{
		{
			Name:                  "unset",
			ShouldFindAnnotation:  true,
			ShouldShowContextMenu: true,
			Policy:                &policy.QuickAnswersUnitConversionEnabled{Stat: policy.StatusUnset},
		},
		{
			Name:                  "enabled",
			ShouldFindAnnotation:  true,
			ShouldShowContextMenu: true,
			Policy:                &policy.QuickAnswersUnitConversionEnabled{Val: true},
		},
		{
			Name:                  "disabled",
			ShouldFindAnnotation:  false,
			ShouldShowContextMenu: false,
			Policy:                &policy.QuickAnswersUnitConversionEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup a browser.
			bt := s.Param().(browser.Type)
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			if err := quickanswersutil.TriggerQuickAnswersUnitConversion(ctx, param, server, br, tconn); err != nil {
				s.Fatal("Failed to trigger and verify quick answers unit conversion: ", err)
			}

			// Stop logging and check the logs for quick_answers_loader NetworkTrafficAnnotationTag.
			foundAnnotation, err := annotations.StopLoggingCheckLogs(ctx, cr, br, quickanswersutil.AnnotationID)
			if err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			}
			if param.ShouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Expected: %t. Actual: %t", param.ShouldFindAnnotation, foundAnnotation)
			}
		})
	}
}
