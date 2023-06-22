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
	"go.chromium.org/tast-tests/cros/local/input"
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

type triggerOptionalService func(ctx context.Context, s *testing.State, cr *chrome.Chrome, br *browser.Browser, server *httptest.Server, tconn *chrome.TestConn, paramIndex int) error

type optionalService struct {
	// name of the optional service.
	name string
	// Hashcodes of annotations associated with the service.
	associatedAnnotations []string
	// policies associated with this service. Those policies will be set to the
	// specified value before triggering the optional services.
	policies []policy.Policy
	// The function which triggers the optional service.
	trigger triggerOptionalService
}

func optionalServices() []optionalService {
	return []optionalService{
		{
			name:                  "calendar_integration",
			associatedAnnotations: []string{calendarintegration.AnnotationHashCode},
			policies:              []policy.Policy{&policy.CalendarIntegrationEnabled{Val: false}},
			trigger:               calendarintegration.TriggerCalendarIntegration,
		},
		{
			name: "default_search_provider",
			// Annotation will be found even when the policy is disabled.
			associatedAnnotations: []string{},
			policies:              []policy.Policy{&policy.DefaultSearchProviderEnabled{Val: false}},
			trigger:               defaultsearchprovider.TriggerDefaultSearchProvider,
		},
		{
			name: "nearby_share",
			// No network annotations are checked for this service, since the network
			// calls only occur after Nearby Share setup is complete.
			associatedAnnotations: []string{},
			policies:              []policy.Policy{&policy.NearbyShareAllowed{Val: false}},
			trigger:               nearbyshare.VerifyNearbySharePermissions,
		},
		{
			name:                  "password_leak_detection",
			associatedAnnotations: []string{passwordleakdetection.AnnotationHashCode},
			policies:              []policy.Policy{&policy.PasswordLeakDetectionEnabled{Val: false}},
			trigger:               passwordleakdetection.TriggerPasswordLeakDetection,
		},
		{
			name:                  "quick_answers_definition",
			associatedAnnotations: []string{policyquickanswers.AnnotationHashCode},
			policies:              []policy.Policy{&policy.QuickAnswersDefinitionEnabled{Val: false}},
			trigger:               policyquickanswers.TriggerQuickAnswersDefinition,
		},
		{
			name:                  "quick_answers_unit_conversion",
			associatedAnnotations: []string{policyquickanswers.AnnotationHashCode},
			policies:              []policy.Policy{&policy.QuickAnswersUnitConversionEnabled{Val: false}},
			trigger:               policyquickanswers.TriggerQuickAnswersUnitConversion,
		},
		{
			name:                  "search_suggestion",
			associatedAnnotations: []string{searchsuggestion.AnnotationHashCode},
			policies:              []policy.Policy{&policy.SearchSuggestEnabled{Val: false}},
			trigger:               searchsuggestion.TriggerSearchSuggestion,
		},
		{
			name:                  "spell_check",
			associatedAnnotations: []string{spellcheck.AnnotationHashCode},
			policies:              []policy.Policy{&policy.SpellCheckServiceEnabled{Val: false}},
			trigger:               spellcheck.TriggerSpellCheck,
		},
		{
			name:                  "user_feedback",
			associatedAnnotations: []string{userfeedback.HelpContentProviderHashCode, userfeedback.ChromeFeedbackReportAppHashCode},
			policies:              []policy.Policy{&policy.UserFeedbackAllowed{Val: false}},
			trigger:               userfeedback.TriggerUserFeedback,
		},
	}
}

// concatPolicyLists concats the lists of policies associated with the optional
// services and returns a single list.
func concatPolicyLists() []policy.Policy {
	// In lacros mode, for unknown reasons, we are unable to stop the browser
	// network logging after we run user avatar customization logic. We need to
	// ensure this runs after the stop logging call. That is why the function
	// optionalServices() does not return user avatar customization service. We
	// will separately trigger this service after we stop browser network logging.
	// Adding the policy related to user avatar customization service as it is not
	// returned by the function optionalServices().
	policies := []policy.Policy{&policy.UserAvatarCustomizationSelectorsEnabled{Val: false}}
	for _, service := range optionalServices() {
		policies = append(policies, service.policies...)
	}
	return policies
}

func NetworkRequestMonitor(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	isLacros := s.Param().(browser.Type) == browser.TypeLacros

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up keyboard.
	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

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

	// Enable the pref that indicates the user has enabled the Quick Answers
	// services.
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
	policies := concatPolicyLists()
	policyBlob.AddPolicies(policies)
	// Updates policies in Chrome by updating the the policy blob of FakeDMS. This
	// allows using a custom PolicyUser for the policy blob instead of the default
	// one.
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

	// If running in lacros mode, also start an OS net-export session. Annotations
	// in the OS (Ash) binary will only be present in the OS net-export log file.
	if isLacros {
		if err := annotations.StartOSLogging(ctx, cr, br, kb); err != nil {
			s.Fatal("Failed to start logging: ", err)
		}
	}

	// Network traffic annotation hashcodes associated with the optional services.
	var hashCodes []string

	// Trigger the optional services one by one.
	for _, service := range optionalServices() {
		s.Run(ctx, service.name, func(ctx context.Context, s *testing.State) {
			if err := service.trigger(ctx, s, cr, br, server, tconn, 0); err != nil {
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
	// Also check OS logs during lacros runs. Note that technically annotations
	// would either be present in the browser logs or the OS logs, depending on
	// which binary (Ash vs Lacros) they are part of. For now, we just check for
	// all annotations in both log files.
	if isLacros {
		if _, err := annotations.StopOSLoggingVerifyAnnotationSet(ctx, cr, br, kb, false, hashCodes); err != nil {
			s.Fatal("Failed to stop OS logging and verify logs: ", err)
		}
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
