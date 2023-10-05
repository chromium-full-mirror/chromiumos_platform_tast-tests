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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/advancedprotection"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/autofillpayments"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/calendarintegration"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/defaultsearchprovider"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/domainreliability"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/passwordleakdetection"
	policyquickanswers "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/quickanswers"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/remotedesktop"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/searchsuggestion"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/spellcheck"
	ukm "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/urlkeydatacollection"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/useravatar"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/userfeedback"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/wallpapergooglephotos"
	webrtc "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/webrtclogupload"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/netexport"
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
			Fixture: fixture.FakeDMSEnrolled,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.PersistentLacrosEnrolled, // FakeDMSEnrolled with lacros policy.
			Val:               browser.TypeLacros,
		}},
		Data: dataFiles(),
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AdvancedProtectionAllowed{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.AutofillAddressEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.AutofillCreditCardEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DefaultSearchProviderEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.DefaultSearchProviderEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DomainReliabilityAllowed{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.NearbyShareAllowed{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PasswordLeakDetectionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PasswordManagerEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersDefinitionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersDefinitionEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.QuickAnswersUnitConversionEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.RemoteAccessHostAllowRemoteSupportConnections{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SafeBrowsingProtectionLevel{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.SearchSuggestEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SearchSuggestEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.SpellCheckServiceEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SyncTypesListDisabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.UrlKeyedAnonymizedDataCollectionEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.UserAvatarCustomizationSelectorsEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.UserFeedbackAllowed{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.WallpaperGooglePhotosIntegrationEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.WebRtcEventLogCollectionAllowed{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.WebRtcTextLogCollectionAllowed{}, pci.VerifiedValue),
		},
		Timeout: 10 * time.Minute,
	})
}

type triggerOptionalService func(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) error

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

// optionalServices returns the optionalService objects with policies set according
// to the policySetting parameter.
func optionalServices(policySetting networkrequestmonitor.PolicySetting) []optionalService {
	return []optionalService{
		{
			name:                  "advanced_protection",
			associatedAnnotations: []string{advancedprotection.UploadAnnotationHashCode},
			policies:              []policy.Policy{advancedprotection.TestCases()[policySetting].Policy},
			trigger:               advancedprotection.TriggerUploadForScanning,
		},
		{
			name:                  "autofill_payments",
			associatedAnnotations: []string{autofillpayments.AutofillCreditCardAnnotationHash},
			policies: []policy.Policy{
				autofillpayments.GetTestCases()[policySetting].Policy,
				&policy.AutofillAddressEnabled{Val: false},
				&policy.PasswordManagerEnabled{Val: false},
			},
			trigger: autofillpayments.TriggerAutofillCreditCardEnabled,
		},
		{
			name:                  "calendar_integration",
			associatedAnnotations: []string{calendarintegration.AnnotationHashCode},
			policies:              []policy.Policy{calendarintegration.TestCases()[policySetting].Policy},
			trigger:               calendarintegration.TriggerCalendarIntegration,
		},
		{
			name: "default_search_provider",
			// Annotation will be found even when the policy is disabled.
			associatedAnnotations: []string{},
			policies:              []policy.Policy{defaultsearchprovider.TestCases()[policySetting].Policy},
			trigger:               defaultsearchprovider.TriggerDefaultSearchProvider,
		},
		{
			name:                  "domain_reliability",
			associatedAnnotations: []string{},
			policies:              []policy.Policy{domainreliability.TestCases()[policySetting].Policy},
			trigger:               domainreliability.TriggerDomainReliabilityAllowed,
		},
		{
			name: "nearby_share",
			// No network annotations are checked for this service, since the network
			// calls only occur after Nearby Share setup is complete.
			associatedAnnotations: []string{},
			policies:              []policy.Policy{nearbyshare.TestCases()[policySetting].Policy},
			trigger:               nearbyshare.VerifyNearbySharePermissions,
		},
		{
			name:                  "password_leak_detection",
			associatedAnnotations: []string{passwordleakdetection.AnnotationHashCode},
			policies:              []policy.Policy{passwordleakdetection.TestCases()[policySetting].Policy},
			trigger:               passwordleakdetection.TriggerPasswordLeakDetection,
		},
		{
			name:                  "quick_answers_definition",
			associatedAnnotations: []string{policyquickanswers.AnnotationHashCode},
			policies:              []policy.Policy{policyquickanswers.DefinitionTestCases()[policySetting].Policy},
			trigger:               policyquickanswers.TriggerQuickAnswersDefinition,
		},
		{
			name:                  "quick_answers_unit_conversion",
			associatedAnnotations: []string{policyquickanswers.AnnotationHashCode},
			policies:              []policy.Policy{policyquickanswers.UnitConversionTestCases()[policySetting].Policy},
			trigger:               policyquickanswers.TriggerQuickAnswersUnitConversion,
		},
		{
			name: "remote_desktop",
			associatedAnnotations: []string{
				remotedesktop.FTLMessagingClientReceiveMessagesHashCode,
				remotedesktop.FTLRegistrationManagerHashCode,
				remotedesktop.RemotingRegisterSupportHostRequestHashCode,
			},
			policies: []policy.Policy{remotedesktop.TestCases()[policySetting].Policy},
			trigger:  remotedesktop.TriggerRemoteSupportRegistration,
		},
		{
			name:                  "search_suggestion",
			associatedAnnotations: []string{searchsuggestion.AnnotationHashCode},
			policies:              []policy.Policy{searchsuggestion.TestCases()[policySetting].Policy},
			trigger:               searchsuggestion.TriggerSearchSuggestion,
		},
		{
			name:                  "spell_check",
			associatedAnnotations: []string{spellcheck.AnnotationHashCode},
			policies:              []policy.Policy{spellcheck.TestCases()[policySetting].Policy},
			trigger:               spellcheck.TriggerSpellCheck,
		},
		{
			name:                  "url_keyed_data_collection",
			associatedAnnotations: []string{ukm.UkmNetworkAnnotationID},
			policies: []policy.Policy{
				&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: false},
				// TODO(b/293876410): Reenable App Sync once AppKM traffic annotations are split off or can be ignored.
				&policy.SyncTypesListDisabled{Val: []string{"apps"}},
			},
			trigger: ukm.TriggerAndVerifyUkmAppFromPolicySetting,
		},
		{
			name:                  "user_feedback",
			associatedAnnotations: []string{userfeedback.HelpContentProviderHashCode, userfeedback.ChromeFeedbackReportAppHashCode},
			policies:              []policy.Policy{userfeedback.TestCases()[policySetting].Policy},
			trigger:               userfeedback.TriggerUserFeedback,
		},
		{
			name: "wallpaper_google_photos",
			associatedAnnotations: []string{
				wallpapergooglephotos.EnabledHashCode,
				wallpapergooglephotos.AlbumsHashCode,
				wallpapergooglephotos.PhotosHashCode,
			},
			policies: []policy.Policy{wallpapergooglephotos.TestCases()[policySetting].Policy},
			trigger:  wallpapergooglephotos.TriggerWallpaperGooglePhotosIntegration,
		},
		{
			name: "webrtc_event_and_text_log_collection",
			associatedAnnotations: []string{
				webrtc.EventLogCollectionHashID,
				webrtc.TextLogCollectionHashID},
			policies: webrtc.TestCases()[policySetting].Policies,
			trigger:  webrtc.TriggerWebRTCLogUploads,
		},
		// Note: user_avatar_customization should be kept last in this list to avoid
		// issues with other test cases.
		{
			name:                  "user_avatar_customization",
			associatedAnnotations: []string{useravatar.AnnotationHashCode},
			policies:              []policy.Policy{useravatar.TestCases()[policySetting].Policy},
			trigger:               useravatar.TriggerUserAvatarCustomization,
		},
	}
}

// dataFiles returns the list of data files to be copied to the DUT and made
// available while the test is running.
func dataFiles() []string {
	dataFileLists := [][]string{
		advancedprotection.DataFiles(),
		autofillpayments.DataFiles(),
		passwordleakdetection.DataFiles(),
		policyquickanswers.DataFiles(),
		spellcheck.DataFiles(),
	}

	var dataFiles []string
	for _, serviceFiles := range dataFileLists {
		dataFiles = append(dataFiles, serviceFiles...)
	}
	return dataFiles
}

func NetworkRequestMonitor(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

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
		chrome.DMSPolicy(fdms.URL),                           // FakeDMS for setting policies.
		chrome.GAIALogin(gaiaCreds),                          // Some of the optional service tests need a real GAIA account.
		chrome.ExtraArgs("--metrics-upload-interval=1"),      // Reduce upload interval for UKM.
		chrome.ExtraArgs("--force-devtools-available"),       // Enable developer tools for extensions.
		chrome.KeepEnrollment(),                              // Required when restarting Chrome for device policy tests.
		chrome.LacrosExtraArgs("--force-devtools-available"), // Enable developer tools for extensions.
	}

	browserType := s.Param().(browser.Type)
	// Add args to start net export on startup.
	opts = append(opts, netexport.CommandLineArgs(browserType)...)

	// If browser type is lacros, handle differently.
	if browserType == browser.TypeLacros {
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
	services := optionalServices(networkrequestmonitor.PolicyDisabled)
	var policies []policy.Policy
	for _, service := range services {
		policies = append(policies, service.policies...)
	}
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

	// Restart Chrome to trigger domain reliability setup.
	// Reset tconn for new Chrome.
	cr, err = chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Setup the browser and get lacros object after the policy was set.
	var br *browser.Browser
	var lacrosSession *lacros.Lacros
	switch browserType {
	case browser.TypeLacros:
		lacrosSession, err = lacros.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch lacros-chrome: ", err)
		}
		br = lacrosSession.Browser()
		defer lacrosSession.Close(cleanupCtx)
	case browser.TypeAsh:
		br = cr.Browser()
	default:
		s.Fatalf("Unrecognized browser type %s", browserType)
	}

	// Setup credential for a test bond user to join Meet call.
	webrtc.SetBondCredentials(s.RequiredVar("ui.bond_credentials"))

	// Setup connection source for webrtc Meet operations.
	if browserType == browser.TypeLacros {
		webrtc.SetConnSource(lacrosSession)
	} else {
		webrtc.SetConnSource(cr)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_network_request_monitor")

	// Network traffic annotation hashcodes associated with the optional services.
	var hashCodes []string

	// Trigger the optional services one by one.
	for _, service := range services {
		s.Run(ctx, service.name, func(ctx context.Context, s *testing.State) {
			params := networkrequestmonitor.OptionalServiceParams{
				Chrome:        cr,
				Browser:       br,
				Server:        server,
				PolicySetting: networkrequestmonitor.PolicyDisabled}
			if err := service.trigger(ctx, params); err != nil {
				s.Fatalf("Failed to trigger %v: %v", service.name, err)
			}
			hashCodes = append(hashCodes, service.associatedAnnotations...)
		})
	}

	// Get net export session.
	netExport, err := netexport.FromCommandLineArg(browserType)
	if err != nil {
		s.Fatal("Failed to get net export session: ", err)
	}

	// Verify network traffic annotations associated with the optional services
	// are not found in the logs.
	foundAnnotations, err := netExport.FindMultipleAnnotationsUntil(ctx, hashCodes,
		&testing.PollOptions{Timeout: 80 * time.Second, Interval: 10 * time.Second})
	if err != nil {
		s.Fatal("Failed to poll hashcode in log: ", err)
	}

	for _, annotationID := range hashCodes {
		if _, exists := foundAnnotations[annotationID]; exists {
			s.Error("Found unexpected annotation = ", foundAnnotations)
		}
	}
}
