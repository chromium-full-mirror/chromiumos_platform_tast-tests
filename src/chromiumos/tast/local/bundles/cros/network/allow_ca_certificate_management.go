// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	utils "chromiumos/tast/local/certpageutils"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/checked"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/policyutil"
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

// caCertFile is a file's name for the root certificate that
// is used to create a client and website certificates.
// Chrome will need to import it to trust that the website certificate is valid.
// Website server will need to use it to trust that the client certificate is valid.
const caCertFile = "cert_settings_page_root_cert.crt"

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
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		Timeout:      3 * time.Minute,
		Data:         []string{caCertFile},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.CACertificateManagementAllowed{}, pci.VerifiedFunctionalityUI),
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

	// Make sure that Edit button is not present in popup menu, but other buttons are there.
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

// expectEditTrustForCACertNotPossible testing that trust bit for CA certificate can not be turned off.
func expectEditTrustForCACertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context, caOrg, caCertName string) {
	if err := utils.SelectCACertificate(ctx, ui, caOrg); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
	}
	if err := utils.OpenActionMenuForCACertificate(ctx, ui, caCertName); err != nil {
		s.Fatal("Failed to open action menu for the certificate: ", err)
	}

	// Make sure that Edit button is not present in popup menu, but other buttons are there.
	editItem := nodewith.Name("Edit").Role(role.MenuItem)
	viewItem := nodewith.Name("View").Role(role.MenuItem)
	exportItem := nodewith.Name("Export").Role(role.MenuItem)
	assertUIElementNotPresent(ctx, s, ui, editItem)
	assertUIElementPresent(ctx, s, ui, viewItem)
	assertUIElementPresent(ctx, s, ui, exportItem)

	// Close popup menu and previously selected CA org.
	utils.PressEscape(ctx)
	if err := utils.SelectCACertificate(ctx, ui, caOrg); err != nil {
		s.Fatal("Failed to select CA certificate: ", err)
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

	// Copy all required for test certificates to Download.
	prepareCACertificate(s, downloadsPath)

	// Loop via different policy settings and check that they are working as expected.
	for _, param := range []struct {
		name                          string
		value                         *policy.CACertificateManagementAllowed
		canManageUserCACert           bool
		canEditTrustForProvidedCACert bool
	}{
		{
			name:                          "unset",
			value:                         &policy.CACertificateManagementAllowed{Stat: policy.StatusUnset},
			canManageUserCACert:           true,
			canEditTrustForProvidedCACert: true,
		},
		{
			name:                          "all_allowed",
			value:                         &policy.CACertificateManagementAllowed{Val: 0},
			canManageUserCACert:           true,
			canEditTrustForProvidedCACert: true,
		},
		{
			name:                          "only_user_allowed",
			value:                         &policy.CACertificateManagementAllowed{Val: 1},
			canManageUserCACert:           true,
			canEditTrustForProvidedCACert: false,
		},
		{
			name:                          "not_allowed",
			value:                         &policy.CACertificateManagementAllowed{Val: 2},
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
			if !param.canManageUserCACert {
				// Opening a new tab in browser.
				conn, err := cr.NewConn(ctx, "chrome://settings/certificates")
				if err != nil {
					s.Fatal("Failed to open a new tab in browser: ", err)
				}
				defer conn.Close()

				expectImportUserCACertSuccess(ctx, s, ui)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Opening a new tab in browser on certificates page.
			conn, err := cr.NewConn(ctx, "chrome://settings/certificates")
			if err != nil {
				s.Fatal("Failed to open a new tab in browser: ", err)
			}
			defer conn.Close()

			if param.canManageUserCACert {
				expectImportUserCACertSuccess(ctx, s, ui)
				expectEditTrustUserCACertSuccess(ctx, s, ui)
				expectDeleteUserCACertSuccess(ctx, s, ui)
			} else {
				expectImportUserCACertNotPossible(ctx, s, ui)
				expectEditTrustForCACertNotPossible(ctx, s, ui, userCaOrg, userCaCertName)
				expectDeleteUserCACertNotPossible(ctx, s, ui)
			}

			if param.canEditTrustForProvidedCACert {
				expectEditTrustProvidedCACertSuccess(ctx, s, ui)
			} else {
				expectEditTrustForCACertNotPossible(ctx, s, ui, providedCaOrg, providedCaCertName)
			}
		})
	}
}
