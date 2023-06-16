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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/defaultsearchprovider"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/passwordleakdetection"
	policyquickanswers "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/quickanswers"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/searchsuggestion"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/spellcheck"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/useravatar"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/userfeedback"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
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
			"dp-chromeos-eng@google.com",
			"ramyagopalan@google.com",
			"shahinmd@google.com", // Test author.
		},
		BugComponent: "b:1129862",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
		VarDeps: []string{"policy.managedUserAccountPool",
			"ui.bond_credentials"},
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
			pci.SearchFlag(&policy.DefaultSearchProviderEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.DefaultSearchProviderEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.NearbyShareAllowed{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PasswordLeakDetectionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersDefinitionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersDefinitionEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.SafeBrowsingProtectionLevel{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.SearchSuggestEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SearchSuggestEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.SpellCheckServiceEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.UserAvatarCustomizationSelectorsEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.UserFeedbackAllowed{}, pci.VerifiedFunctionalityUI),
		},
		Timeout: 5 * time.Minute,
	})
}

// getPolicyList returns the list of policies to be set at the beginning of the test.
func getPolicyList() []policy.Policy {
	return []policy.Policy{
		&policy.CalendarIntegrationEnabled{Val: false},
		&policy.DefaultSearchProviderEnabled{Val: false},
		&policy.PasswordLeakDetectionEnabled{Val: false},
		&policy.QuickAnswersDefinitionEnabled{Val: false},
		&policy.QuickAnswersUnitConversionEnabled{Val: false},
		&policy.SearchSuggestEnabled{Val: false},
		&policy.SpellCheckServiceEnabled{Val: false},
		&policy.UserAvatarCustomizationSelectorsEnabled{Val: false},
		&policy.UserFeedbackAllowed{Val: false},
	}
}

type triggerOptionalService func(ctx context.Context, s *testing.State, cr *chrome.Chrome, br *browser.Browser, server *httptest.Server, tconn *chrome.TestConn, paramIndex int) error

type optionalService struct {
	name                  string
	associatedAnnotations []string
	trigger               triggerOptionalService
	paramIndex            int
}

func getOptionalServices() []optionalService {
	return []optionalService{
		{
			name:                  "calendar_integration",
			associatedAnnotations: []string{calendarintegration.AnnotationHashCode},
			trigger:               calendarintegration.TriggerCalendarIntegration,
			paramIndex:            0,
		},
		{
			name: "default_search_provider",
			// Annotation will be found even when the policy is disabled.
			associatedAnnotations: []string{},
			trigger:               defaultsearchprovider.TriggerDefaultSearchProvider,
			paramIndex:            0,
		},
		{
			name: "nearby_share",
			// No network annotations are checked for this service, since the network
			// calls only occur after Nearby Share setup is complete.
			associatedAnnotations: []string{},
			trigger:               nearbyshare.VerifyNearbySharePermissions,
			paramIndex:            0,
		},
		{
			name:                  "password_leak_detection",
			associatedAnnotations: []string{passwordleakdetection.AnnotationHashCode},
			trigger:               passwordleakdetection.TriggerPasswordLeakDetection,
			paramIndex:            0,
		},
		{
			name:                  "quick_answers_definition",
			associatedAnnotations: []string{policyquickanswers.AnnotationHashCode},
			trigger:               policyquickanswers.TriggerQuickAnswersDefinition,
			paramIndex:            0,
		},
		{
			name:                  "quick_answers_unit_conversion",
			associatedAnnotations: []string{policyquickanswers.AnnotationHashCode},
			trigger:               policyquickanswers.TriggerQuickAnswersUnitConversion,
			paramIndex:            0,
		},
		{
			name:                  "search_suggestion",
			associatedAnnotations: []string{searchsuggestion.AnnotationHashCode},
			trigger:               searchsuggestion.TriggerSearchSuggestion,
			paramIndex:            0,
		},
		{
			name:                  "spell_check",
			associatedAnnotations: []string{spellcheck.AnnotationHashCode},
			trigger:               spellcheck.TriggerSpellCheck,
			paramIndex:            0,
		},
		{
			name:                  "user_feedback",
			associatedAnnotations: []string{userfeedback.HelpContentProviderHashCode, userfeedback.ChromeFeedbackReportAppHashCode},
			trigger:               userfeedback.TriggerUserFeedback,
			paramIndex:            0,
		},
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
		chrome.DMSPolicy(fdms.URL),        // FakeDMS for setting policies.
		chrome.GAIALogin(gaiaCreds),       // Some of the optional service tests need a real GAIA account.
		chrome.ExtraArgs("--log-net-log"), // Enable netlog on startup.
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

	// Trigger the optional services one by one.
	for _, service := range getOptionalServices() {
		s.Run(ctx, service.name, func(ctx context.Context, s *testing.State) {
			if err := service.trigger(ctx, s, cr, br, server, tconn, service.paramIndex); err != nil {
				s.Fatalf("Failed to trigger %v: %v", service.name, err)
			}
			hashCodes = append(hashCodes, service.associatedAnnotations...)
		})
	}

	// Stop logging and verify network traffic annotations associated with the
	// optional services are not found in the logs.
	_, err = annotations.StopLoggingVerifyAnnotationSet(ctx, cr, br, false, hashCodes)
	if err != nil {
		s.Fatal("Failed to stop logging and verify logs: ", err)
	}

	// Note: In lacros mode, for unknown reasons, we are unable to stop the
	// browser network logging after we run this logic. We should ensure this
	// runs after the stop logging call.
	s.Run(ctx, "user_avatar_customization", func(ctx context.Context, s *testing.State) {
		userAvatarCustomizationParam := useravatar.CustomizationTestCase{
			Name:                      "disabled",
			ShouldFindAnnotation:      false,
			ShouldFindCustomSelectors: false,
			Policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Val: false},
		}
		if err := useravatar.TriggerUserAvatarCustomization(ctx, userAvatarCustomizationParam, tconn); err != nil {
			s.Fatal("Failed to trigger user avatar customization: ", err)
		}
	})

	// Check annotations that are only present in the netlog created on startup.
	// As of now, we only have one annotation where this is necessary. If we add
	// more annotations in the future, we should consider refactoring this test.
	foundAnnotation, err := annotations.CheckLogsFromFile(ctx, cr, useravatar.AnnotationHashCode, annotations.UserDirNetLogFile)
	if err != nil {
		s.Fatalf("Failed to check logs for %s: %v", annotations.UserDirNetLogFile, err)
	}
	if foundAnnotation {
		s.Fatalf("Annotation %s should not have been found", useravatar.AnnotationHashCode)
	}
}
