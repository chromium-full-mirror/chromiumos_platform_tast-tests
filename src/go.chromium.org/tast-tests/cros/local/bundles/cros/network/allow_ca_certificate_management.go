// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	utils "go.chromium.org/tast-tests/cros/local/certpageutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/testing"
)

// userCaCertName is a name for the CA certificate provided by user, usually it is CN or OU in certificate.
const userCaCertName = "TEST_CA_ORG"

// userCaOrg is a name for the org which has issued users CA certificate provided by user, it is visible in the list of Authorities.
const userCaOrg = "org-TEST_CA_ORG"

// providedCaCertName is a name for the CA certificate provided by OS, usually it is CN or OU in certificate.
const providedCaCertName = "GTS Root R1"

// providedCaOrg is a name for the org which has issued CA certificate provided by OS, it is visible in the list of Authorities.
const providedCaOrg = "org-Google Trust Services LLC"

// policyProvidedCaCertName is a name for the CA certificate provided by policy, usually it is CN or OU in certificate.
const policyProvidedCaCertName = "root_ca_cert"

// policyProvidedCaOrg is a name for the org which has issued CA certificate provided by policy, it is visible in the list of Authorities.
const policyProvidedCaOrg = "org-root_ca_cert"

// caCertFile is a file's name for the root certificate that
// is used to create a client and website certificates.
// Chrome will need to import it to trust that the website certificate is valid.
// Website server will need to use it to trust that the client certificate is valid.
const caCertFile = "cert_settings_page_root_cert.crt"

// editMenuItem  is UI element finder for "Edit" menu item.
var editMenuItem = nodewith.Name("Edit").Role(role.MenuItem)

// viewMenuItem  is UI element finder for "View" menu item.
var viewMenuItem = nodewith.Name("View").Role(role.MenuItem)

// exportMenuItem  is UI element finder for "Export" menu item.
var exportMenuItem = nodewith.Name("Export").Role(role.MenuItem)

// deleteMenuItem  is UI element finder for "Delete" menu item.
var deleteMenuItem = nodewith.Name("Delete").Role(role.MenuItem)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AllowCACertificateManagement,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that policy can block user from managing CA certificates",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"olsa@google.com", // Test author
		},
		BugComponent: "b:1000044",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		Timeout:      5 * time.Minute,
		Data:         []string{caCertFile},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.CACertificateManagementAllowed{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.OpenNetworkConfiguration{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// verify that managed users either have or don't have the ability to manage and CA certificates based on the setting of the configured policy.
				// (COM_FOUND_CUJ4_TASK2_WF1).
				Value: "screenplay-7c74e36b-7675-4fa7-91ca-24577bb37203",
			},
		},
	})
}

// prepareCACertificate copy CA certificate which is required for tests to the Downloads.
func prepareCACertificate(s *testing.State, downloadsPath string) {
	const caCert = caCertFile
	if err := utils.CopyToDownloads(downloadsPath, s.DataPath(caCert), caCert); err != nil {
		s.Fatal("Failed to copy CA certificate file to Download: ", err)
	}
}

// cleanCACertificate removes certificate from the Downloads.
func cleanCACertificate(s *testing.State, downloadsPath string) {
	const caCert = caCertFile
	if err := utils.RemoveFromDownloads(downloadsPath, s.DataPath(caCert), caCert); err != nil {
		s.Fatal("Failed to delete certificate file from Download: ", err)
	}
}

// expectImportUserCACertSuccess imports CA certificate or fails.
func expectImportUserCACertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.ImportCACert(ctx, ui, caCertFile); err != nil {
		s.Fatal("Failed to import CA certificate: ", err)
	}
}

// isUIElementFound searching for provided UI element.
func isUIElementFound(ctx context.Context, ui *uiauto.Context, uiElement *nodewith.Finder) (result bool) {
	if err := ui.Exists(uiElement)(ctx); err == nil {
		return true
	}
	return false
}

// assertUIElementNotPresent confirms that provided UI element can no be found.
func assertUIElementNotPresent(ctx context.Context, s *testing.State, ui *uiauto.Context, uiElement *nodewith.Finder) {
	if isUIElementFound(ctx, ui, uiElement) != false {
		s.Error("Unexpected existence of UI element: ", uiElement)
	}
}

// assertUIElementPresent confirms that provided UI element can be found.
func assertUIElementPresent(ctx context.Context, s *testing.State, ui *uiauto.Context, uiElement *nodewith.Finder) {
	if isUIElementFound(ctx, ui, uiElement) != true {
		s.Error("Unexpected miss of UI element: ", uiElement)
	}
}

// expectImportUserCACertNotPossible checks that import of CA certificate is not possible
// because button "Import" is absent.
func expectImportUserCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := uiauto.Combine("Select CA certificates tab",
		ui.DoDefault(nodewith.Name("Authorities").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Authorities").ClassName("tab selected")),
	)(ctx); err != nil {
		s.Fatal("Failed to select CA certificates tab: ", err)
	}

	importButton := nodewith.Name("Import").Role(role.Button)
	assertUIElementNotPresent(ctx, s, ui, importButton)
}

// expectDeleteUserCACertSuccess selects and deletes user's CA certificate on CA tab.
func expectDeleteUserCACertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.DeleteCACert(ctx, ui, userCaCertName, userCaOrg); err != nil {
		s.Fatal("Failed to delete CA certificate: ", err)
	}
}

// expectDeleteUserCACertNotPossible checks that "Delete" button is not shown for user's CA certificate.
func expectDeleteUserCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.SelectCACertificate(ctx, ui, userCaOrg); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	if err := utils.OpenActionMenuForCACertificate(ctx, ui, userCaOrg); err != nil {
		s.Fatal("Failed to open action menu: ", err)
	}

	// Make sure that Delete buttons are not present in the popup menu, but other buttons are there.
	deleteItem := nodewith.Name("Delete").Role(role.MenuItem)
	viewItem := nodewith.Name("View").Role(role.MenuItem)
	exportItem := nodewith.Name("Export").Role(role.MenuItem)
	assertUIElementNotPresent(ctx, s, ui, deleteItem)
	assertUIElementPresent(ctx, s, ui, viewItem)
	assertUIElementPresent(ctx, s, ui, exportItem)

	// Close popup menu and previously selected CA org.
	utils.PressEscape(ctx)
	if err := utils.SelectCACertificate(ctx, ui, userCaOrg); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}
}

// expectEditTrustUserCACertSuccess testing that trust bit for the user's CA certificate can be turned off.
func expectEditTrustUserCACertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.SetCACertTrust(ctx, ui, checked.False /*targetState*/, userCaCertName, userCaOrg); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}
	// Return trust value back to original state.
	if err := utils.SetCACertTrust(ctx, ui, checked.True /*targetState*/, userCaCertName, userCaOrg); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}
}

// expectEditTrustProvidedCACertSuccess testing that trust bit for the provided CA certificate can be turned off.
func expectEditTrustProvidedCACertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.SetCACertTrust(ctx, ui, checked.False /*targetState*/, providedCaCertName, providedCaOrg); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}
	// Return trust value back to original state.
	if err := utils.SetCACertTrust(ctx, ui, checked.True /*targetState*/, providedCaCertName, providedCaOrg); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}
}

// expectManagePolicyProvidedCACertNotPossible testing that it is not possible to manage CA certificate provided by policy.
// It will select CA org, then it will select specific CA certificate and open action menu for it. Then
// it will check that "Edit" and "Delete" buttons are not shown while "View" and "Export" buttons are shown.
func expectManagePolicyProvidedCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context, caOrg, caCertName string) {
	if err := utils.SelectPolicyProvidedCACertificate(ctx, ui, caOrg); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	if err := utils.OpenActionMenuForPolicyProvidedCACertificate(ctx, ui, caCertName); err != nil {
		s.Fatal("Failed to open action menu for the certificate: ", err)
	}

	// Make sure that "Edit" and "Delete" menu items are not present in the popup menu, but other buttons are there.
	assertUIElementNotPresent(ctx, s, ui, editMenuItem)
	assertUIElementPresent(ctx, s, ui, viewMenuItem)
	assertUIElementPresent(ctx, s, ui, exportMenuItem)
	assertUIElementNotPresent(ctx, s, ui, deleteMenuItem)

	// Close popup menu and previously selected CA org.
	if err := utils.PressEscape(ctx); err != nil {
		s.Fatal("Failed to press Esc: ", err)
	}
	if err := utils.SelectPolicyProvidedCACertificate(ctx, ui, caOrg); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}
}

// expectManageCACertNotPossible testing that it is not possible to manage CA certificate.
// It will select CA org, then it will select specific CA certificate and open action menu for it. Then
// it will check that "Edit" and "Delete" buttons are not shown while "View" and "Export" buttons are shown.
func expectManageCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context, caOrg, caCertName string) {
	if err := utils.SelectCACertificate(ctx, ui, caOrg); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	if err := utils.OpenActionMenuForCACertificate(ctx, ui, caCertName); err != nil {
		s.Fatal("Failed to open action menu for the certificate: ", err)
	}

	// Make sure that "Edit" and "Delete" menu items are not present in the popup menu, but other buttons are there.
	assertUIElementNotPresent(ctx, s, ui, editMenuItem)
	assertUIElementPresent(ctx, s, ui, viewMenuItem)
	assertUIElementPresent(ctx, s, ui, exportMenuItem)
	assertUIElementNotPresent(ctx, s, ui, deleteMenuItem)

	// Close popup menu and previously selected CA org.
	if err := utils.PressEscape(ctx); err != nil {
		s.Fatal("Failed to press Esc: ", err)
	}
	if err := utils.SelectCACertificate(ctx, ui, caOrg); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}
}

// expectCACertNotImported checks that CA certificate's org is not present in the list of known orgs for CA certificates .
// We are checking only org and not checking exact certificates, because even org should not exist.
func expectCACertNotImported(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if status := utils.IsCACertOrgExists(ctx, ui, userCaOrg); status == true {
		s.Fatal("CA Org is already present in system")
	}
}

// AllowCACertificateManagement tests that user can or can not manage CA certificates
// based on the policy setting. Policy description is here https://chromeenterprise.google/policies/#ClientCertificateManagementAllowed
func AllowCACertificateManagement(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	kb, err := input.Keyboard(ctx)
	defer kb.Close(ctx)
	if err != nil {
		s.Fatal("Can not use keyboard: ", err)
	}
	ctx = context.WithValue(ctx, utils.KeyboardKey, kb)

	// Copy all required for test certificates to Download.
	prepareCACertificate(s, downloadsPath)
	defer cleanCACertificate(s, downloadsPath)

	commonPolicies := []policy.Policy{
		&policy.OpenNetworkConfiguration{
			Val: &policy.ONC{
				Certificates: []*policy.ONCCertificate{
					{
						GUID:      "{b3aae353-cfa9-4093-9aff-9f8ee2bf8c29}",
						TrustBits: []string{"Web"},
						Type:      "Authority",
						X509:      "-----BEGIN CERTIFICATE-----\nMIIDHzCCAgegAwIBAgIUKb0vi5cSMIah3JFznmml8NFDPSkwDQYJKoZIhvcNAQEL\nBQAwFzEVMBMGA1UEAwwMcm9vdF9jYV9jZXJ0MB4XDTE5MTIwNTEyNTMyM1oXDTI5\nMTIwMjEyNTMyM1owFzEVMBMGA1UEAwwMcm9vdF9jYV9jZXJ0MIIBIjANBgkqhkiG\n9w0BAQEFAAOCAQ8AMIIBCgKCAQEAt1JobSyZGOXzNARok+UMWTWJ0PEkXb7qWYGB\nv6eWuEBvUywCUyq8D29qzWGBc2JW3KdI5l8WRoQ2WPfo6+3MHVht13gzN0icAMTW\naQKedk+b6dcQZVESEPFHF8m47iEfQEsoF2RvlYIN/WQuYxAcf0SJFfsgq1A7St94\n0nO3gl5RNjLtFBpTIGyri/SmD1/EEyD3J2XFPGLtVYQH65c8m7kNDuHQawBvEAnv\nAlEsXxNUeqVg887UdkhG4N8i3ULvzI1QZX0WzugCrQX9XCG1w7txzmYKfIYPa2zP\nG7p+MjdEflahrXNCbLnnD7nALUJ3zgRRxZ3ZleUdSDv71bU2fwIDAQABo2MwYTAP\nBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRzXr2ldI6MxTGQB2CS2dcsV69MZzAf\nBgNVHSMEGDAWgBRzXr2ldI6MxTGQB2CS2dcsV69MZzAOBgNVHQ8BAf8EBAMCAQYw\nDQYJKoZIhvcNAQELBQADggEBALVxM5T4JYxv8X8vG/tNRpdStkQUFWSQVDuwjEVx\nbg3DMmR+OT8N4UGwgkzz/wC6VCiNKUStfjtu3vbA98qKykEpBI5G973JZdLNqZuz\nJJxsG1lsma+dHLMFJV8LCYQAjYMTlD9YgJezL0B5jbquOxCXSTbzSuyzIvjyMSZk\nQsxxNsKuGuvdXm8Nd2zOzIixibOsd3kYRAkqLTG5QBm0K6Bt+jdYkGrh8WbFIAZr\nNZ8WwHnPI0DAYwSNrCfx9ofBMoaWa3vxf64rO4+A/snJ4RTEN+Jj+F5anDgTnK9S\nhjk7IYiGv73aNhOZ5wQSsJQAEWdE/h6oeXR2T946XOJIcGI=\n-----END CERTIFICATE-----\n",
					},
				},
			},
		},
	}

	// Loop via different policy settings and check that they are working as expected.
	for _, param := range []struct {
		name                          string
		policies                      []policy.Policy
		canManageUserCACert           bool
		canEditTrustForProvidedCACert bool
	}{
		{
			name: "unset",
			policies: append(
				commonPolicies,
				&policy.CACertificateManagementAllowed{Stat: policy.StatusUnset},
			),
			canManageUserCACert:           true,
			canEditTrustForProvidedCACert: true,
		},
		{
			name: "all_allowed",
			policies: append(
				commonPolicies,
				&policy.CACertificateManagementAllowed{Val: 0},
			),
			canManageUserCACert:           true,
			canEditTrustForProvidedCACert: true,
		},
		{
			name: "only_user_allowed",
			policies: append(
				commonPolicies,
				&policy.CACertificateManagementAllowed{Val: 1},
			),
			canManageUserCACert:           true,
			canEditTrustForProvidedCACert: false,
		},
		{
			name: "not_allowed",
			policies: append(
				commonPolicies,
				&policy.CACertificateManagementAllowed{Val: 2},
			),
			canManageUserCACert:           false,
			canEditTrustForProvidedCACert: false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Import user's certificate which will be used for testing, if operations
			// with certificates are forbidden after the policy is applied.
			isCleanupCertRequired := false
			if !param.canManageUserCACert {
				// Opening a new tab in browser.
				conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
				if err != nil {
					s.Fatal("Failed to open a new tab in browser: ", err)
				}
				defer conn.Close()

				expectImportUserCACertSuccess(ctx, s, ui)
				isCleanupCertRequired = true
			}

			// Update policies.
			// Due to e.g. onc_normalizer.cc, a re-exported policy for ONC usually doesn't
			// match the provided policy, so ServerAndVerify would fail.
			// Use ServeAndRefresh instead.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, param.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Opening a new tab in browser on certificates page.
			conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
			if err != nil {
				s.Fatal("Failed to open a new tab in browser: ", err)
			}
			defer conn.Close()

			// CA certificates provided by policy can not be managed at all.
			expectManagePolicyProvidedCACertNotPossible(ctx, s, ui, policyProvidedCaOrg, policyProvidedCaCertName)

			if param.canManageUserCACert {
				expectCACertNotImported(ctx, s, ui)
				expectImportUserCACertSuccess(ctx, s, ui)
				expectEditTrustUserCACertSuccess(ctx, s, ui)
				expectDeleteUserCACertSuccess(ctx, s, ui)
			} else {
				expectImportUserCACertNotPossible(ctx, s, ui)
				expectManageCACertNotPossible(ctx, s, ui, userCaOrg, userCaCertName)
				expectDeleteUserCACertNotPossible(ctx, s, ui)
			}

			// TODO(b/291182593): Re-enable this when the new UI for modifying
			// trust on root certificates is implemented.
			// if param.canEditTrustForProvidedCACert {
			// 	expectEditTrustProvidedCACertSuccess(ctx, s, ui)
			// } else {
			// 	expectManageCACertNotPossible(ctx, s, ui, providedCaOrg, providedCaCertName)
			// }

			// Reset policy and delete cert if cert deletion was forbidden by policy during test.
			if isCleanupCertRequired {
				if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
					s.Fatal("Failed to clean up: ", err)
				}
				conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
				if err != nil {
					s.Fatal("Failed to open a new tab in browser: ", err)
				}
				defer conn.Close()
				expectDeleteUserCACertSuccess(ctx, s, ui)
			}
		})
	}
	// Make sure that cleanup done.
	conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
	if err != nil {
		s.Fatal("Failed to open a new tab in browser: ", err)
	}
	defer conn.Close()
	expectCACertNotImported(ctx, s, ui)
}
