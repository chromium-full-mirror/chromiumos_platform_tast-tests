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

// userCaOrgNewUI is a name for the org which has issued users CA certificate provided by user, in new UI, cert name will start with it.
const userCaOrgNewUI = "O=TEST_CA_ORG"

// policyProvidedCaCertName is a name for the CA certificate provided by policy, usually it is CN or OU in certificate.
const policyProvidedCaCertName = "root_ca_cert"

// policyProvidedCaOrg is a name for the org which has issued CA certificate provided by policy, it is visible in the list of Authorities.
const policyProvidedCaOrg = "org-root_ca_cert"

// caCertFile is a file's name for the root certificate that
// is used to create a client and website certificates.
// Chrome will need to import it to trust that the website certificate is valid.
// Website server will need to use it to trust that the client certificate is valid.
const caCertFile = "cert_settings_page_root_cert.pem"

// editMenuItem is a UI element finder for "Edit" menu item.
var editMenuItem = nodewith.Name("Edit").Role(role.MenuItem)

// viewMenuItem is a UI element finder for "View" menu item.
var viewMenuItem = nodewith.Name("View").Role(role.MenuItem)

// exportMenuItem is a UI element finder for "Export" menu item.
var exportMenuItem = nodewith.Name("Export").Role(role.MenuItem)

// deleteMenuItem is a UI element finder for "Delete" menu item.
var deleteMenuItem = nodewith.Name("Delete").Role(role.MenuItem)

// generalTab is a UI element finder for "General" tab in cert viewer.
var generalTab = nodewith.Name("General").Role(role.Tab)

// detailsTab is a UI element finder for "Details" tab in cert viewer.
var detailsTab = nodewith.Name("Details").Role(role.Tab)

// modificationTab is a UI element finder for "Modifications" tab in cert viewer.
var modificationTab = nodewith.Name("Modifications").Role(role.Tab)

func init() {
	testing.AddTest(&testing.Test{
		Func: AllowCACertificateManagement,
		Desc: "Test that policy can block user from managing CA certificates",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"olsa@google.com", // Test author
		},
		BugComponent: "b:1000044",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
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

// isNewUIActive returns the status of new UI usage in certificate manager.
func isNewUIActive(ctx context.Context) bool {
	return utils.IsNewUIActive(ctx)
}

// expectImportUserCACertSuccess imports CA certificate or fails.
func expectImportUserCACertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if isNewUIActive(ctx) {
		if err := utils.ImportCACertNewUI(ctx, ui, caCertFile); err != nil {
			s.Fatal("Failed to import CA certificate: ", err)
		}
		return
	}

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
	if isUIElementFound(ctx, ui, uiElement) {
		s.Error("Unexpected existence of UI element: ", uiElement)
	}
}

// assertUIElementPresent confirms that provided UI element can be found.
func assertUIElementPresent(ctx context.Context, s *testing.State, ui *uiauto.Context, uiElement *nodewith.Finder) {
	if !isUIElementFound(ctx, ui, uiElement) {
		s.Error("Unexpected miss of UI element: ", uiElement)
	}
}

// expectImportUserCACertNotPossible checks that import of CA certificate is not possible
// because button "Import" is absent.
func expectImportUserCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if isNewUIActive(ctx) {
		if err := utils.OpenUserInstalledCACertsNewUI(ctx, ui); err != nil {
			s.Fatal("Failed to open users CA certificates tab: ", err)
		}

		// There are 3 "Import" buttons, UI finder will look for any of them.
		importButton := nodewith.Name("Import").Role(role.Button)
		assertUIElementNotPresent(ctx, s, ui, importButton)
		return
	}

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
func expectDeleteUserCACertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn) {
	if isNewUIActive(ctx) {
		// This is testing that cert is present before it is deleted.
		if status := utils.IsCACertOrgExistsNewUI(ctx, ui, userCaOrgNewUI); !status {
			s.Fatal("CA Org is not present in system or IsCACertOrgExistsNewUI can not find the cert ")
		}

		if err := utils.DeleteCACertNewUI(ctx, ui, conn, userCaOrgNewUI, userCaCertName); err != nil {
			s.Fatal("Failed to delete CA certificate: ", err)
		}
		return
	}

	// This is testing that cert is present before it is deleted.
	if status := utils.IsCACertOrgExists(ctx, ui, userCaOrg); !status {
		s.Fatal("CA Org is not present in system or IsCACertOrgExists can not find the cert ")
	}

	if err := utils.DeleteCACert(ctx, ui, conn, userCaOrg, userCaCertName); err != nil {
		s.Fatal("Failed to delete CA certificate: ", err)
	}
}

// expectDeleteUserCACertNotPossibleNewUI checks that "Delete" button is not shown for the user's CA certificate.
func expectDeleteUserCACertNotPossibleNewUI(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn) {
	if err := utils.OpenUserInstalledCACertsNewUI(ctx, ui); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	// Make sure that Delete buttons are not present, but other buttons are there.
	deleteNamePrefix := "Delete certificate " + userCaOrgNewUI
	viewNamePrefix := "View certificate details for " + userCaOrgNewUI
	exportName := "Export all Trusted Certificates"
	deleteItem := nodewith.NameStartingWith(deleteNamePrefix).Role(role.Button)
	viewItem := nodewith.NameStartingWith(viewNamePrefix).Role(role.Button)
	exportItem := nodewith.Name(exportName).Role(role.Button)

	assertUIElementNotPresent(ctx, s, ui, deleteItem)
	assertUIElementPresent(ctx, s, ui, viewItem)
	assertUIElementPresent(ctx, s, ui, exportItem)
}

// expectDeleteUserCACertNotPossible checks that "Delete" button is not shown for user's CA certificate.
func expectDeleteUserCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn) {
	if isNewUIActive(ctx) {
		expectDeleteUserCACertNotPossibleNewUI(ctx, s, ui, conn)
		return
	}

	if err := utils.SelectCACertificate(ctx, ui, conn, userCaOrg, userCaCertName); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	if err := utils.OpenActionMenuForCACertificate(ctx, ui, conn, userCaCertName); err != nil {
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
	if err := utils.SelectCACertificate(ctx, ui, conn, userCaOrg, userCaCertName); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}
}

// expectEditTrustUserCACertSuccessNewUI tests that trust settings for the user's CA certificate can be changed.
func expectEditTrustUserCACertSuccessNewUI(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn) {
	if err := utils.SetCACertTrustNewUI(ctx, ui, conn, false /*targetState*/, userCaOrgNewUI); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}

	caCertOrgText := nodewith.NameStartingWith(userCaOrgNewUI).Role(role.StaticText)
	distrustedCertContainer := nodewith.ClassName("collapse-opened").Role(role.Group).Nth(2)
	distrustedCert := nodewith.NameStartingWith(userCaOrgNewUI).Role(role.StaticText).Ancestor(distrustedCertContainer)
	if err := uiauto.Combine("Check that the cert included into distrusted list",
		ui.WaitUntilExists(caCertOrgText),
		ui.WaitUntilExists(distrustedCert),
	)(ctx); err != nil {
		s.Fatal("Failed to find the cert in distrusted list: ", err)
	}

	if err := utils.SetCACertTrustNewUI(ctx, ui, conn, true /*targetState*/, userCaOrgNewUI); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}

	trustedCertContainer := nodewith.ClassName("collapse-opened").Role(role.Group).Nth(0)
	trustedCert := nodewith.NameStartingWith(userCaOrgNewUI).Role(role.StaticText).Ancestor(trustedCertContainer)
	if err := uiauto.Combine("Check that the cert included into trusted list",
		ui.WaitUntilExists(caCertOrgText),
		ui.WaitUntilExists(trustedCert),
	)(ctx); err != nil {
		s.Fatal("Failed to find the cert in trusted list: ", err)
	}
}

// expectEditTrustUserCACertSuccess tests that trust bit for the user's CA certificate can be turned off.
func expectEditTrustUserCACertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn) {
	if isNewUIActive(ctx) {
		expectEditTrustUserCACertSuccessNewUI(ctx, s, ui, conn)
		return
	}

	if err := utils.SetCACertTrust(ctx, ui, conn, checked.False /*targetState*/, userCaOrg, userCaCertName); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}
	// Return trust value back to original state.
	if err := utils.SetCACertTrust(ctx, ui, conn, checked.True /*targetState*/, userCaOrg, userCaCertName); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}
}

// expectManagePolicyProvidedCACertNotPossibleNewUI tests that it is not possible to manage CA certificate provided by policy.
// It will select specific CA certificate and open view tab for it. Then
// it will check that "General" and "Details" tabs are shown while "Modification" tab is not shown.
func expectManagePolicyProvidedCACertNotPossibleNewUI(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn, caOrg, caCertName string) {
	if err := utils.OpenAdminInstalledCACertsNewUI(ctx, ui); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	// Make sure that Delete buttons are not present.
	deleteNamePrefix := "Delete certificate " + userCaOrgNewUI
	deleteItem := nodewith.NameStartingWith(deleteNamePrefix).Role(role.Button)
	assertUIElementNotPresent(ctx, s, ui, deleteItem)

	if err := utils.OpenCertificateViewNewUI(ctx, ui, caCertName); err != nil {
		s.Fatal("Failed to open View menu for the certificate: ", err)
	}

	// Make sure that "General" and "Details" tabs items are present in the popup,
	// but "Modification" is not there.
	assertUIElementPresent(ctx, s, ui, generalTab)
	assertUIElementPresent(ctx, s, ui, detailsTab)
	assertUIElementNotPresent(ctx, s, ui, modificationTab)

	// Close popup menu.
	if err := utils.PressEscape(ctx); err != nil {
		s.Fatal("Failed to close popup with Esc: ", err)
	}
	return

}

// expectManagePolicyProvidedCACertNotPossible tests that it is not possible to manage CA certificate provided by policy.
// It will select CA org, then it will select specific CA certificate and open action menu for it. Then
// it will check that "Edit" and "Delete" buttons are not shown while "View" and "Export" buttons are shown.
func expectManagePolicyProvidedCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn, caOrg, caCertName string) {
	if isNewUIActive(ctx) {
		expectManagePolicyProvidedCACertNotPossibleNewUI(ctx, s, ui, conn, caOrg, caCertName)
		return
	}
	if err := utils.SelectPolicyProvidedCACertificate(ctx, ui, conn, caOrg, caCertName); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	if err := utils.OpenActionMenuForPolicyProvidedCACertificate(ctx, ui, conn, caCertName); err != nil {
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
	if err := utils.SelectPolicyProvidedCACertificate(ctx, ui, conn, caOrg, caCertName); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}
}

// expectManageUserCACertNotPossibleNewUI tests that it is not possible to manage CA certificate.
// It will select specific CA certificate and open view popup for it. Then
// it will check that "General", "Details" and "Modification" tabs are shown.
// Then it will try to change "Trusted" value to "Distrusted" and check that
// it will not become visible. The field with trust value is disabled and all
// dropdown values are invisible. It is still possible to change visibility using developer
// tool, but it will fail during Save. Unfortunately error message
// has no node and can not be checked, so it tests that all values are still not
// visible and also it tests that cert stays in the "Trusted" list after changes.
func expectManageUserCACertNotPossibleNewUI(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn, caOrg string) {
	if err := utils.OpenUserInstalledCACertsNewUI(ctx, ui); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	if err := utils.OpenCertificateViewNewUI(ctx, ui, caOrg); err != nil {
		s.Fatal("Failed to open View menu for the certificate: ", err)
	}

	// Make sure that "General", "Modification" and "Details" tabs are present.
	assertUIElementPresent(ctx, s, ui, generalTab)
	assertUIElementPresent(ctx, s, ui, detailsTab)
	assertUIElementPresent(ctx, s, ui, modificationTab)

	if err := uiauto.Combine("Check that modify trust setting is deactivated",
		ui.WaitUntilExists(nodewith.Name("Modifications").Role(role.Tab)),
		ui.DoDefault(nodewith.Name("Modifications").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Trust State").Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name("Distrusted").Role(role.MenuListOption).Invisible()),
		ui.WaitUntilExists(nodewith.Name("Trusted").Role(role.MenuListOption).Invisible()),
		ui.WaitUntilExists(nodewith.Name("Hint").Role(role.MenuListOption).Invisible()),
		// Attempt to select "Distrusted" should not change state - all nodes still
		// invisible. There will be also error in UI, but it has no node/element.
		ui.MakeVisible(nodewith.Name("Distrusted").Role(role.MenuListOption)),
		ui.DoDefault(nodewith.Name("Distrusted").Role(role.MenuListOption)),
		ui.WaitUntilExists(nodewith.Name("Distrusted").Role(role.MenuListOption).Invisible()),
		ui.WaitUntilExists(nodewith.Name("Trusted").Role(role.MenuListOption).Invisible()),
		ui.WaitUntilExists(nodewith.Name("Hint").Role(role.MenuListOption).Invisible()),
	)(ctx); err != nil {
		s.Fatal("Failed to check that trust is deactivated: ", err)
	}

	// Close popup for cert viewer.
	if err := utils.PressEscape(ctx); err != nil {
		s.Fatal("Failed to close popup Esc: ", err)
	}

	// Check that the cert still in "Trusted list".
	caCertOrgText := nodewith.NameStartingWith(userCaOrgNewUI).Role(role.StaticText)
	trustedCertContainer := nodewith.ClassName("collapse-opened").Role(role.Group).Nth(0)
	trustedCert := nodewith.NameStartingWith(caOrg).Role(role.StaticText).Ancestor(trustedCertContainer)
	if err := uiauto.Combine("Check that the cert included into trusted list",
		ui.WaitUntilExists(caCertOrgText),
		ui.WaitUntilExists(trustedCert),
	)(ctx); err != nil {
		s.Fatal("Failed to find the cert in trusted list: ", err)
	}
}

// expectManageCACertNotPossible tests that it is not possible to manage CA certificate.
// It will select CA org, then it will select specific CA certificate and open action menu for it. Then
// it will check that "Edit" and "Delete" buttons are not shown while "View" and "Export" buttons are shown.
func expectManageCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context, conn *chrome.Conn, caOrg, caCertName string) {
	if isNewUIActive(ctx) {
		expectManageUserCACertNotPossibleNewUI(ctx, s, ui, conn, userCaOrgNewUI)
		return
	}

	if err := utils.SelectCACertificate(ctx, ui, conn, caOrg, caCertName); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}

	if err := utils.OpenActionMenuForCACertificate(ctx, ui, conn, caCertName); err != nil {
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
	if err := utils.SelectCACertificate(ctx, ui, conn, caOrg, caCertName); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}
}

// expectCACertNotImported checks that CA certificate's org is not present in the list of known orgs for CA certificates .
// We are checking only org and not checking exact certificates, because even org should not exist.
func expectCACertNotImported(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if isNewUIActive(ctx) {
		if status := utils.IsCACertOrgExistsNewUI(ctx, ui, userCaOrgNewUI); status {
			s.Fatal("CA Org is already present in system")
		}
		return
	}
	if status := utils.IsCACertOrgExists(ctx, ui, userCaOrg); status {
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
	if err != nil {
		s.Fatal("Can not use keyboard: ", err)
	}
	defer kb.Close(ctx)
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

			// Check if new UI for the certificate manager is used or the old UI.
			isNewUIPresent := false
			{
				// Opening a new tab in browser.
				conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
				if err != nil {
					s.Fatal("Failed to open a new tab in browser: ", err)
				}
				defer conn.Close()
				isNewUIPresent = utils.IsNewUIUsed(ctx, ui)
			}
			ctx = context.WithValue(ctx, utils.IsNewUI, isNewUIPresent)

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
			expectManagePolicyProvidedCACertNotPossible(ctx, s, ui, conn, policyProvidedCaOrg, policyProvidedCaCertName)

			if param.canManageUserCACert {
				expectCACertNotImported(ctx, s, ui)
				expectImportUserCACertSuccess(ctx, s, ui)
				expectEditTrustUserCACertSuccess(ctx, s, ui, conn)
				expectDeleteUserCACertSuccess(ctx, s, ui, conn)
			} else {
				expectImportUserCACertNotPossible(ctx, s, ui)
				expectManageCACertNotPossible(ctx, s, ui, conn, userCaOrg, userCaCertName)
				expectDeleteUserCACertNotPossible(ctx, s, ui, conn)
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
				expectDeleteUserCACertSuccess(ctx, s, ui, conn)
			}
		})
	}
	// Make sure that cleanup done.
	conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
	if err != nil {
		s.Fatal("Failed to open a new tab in browser: ", err)
	}
	defer conn.Close()
	isNewUIPresent := utils.IsNewUIUsed(ctx, ui)
	ctx = context.WithValue(ctx, utils.IsNewUI, isNewUIPresent)
	expectCACertNotImported(ctx, s, ui)
}
