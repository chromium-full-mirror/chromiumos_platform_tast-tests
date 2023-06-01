// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/calendarintegration"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/passwordleakdetection"
	policyquickanswers "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/quickanswers"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/searchsuggestion"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/spellcheck"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/quickanswers"

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
			"shahinmd@google.com", // Test author.
		},
		BugComponent: "b:1129862",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		VarDeps:      []string{"policy.managedUserAccountPool"},
		Params: []testing.Param{{
			Fixture: fixture.FakeDMS,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.PersistentLacros, // FakeDMS with lacros policy.
			Val:               browser.TypeLacros,
		}},
		Data: []string{"spell_checking.html", "quick_answers.html", "password_leak_detection.html"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.PasswordLeakDetectionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersDefinitionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersDefinitionEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.SafeBrowsingProtectionLevel{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.SearchSuggestEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SearchSuggestEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.SpellCheckServiceEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// getPolicyList returns the list of policies to be set at the beginning of the test.
func getPolicyList() []policy.Policy {
	return []policy.Policy{
		&policy.CalendarIntegrationEnabled{Val: false},
		&policy.PasswordLeakDetectionEnabled{Val: false},
		&policy.QuickAnswersDefinitionEnabled{Val: false},
		&policy.QuickAnswersUnitConversionEnabled{Val: false},
		&policy.SearchSuggestEnabled{Val: false},
		&policy.SpellCheckServiceEnabled{Val: false},
	}
}

func NetworkRequestMonitor(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	gaiaCreds, err := credconfig.PickRandomCreds(s.RequiredVar("policy.managedUserAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse managed user creds: ", err)
	}

	policyBlob := policy.NewBlob()
	policyBlob.PolicyUser = gaiaCreds.User
	if err := fdms.WritePolicyBlob(policyBlob); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	opts := []chrome.Option{
		chrome.DMSPolicy(fdms.URL),  // FakeDMS for setting policies.
		chrome.GAIALogin(gaiaCreds), // Some of the optional service tests need a real GAIA account.
	}
	// If browser type is lacros, handle differently.
	if s.Param().(browser.Type) == browser.TypeLacros {
		opts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		if err != nil {
			s.Fatal("Failed to compute lacros chrome options: ", err)
		}
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Enable the pref that indicates the user has enabled the Quick Answers services.
	if err := quickanswers.SetPrefValue(ctx, tconn, "settings.quick_answers.enabled", true); err != nil {
		s.Fatal("Failed to enable Quick Answers: ", err)
	}

	// Setup and start webserver (implicitly provides data form above).
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Perform cleanup.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	// Update policies.
	policies := getPolicyList()
	policyBlob.AddPolicies(policies)
	// Updates policies in Chrome by updating the the policy blob of FakeDMS. This allows using a custom PolicyUser for the policy blob instead of the default one.
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, policyBlob); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}
	if err := policyutil.Verify(ctx, tconn, policies); err != nil {
		s.Fatal("Failed to verify updated policies: ", err)
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

	// Network traffic annotation hashcodes associated with the optional services.
	var hashCodes []string

	s.Run(ctx, "calendar_integration_service", func(ctx context.Context, s *testing.State) {
		calendarIntegrationParam := calendarintegration.TestCase{
			Name:                    "disabled",
			ShouldFindEventListView: false,
			ShouldFindManagedIcon:   true,
			ShouldFindAnnotation:    false,
			Policy:                  &policy.CalendarIntegrationEnabled{Val: false},
		}
		if err := calendarintegration.TriggerCalendarIntegration(ctx, calendarIntegrationParam, br, tconn, s); err != nil {
			s.Fatal("Failed to trigger and verify calendar integration: ", err)
		}
		hashCodes = append(hashCodes, calendarintegration.AnnotationHashCode)
	})

	s.Run(ctx, "spell_check_service", func(ctx context.Context, s *testing.State) {
		spellCheckParam := spellcheck.TestCase{
			Name:              "disallow",
			Value:             &policy.SpellCheckServiceEnabled{Val: false},
			WantRestriction:   restriction.Disabled,
			WantSettingsCheck: checked.False,
			// "" means that there is no checkmark.
			WantContextCheck:     "",
			ShouldFindAnnotation: false,
		}
		if err := spellcheck.TriggerSpellCheck(ctx, spellCheckParam, cr, server, br, tconn); err != nil {
			s.Fatal("Failed to trigger and verify spellcheck: ", err)
		}
		hashCodes = append(hashCodes, spellcheck.AnnotationHashCode)
	})

	s.Run(ctx, "quick_answers_service", func(ctx context.Context, s *testing.State) {
		quickAnswersDefinitionParam := policyquickanswers.DefinitionTestCase{
			Name:                  "disabled",
			ShouldFindAnnotation:  false,
			ShouldShowContextMenu: false,
			Policy:                &policy.QuickAnswersDefinitionEnabled{Val: false},
		}
		if err := policyquickanswers.TriggerQuickAnswersDefinition(ctx, quickAnswersDefinitionParam, server, br, tconn); err != nil {
			s.Fatal("Failed to trigger and verify quick answers definition: ", err)
		}

		quickAnswersUnitCoversionParam := policyquickanswers.UnitConversionTestCase{
			Name:                  "disabled",
			ShouldFindAnnotation:  false,
			ShouldShowContextMenu: false,
			Policy:                &policy.QuickAnswersUnitConversionEnabled{Val: false},
		}
		if err := policyquickanswers.TriggerQuickAnswersUnitConversion(ctx, quickAnswersUnitCoversionParam, server, br, tconn); err != nil {
			s.Fatal("Failed to trigger and verify quick answers unit conversion: ", err)
		}
		hashCodes = append(hashCodes, policyquickanswers.AnnotationHashCode)
	})

	s.Run(ctx, "password_leak_detection", func(ctx context.Context, s *testing.State) {
		if err := passwordleakdetection.TriggerPasswordLeakDetection(ctx, cr, server, br); err != nil {
			s.Fatal("Failed to trigger password leak detection: ", err)
		}
		hashCodes = append(hashCodes, passwordleakdetection.AnnotationHashCode)
	})

	s.Run(ctx, "search_suggestion", func(ctx context.Context, s *testing.State) {
		searchSuggestionParam := searchsuggestion.TestCase{
			Name:                 "disabled",
			ShouldFindAnnotation: false,
			Policy:               &policy.SearchSuggestEnabled{Val: false},
			Enabled:              false,
		}
		if err := searchsuggestion.TriggerSearchSuggestion(ctx, searchSuggestionParam, tconn, br); err != nil {
			s.Fatal("Failed to trigger search suggestion: ", err)
		}
		hashCodes = append(hashCodes, searchsuggestion.AnnotationID)
	})

	// Stop logging and verify network traffic annotations associated with the
	// optional services are not found in the logs.
	_, err = annotations.StopLoggingVerifyAnnotationSet(ctx, cr, br, false, hashCodes)
	if err != nil {
		s.Fatal("Failed to stop logging and verify logs: ", err)
	}
}
