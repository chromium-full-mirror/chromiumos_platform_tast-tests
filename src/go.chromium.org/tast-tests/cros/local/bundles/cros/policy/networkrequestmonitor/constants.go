// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package networkrequestmonitor contains values used across the umbrella test and its component subtests.
package networkrequestmonitor

import (
	"net/http/httptest"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/proxy"
)

// PolicySetting is the key for a test case of a service, indicating the policy
// value being tested and other expectations of the service behavior.
type PolicySetting int

const (
	// PolicyEnabled is for enabled service test cases.
	PolicyEnabled PolicySetting = iota

	// PolicyDisabled is for disabled service test cases.
	PolicyDisabled

	// PolicyUnset is for the default unset state of the service test case.
	PolicyUnset
)

// OptionalServiceParams is a struct to define params to trigger optional services
type OptionalServiceParams struct {
	Chrome        *chrome.Chrome
	Browser       *browser.Browser
	Server        *httptest.Server
	BaseDirectory string
	PolicySetting PolicySetting
	Proxy         proxy.Proxy
}

// VariantName is type for the variants of the NetworkRequestMonitor test.
type VariantName int

const (
	// Umbrella is for the umbrella test with the following configuration:
	//  - Optional services are disabled.
	//  - The test asserts that none of the annotations specified in the
	//    individual subtests are sent.
	Umbrella VariantName = iota

	// AnnotationsDiff is for the annotations diff test with the following configuration:
	//  - Optional services are disabled.
	//  - The test asserts that all annotations sent throughout the test are
	//    present in the AnnotationsDiffAllowlist.
	AnnotationsDiff

	// AnnotationsDiffEnabled is for the annotations diff test with optional
	// services enabled.
	//  - Optional services are enabled.
	//  - The test asserts that all annotations sent throughout the test are
	//    present in the AnnotationsDiffAllowlist, or else specified in the
	//    subtests being run.
	AnnotationsDiffEnabled
)

// TestVariant defines the variants of the NetworkRequestMonitor test.
type TestVariant struct {
	// Variant specifies which alternative of this test is being run (umbrella vs annotationsdiff).
	Variant VariantName
	// BrowserType specifies which browser to use for this test.
	BrowserType browser.Type
	// PolicyStatus specified how the policies should be set before the test is run.
	PolicyStatus PolicySetting
	// ExcludeServices specifies service keys to skip for this test.
	ExcludeServices []string
}

// AnnotationsDiffAllowlist is the set of network traffic annotations expected
// to fire for a full run through the annotations_diff* variants of the
// NetworkRequestMonitor test with all services disabled.
// The allowlist for enabled services is the union of this set with the expected
// annotations from each service subtest.
var AnnotationsDiffAllowlist = map[string]struct{}{
	// accounts_image_fetcher
	"98658519": {},
	// affiliation_lookup_by_hash
	"57748571": {},
	// autofill_query
	// TODO(b/292119918): Add Autofill Address policy to NetworkRequestMonitor test
	"88863520": {},
	// blink_resource_loader
	"101845102": {},
	// chrome_commerce_waa_fetcher
	"63068596": {},
	// chrome_variations_service
	"115188287": {},
	// client_download_request
	"125522256": {},
	// customization_document
	"117635540": {},
	// device_management_service
	"117782019": {},
	// domain_reliability_report_upload
	// TODO(b/302726313): check domain_reliability_report_upload in NetworkRequestMonitor umbrella test
	"108804096": {},
	// Webstore policies exempted from uber-switch, so these remain enabled
	// on NetworkRequestMonitor test variants. (Reference: b/282010592)
	// extension_crx_fetcher
	"21145003": {},
	// extension_manifest_fetcher
	"5151071": {},
	// favicon_loader
	"112189210": {},
	// gaia_auth_check_connection_info
	"4598626": {},
	// gaia_auth_exchange_device_id
	"39877119": {},
	// gaia_auth_list_accounts
	"35565745": {},
	// gaia_auth_multilogin
	"31445884": {},
	// gaia_cookie_manager_external_cc_result
	"4300475": {},
	// gaia_oauth_client_get_account_capabilities
	"87437888": {},
	// gaia_oauth_client_get_token_info
	"32585152": {},
	// gaia_oauth_client_get_user_info
	"83476155": {},
	// gcm_checkin
	"65957842": {},
	// gcm_registration
	"61656965": {},
	// hintsfetcher_gethintsrequest
	"34557599": {},
	// ime_url_downloader
	"60291160": {},
	// logo_service
	"35473769": {},
	// metrics_report_ukm
	// TODO(b/302747744): UKM enabled option for annotations_diff_enabled variant of NetworkRequestMonitor
	"727478": {},
	// metrics_report_uma
	// TODO(b/302551394): Remote metrics_report_uma from annotations_diff allowlist
	"727528": {},
	// navigation_url_loader
	"63171670": {},
	// new_tab_page_handler
	"48673144": {},
	// oauth2_access_token_fetcher
	"27915688": {},
	// omnibox_documentsuggest
	"6055066": {},
	// omnibox_result_change
	"73107389": {},
	// omnibox_zerosuggest
	"7687691": {},
	// one_google_bar_service
	"78917933": {},
	// promo_service
	"67052219": {},
	// quick_answers_spellchecker
	"91066830": {},
	// ranker_url_fetcher
	"95682324": {},
	// reporting
	"109891200": {},
	// safe_browsing_realtime_url_lookup
	"119324658": {},
	// safe_browsing_v4_get_hash
	"8561691": {},
	// safe_browsing_v4_update
	"82509217": {},
	// sanitized_image_source
	"36944304": {},
	// service_worker_script_load
	"21498113": {},
	// signed_in_profile_avatar
	"108903331": {},
	// sync_http_bridge
	"57144960": {},
	// translate_url_fetcher
	"137116619": {},
	// trusted_vault_request
	"30516662": {},
	// undefined ie. missing annotation
	"45578882": {},
	// update_client
	"54845618": {},
	// wallpaper_backdrop_collection_names
	"88516899": {},
	// wallpaper_backdrop_images_info
	"43574161": {},
	// webstore_install_helper
	"25921771": {},
	// webstore_installer
	"18764319": {},
}
