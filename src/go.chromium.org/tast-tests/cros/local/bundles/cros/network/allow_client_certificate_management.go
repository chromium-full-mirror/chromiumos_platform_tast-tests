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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/testing"
)

// clientCertPassword is a password for the client cert PCKS#12 archive.
const clientCertificatePassword = "12345"

// clientCertificateName is a cert name in list of certificates which is also equal to the org name of client.
const clientCertificateName = "TEST_CLIENT_ORG"

// clientCertificateOrg is name for client's organization used in the list of client's certificates.
const clientCertificateOrg = "org-TEST_CLIENT_ORG"

// clientCertFileName is name of file for the client certificate and key that will
// be used by Chrome to authenticate on the website.
// It is in a PKCS#12 format because that's what chrome supports for importing client certificates.
const clientCertFile = "cert_settings_page_client_cert.p12"

func init() {
	testing.AddTest(&testing.Test{
		Func:         AllowClientCertificateManagement,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that chrome://settings/certificates page can import and manage client certificates based on ClientCertificateManagementAllowed policy",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"olsa@google.com", // Test author
		},
		BugComponent: "b:1000044",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		Timeout:      5 * time.Minute,
		Data:         []string{clientCertFile},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ClientCertificateManagementAllowed{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// verify that managed users either have or don't have the ability to manage clients certificates based on the setting of the configured policy.
				// (COM_FOUND_CUJ4_TASK2_WF1).
				Value: "screenplay-7c74e36b-7675-4fa7-91ca-24577bb37203",
			},
		},
	})
}

// prepareClientCertificate copy certificate which is required for test to the Downloads.
func prepareClientCertificate(s *testing.State, downloadsPath string) {
	const clientCert = clientCertFile
	if err := utils.CopyToDownloads(downloadsPath, s.DataPath(clientCert), clientCert); err != nil {
		s.Fatal("Failed to copy Client certificate file to Download: ", err)
	}
}

// cleanClientCertificate removes certificate from Downloads.
func cleanClientCertificate(s *testing.State, downloadsPath string) {
	const clientCert = clientCertFile
	if err := utils.RemoveFromDownloads(downloadsPath, s.DataPath(clientCert), clientCert); err != nil {
		s.Fatal("Failed to delete Client certificate file from Download: ", err)
	}
}

// expectImportClientCertSuccess imports client certificate using "Import and Bind" button
// on the certificate management page.
func expectImportClientCertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.ImportClientCert(ctx, ui, clientCertFile, clientCertificatePassword); err != nil {
		s.Fatal("Can't import client certificate: ", err)
	}
	if err := utils.WaitForClientCert(ctx, clientCertificateName); err != nil {
		s.Fatal("Failed to wait for client certificate: ", err)
	}
}

// expectClientCertNotImported checks that client certificate is not present in the list of imported certificates.
func expectClientCertNotImported(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if status := utils.IsClientCertImported(ctx, ui, clientCertificateOrg); status == true {
		s.Fatal("Clients certificate is already present in system")
	}
}

// expectImportNoBindClientCertSuccess imports client certificate using "Import" button
// on the certificate management page.
func expectImportNoBindClientCertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.ImportNoBindClientCert(ctx, ui, clientCertFile, clientCertificatePassword); err != nil {
		s.Fatal("Can't import client certificate: ", err)
	}
}

// expectImportClientCertNotPossible checks that "Import" and "Import and Bind" buttons are not shown on screen.
func expectImportClientCertNotPossible(ctx context.Context, s *testing.State,
	ui *uiauto.Context) {
	yourCertTab := nodewith.Name("Your certificates").Role(role.Tab)
	if err := uiauto.Combine("Select Client certificates tab",
		ui.DoDefault(yourCertTab),
		ui.WaitUntilExists(yourCertTab.ClassName("tab selected")),
	)(ctx); err != nil {
		s.Fatal("Failed to select Client certificates tab: ", err)
	}

	importButton := nodewith.Name("Import").Role(role.Button)
	if err := ui.Exists(importButton)(ctx); err == nil {
		s.Fatal("Unexpected presence of 'Import' button")
	}

	importAndBindButton := nodewith.Name("Import and Bind").Role(role.Button)
	if err := ui.Exists(importAndBindButton)(ctx); err == nil {
		s.Fatal("Unexpected presence of 'Import and Bind' button")
	}
}

// expectDeleteClientCertNotPossible checks that "Delete" in not shown in the action menu for the certificate,
// while "View" and "Export" are shown.
func expectDeleteClientCertNotPossible(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.OpenActionMenuForClientCertificate(ctx, ui, clientCertificateOrg); err != nil {
		s.Fatal("Failed to open action menu: ", err)
	}

	deleteItem := nodewith.Name("Delete").Role(role.MenuItem)
	if err := ui.Exists(deleteItem)(ctx); err == nil {
		s.Fatal("Unexpected presence of 'Delete' menu item")
	}

	viewItem := nodewith.Name("View").Role(role.MenuItem)
	if err := ui.Exists(viewItem)(ctx); err != nil {
		s.Fatal("Unexpected missing of 'View' menu item")
	}

	exportItem := nodewith.Name("Export").Role(role.MenuItem)
	if err := ui.Exists(exportItem)(ctx); err != nil {
		s.Fatal("Unexpected missing of 'Export' menu item")
	}

	// Close popup menu.
	utils.PressEscape(ctx)
}

// expectDeleteClientCertSuccess uses the Chrome's cert settings page to delete the client cert.
func expectDeleteClientCertSuccess(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := utils.DeleteClientCert(ctx, ui, clientCertificateOrg); err != nil {
		s.Fatal("Failed to delete client certificate: ", err)
	}
}

// AllowClientCertificateManagement tests that based on value of ClientCertificateManagementAllowed policy,
// user can or user can't manage clients certificates. It includes tests for for the "Import", "Import and Bind",
// and "Delete" actions on client certificates from chrome://settings/certificates page.
func AllowClientCertificateManagement(ctx context.Context, s *testing.State) {
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
	prepareClientCertificate(s, downloadsPath)
	defer cleanClientCertificate(s, downloadsPath)

	// Loop via different policy settings and check that they are working as expected.
	for _, param := range []struct {
		name              string
		value             *policy.ClientCertificateManagementAllowed
		canManageUserCert bool
	}{
		{
			name:              "unset",
			value:             &policy.ClientCertificateManagementAllowed{Stat: policy.StatusUnset},
			canManageUserCert: true,
		},
		{
			name:              "all_allowed",
			value:             &policy.ClientCertificateManagementAllowed{Val: 0},
			canManageUserCert: true,
		},
		{
			name:              "only_user_allowed",
			value:             &policy.ClientCertificateManagementAllowed{Val: 1},
			canManageUserCert: true,
		},
		{
			name:              "not_allowed",
			value:             &policy.ClientCertificateManagementAllowed{Val: 2},
			canManageUserCert: false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Import user's certificate which will be used for testing, if operations
			// with certificates are forbidden.
			if !param.canManageUserCert {
				// Opening a new tab in browser.
				conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
				if err != nil {
					s.Fatal("Failed to open a new tab in browser: ", err)
				}
				defer conn.Close()

				expectImportClientCertSuccess(ctx, s, ui)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Opening a new tab in browser.
			conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
			if err != nil {
				s.Fatal("Failed to open a new tab in browser: ", err)
			}
			defer conn.Close()

			if param.canManageUserCert {
				expectClientCertNotImported(ctx, s, ui)
				expectImportClientCertSuccess(ctx, s, ui)
				expectDeleteClientCertSuccess(ctx, s, ui)
				expectImportNoBindClientCertSuccess(ctx, s, ui)
				expectDeleteClientCertSuccess(ctx, s, ui)
			} else {
				expectImportClientCertNotPossible(ctx, s, ui)
				expectDeleteClientCertNotPossible(ctx, s, ui)

				// Reset policy and perform cleanup.
				if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
					s.Fatal("Failed to clean up: ", err)
				}
				conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
				if err != nil {
					s.Fatal("Failed to open a new tab in browser: ", err)
				}
				defer conn.Close()
				expectDeleteClientCertSuccess(ctx, s, ui)
			}

		})
	}
	// Make sure that cleanup done.
	conn, err := cr.NewConn(ctx, utils.CertificatesPageURL)
	if err != nil {
		s.Fatal("Failed to open a new tab in browser: ", err)
	}
	defer conn.Close()
	expectClientCertNotImported(ctx, s, ui)
}
