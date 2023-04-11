// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/fsutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/checked"
	"chromiumos/tast/local/chrome/uiauto/event"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/sysutil"
	"chromiumos/tast/testing"
)

// The root certificate for the test CA that was used to create client and website certificates below.
// Chrome will need to import it to trust that the website certificate is valid.
// Website server will need to use it to trust that the client certificate is valid.
const rootCertFileName = "cert_settings_page_root_cert.crt"

// Client certificate and key that will be used by Chrome to authenticate on the website.
// It is in a PKCS#12 format because that's what chrome supports for importing client certificates.
const clientCertFileName = "cert_settings_page_client_cert.p12"

// The password for the client cert PCKS#12 archive.
const clientCertFilePassword = "12345"

// Certificate and key pair (both in PEM format) for the test website.
const websiteCertFileName = "cert_settings_page_website_cert.crt"
const websiteKeyFileName = "cert_settings_page_website_key.key"

const pageLoadedRegex = ".*WEBSITE_LOADED.*"

// The ERR_BAD_SSL_CLIENT_AUTH_CERT is the actual correct error. For some reason Chrome
// also can return ERR_SOCKET_NOT_CONNECTED which effectively leads to the same end result
// (Chrome fails to open a page), so we also accept it.
const connectionErrorRegex = ".*(ERR_SOCKET_NOT_CONNECTED|ERR_BAD_SSL_CLIENT_AUTH_CERT).*"

const caInvalidErrorRegex = ".*ERR_CERT_AUTHORITY_INVALID.*"

// The message on the website that indicates that it successfully loaded.
const websiteGreeting = "WEBSITE_LOADED"

// Text on CA ssl trust checkbox.
const trustCheckboxText = "Trust this certificate for identifying websites"

func init() {
	testing.AddTest(&testing.Test{
		Func:         CertSettingsPage,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test that chrome://settings/certificates page can import and use client and CA certificates",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"miersh@google.com",                         // Test author
		},
		BugComponent: "b:1000044",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"chrome", "lacros", "lacros_stable"},
		Fixture:      "lacros",
		Timeout:      5 * time.Minute,
		Data: []string{clientCertFileName, rootCertFileName,
			websiteCertFileName, websiteKeyFileName},
		SearchFlags: []*testing.StringPair{
			{
				Key: "feature_id",
				// Verify that affected users are able to access the protected page
				// (COM_FOUND_CUJ1_TASK3_WF1).
				Value: "screenplay-e6cce756-073c-4d8b-962e-299f376f6dd5",
			},
			{
				Key: "feature_id",
				// Add a client certificate (COM_FOUND_CUJ16_TASK1_WF1).
				Value: "screenplay-475a7692-c5ac-44d3-8db2-890d98d8f6d0",
			},
			{
				Key: "feature_id",
				// Remove a client certificate (COM_FOUND_CUJ16_TASK2_WF1).
				Value: "screenplay-9be9b6c0-367d-4bf2-a3d2-bd2e823ce20f",
			},
			{
				Key: "feature_id",
				// Add a CA certificate (COM_FOUND_CUJ16_TASK3_WF1).
				Value: "screenplay-cc6decc7-8869-4f61-9e6a-c9a4ecabaa00",
			},
			{
				Key: "feature_id",
				// Remove a CA certificate (COM_FOUND_CUJ16_TASK4_WF1).
				Value: "screenplay-6c71993a-4bae-406e-88f8-546ffb7803e6",
			},
			{
				Key: "feature_id",
				// Manage client certificate and CA entries by managed users
				// (COM_FOUND_CUJ16_TASK6_WF1).
				Value: "screenplay-1260e07c-6cfc-4a0c-97f0-d4e799d65261",
			},
			{
				Key: "feature_id",
				// Change trust settings for a CA certificate
				// (COM_FOUND_CUJ16_TASK5_WF1).
				Value: "screenplay-fde6b3d3-987e-4690-8044-d4d84d3ede64",
			},
		},
	})
}

// pressOkButton presses the "OK" on the dialog with the provided `parent`.
func pressOkButton(ctx context.Context, s *testing.State, ui *uiauto.Context, parent *nodewith.Finder) {
	okButton := nodewith.Name("OK").Role(role.Button).Ancestor(parent)

	if err := uiauto.Combine("press OK",
		ui.WaitUntilExists(okButton),
		ui.DoDefault(okButton),
	)(ctx); err != nil {
		s.Fatal("Failed to press OK button: ", err)
	}

	// Most of the time the code above should be enough to successfully click
	// the button.
	if ui.WithTimeout(3*time.Second).WaitUntilGone(okButton)(ctx) == nil {
		return
	}

	// On some dialogs the "OK" button is generally a bit flaky, and on devices
	// in the tablet mode the DoDefault/LeftClick methods don't work at all
	// (while a real touch works). Send "enter" as a workaround.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	if err := kb.Accel(ctx, "enter"); err != nil {
		s.Fatal("Failed to use keyboard: ", err)
	}

	// If still didn't work, fail the test.
	if err := ui.WithTimeout(3 * time.Second).WaitUntilGone(okButton)(ctx); err != nil {
		s.Fatal("OK button didn't work: ", err)
	}
}

// closeCurrentPage presses "ctrl+w" to close the current active window / tab.
func closeCurrentPage(ctx context.Context, s *testing.State) {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close()

	if err := kb.Accel(ctx, "ctrl+w"); err != nil {
		s.Fatal("Failed to close the page: ", err)
	}
}

// copyToDownloads copies the test data file with `fileName` into the Downloads
// directory, so it can be picked from the ChromeOS file picker.
func copyToDownloads(s *testing.State, downloadsPath, fileName string) {
	newPath := filepath.Join(downloadsPath, fileName)
	err := fsutil.CopyFile(s.DataPath(fileName), newPath)
	if err != nil {
		s.Fatalf("Failed to move file %s: %v", fileName, err)
	}
	// Without this the test data files don't have enough permissions and Chrome
	// fails to open them.
	err = os.Chown(newPath, int(sysutil.ChronosUID), int(sysutil.ChronosGID))
	if err != nil {
		s.Fatalf("Failed to chown file %s: %v", fileName, err)
	}
}

// prepareCertificates moves certificates which are required for tests to Downloads.
func prepareCertificates(s *testing.State, downloadsPath string) {
	const caCert = rootCertFileName
	copyToDownloads(s, downloadsPath, caCert)
	const clientCert = clientCertFileName
	copyToDownloads(s, downloadsPath, clientCert)
}

// importCACert copies the `fileName` test data file into the Downloads
// directory and uses the Import button on the chrome://settings/certificates
// page to manually import it.
func importCACert(ctx context.Context, s *testing.State, ui *uiauto.Context, downloadsPath string) {
	const fileName = rootCertFileName
	if err := uiauto.Combine("import CA cert",
		ui.DoDefault(nodewith.Name("Authorities").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Authorities").ClassName("tab selected")),
		ui.DoDefault(nodewith.Name("Import").Role(role.Button)),
		ui.DoDefault(nodewith.Name(fileName).Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name("Open").Role(role.Button).State("focusable", true)),
		ui.DoDefault(nodewith.Name("Open").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name(trustCheckboxText).Role(role.CheckBox)),
		ui.DoDefault(nodewith.Name(trustCheckboxText).Role(role.CheckBox)),
	)(ctx); err != nil {
		s.Fatal("Failed to import CA cert: ", err)
	}

	pressOkButton(ctx, s, ui, nodewith.Name("Settings - Manage certificates").Role("rootWebArea"))
	s.Log("Imported CA cert: ", fileName)
}

// importClientCert copies the client cert data file into the Downloads
// directory and uses the Import and Bind button on the
// chrome://settings/certificates page to manually import it.
func importClientCert(ctx context.Context, s *testing.State, ui *uiauto.Context, downloadsPath string) {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close()

	passwordDialog := nodewith.Name("Enter your certificate password").Role(role.Dialog)
	passwordTextBox := nodewith.Role(role.TextField).Editable()
	if err := uiauto.Combine("import client cert",
		ui.DoDefault(nodewith.Name("Your certificates").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Your certificates").ClassName("tab selected")),
		ui.DoDefault(nodewith.Name("Import and Bind").Role(role.Button)),
		ui.DoDefault(nodewith.Name(clientCertFileName).Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name("Open").Role(role.Button).State("focusable", true)),
		ui.DoDefault(nodewith.Name("Open").Role(role.Button)),
		ui.WaitUntilExists(passwordTextBox.Ancestor(passwordDialog).State("focusable", true)),
		ui.DoDefault(passwordTextBox.Ancestor(passwordDialog)),
		kb.TypeAction(clientCertFilePassword),
	)(ctx); err != nil {
		s.Fatal("Failed to import client certificate: ", err)
	}

	pressOkButton(ctx, s, ui, nodewith.Name("Settings - Manage certificates").Role("rootWebArea"))
	s.Log("Imported client cert: ", clientCertFileName)
}

// waitForClientCert calls pkcs11-tool in a loop to determine when the client
// certificate gets propagated into chaps (and can be actually used by ChromeOS).
// This test assumes that the client certificate was imported last, so when it
// is ready, all the certificates should be usable.
func waitForClientCert(ctx context.Context, s *testing.State) {
	// Wait until the certificate is installed.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// The argument "--slot 1" means "use user slot only". That's where Import
		// and Bind is supposed to place the cert.
		out, err := testexec.CommandContext(ctx,
			"pkcs11-tool", "--module", "libchaps.so", "--slot", "1", "--list-objects").Output()
		if err != nil {
			return errors.Wrap(err, "failed to get certificate list")
		}
		outStr := string(out)

		// Look for the org name of the `clientCertFileName`.
		if !strings.Contains(outStr, "TEST_CLIENT_ORG") {
			return errors.New("certificate not installed")
		}

		return nil

	}, nil); err != nil {
		s.Fatal("Could not verify that client certificate was installed: ", err)
	}
}

// createWebsite creates a website that requires a client certificate from its
// clients. Its server certificate will not be accepted by default Chrome, so it
// also requires clients to use a special CA certificate.
func createWebsite(s *testing.State) *httptest.Server {
	handleRequest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, websiteGreeting)
	})
	testServer := httptest.NewUnstartedServer(handleRequest)

	websiteCert, err := tls.LoadX509KeyPair(s.DataPath(websiteCertFileName),
		s.DataPath(websiteKeyFileName))
	if err != nil {
		s.Fatal("Failed to load website cert: ", err)
	}

	rootCertPem, err := ioutil.ReadFile(s.DataPath(rootCertFileName))
	if err != nil {
		s.Fatal("Failed to read root cert: ", err)
	}
	rootCertPool := x509.NewCertPool()
	rootCertPool.AppendCertsFromPEM(rootCertPem)

	testServer.TLS = &tls.Config{
		// Requires all clients to present a valid client certificate.
		ClientAuth: tls.RequireAndVerifyClientCert,
		// The certificate that the website presents to the client, so the client can trust it.
		// The client must have a corresponding root certificate for this to work.
		Certificates: []tls.Certificate{websiteCert},
		// Root certificates that the website will use to validate client certificates.
		ClientCAs: rootCertPool,
	}

	testServer.StartTLS()

	return testServer
}

// useWebsite attempts to open `website` in Chrome. If `expectCertPopup` is true,
// the function will wait for and handle the cert selection window. `expectedText`
// specifies what text should be seen on the page (for both successful and
// failed page loads).
func useWebsite(ctx context.Context, s *testing.State, browser *browser.Browser,
	ui *uiauto.Context, website *httptest.Server, expectCertPopup bool,
	expectedText string) error {

	websiteConn, err := browser.NewConn(ctx, "")
	if err != nil {
		return err
	}
	defer websiteConn.Close()

	// A navigation with NewConn wouldn't work, because it cannot fully load the
	// page until the client cert is selected and hangs on that.
	websiteConn.Eval(ctx, "window.location.href = '"+website.URL+"';", nil)

	if expectCertPopup {
		// "First()" is good enough because all such UI elements are in the same
		// chain together with the OK button.
		pressOkButton(ctx, s, ui, nodewith.Name("Select a certificate").First())
	}

	if err := ui.WaitUntilExists(nodewith.NameRegex(regexp.MustCompile(expectedText)).First())(ctx); err != nil {
		return err
	}
	return nil
}

// createAndUseWebsite creates a new website that requires the client and the CA
// certs and
func createAndUseWebsite(ctx context.Context, s *testing.State,
	browser *browser.Browser, ui *uiauto.Context, expectCertPopup bool, expectedText string) {

	var loopErr error

	// It can take a bit of time for the imported certs to propagate everywhere and
	// start working. Therefore try several times until the expected result is found.
	for i := 0; i < 3; i++ {
		website := createWebsite(s)
		defer website.Close()

		loopErr = useWebsite(ctx, s, browser, ui, website, expectCertPopup, expectedText)
		closeCurrentPage(ctx, s)
		if loopErr == nil {
			break
		}
	}

	if loopErr != nil {
		s.Fatal("createAndUseWebsite failed: ", loopErr)
	}
}

// useSystemSettings opens a system settings window and checks that the client cert
// is selectable there.
func useSystemSettings(ctx context.Context, s *testing.State,
	chrome *chrome.Chrome, tconn *chrome.TestConn) {
	if err := policyutil.CheckCertificateVisibleInSystemSettings(ctx, tconn, chrome, "TEST_CA_ORG"); err != nil {
		s.Fatal("Failed to select client certificate in system settings: ", err)
	}
	closeCurrentPage(ctx, s)
	s.Log("Client cert is usable in system settings")
}

// deleteClientCert uses the Chrome's cert settings page to delete the client cert.
func deleteClientCert(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	if err := uiauto.Combine("delete client cert",
		ui.DoDefault(nodewith.Name("Your certificates").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Your certificates").ClassName("tab selected")),
		ui.WaitUntilExists(nodewith.Name("org-TEST_CLIENT_ORG").First()),
		ui.DoDefault(nodewith.Name("Show certificates for organization").Role(role.Button)),
		ui.DoDefault(nodewith.Name("More actions").Role(role.Button)),
		ui.DoDefault(nodewith.Name("Delete").Role(role.MenuItem)),
	)(ctx); err != nil {
		s.Fatal("Failed to delete CA cert: ", err)
	}

	pressOkButton(ctx, s, ui, nodewith.Name("Settings - Manage certificates").Role("rootWebArea"))

	if err := ui.WaitUntilGone(nodewith.Name("org-TEST_CLIENT_ORG"))(ctx); err != nil {
		s.Fatal("Failed to delete CA cert: ", err)
	}
}

// deleteCACert selects and deletes specific CA certificate on CA tab.
func deleteCACert(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	selectCACertificate(ctx, s, ui)
	openActionMenuForCACertificate(ctx, s, ui)

	deleteButton := nodewith.Name("Delete").Role(role.MenuItem)
	if err := uiauto.Combine("delete CA cert",
		ui.WaitUntilExists(deleteButton),
		ui.DoDefault(deleteButton),
		ui.WaitUntilExists(nodewith.NameContaining("Delete CA certificate").First()),
	)(ctx); err != nil {
		s.Fatal("Failed to delete CA cert: ", err)
	}

	pressOkButton(ctx, s, ui, nodewith.Name("Settings - Manage certificates").Role("rootWebArea"))

	if err := ui.WaitUntilGone(nodewith.Name("TEST_CA_ORG"))(ctx); err != nil {
		s.Fatal("Failed to delete CA cert: ", err)
	}
}

// moveToNextUIElement selects next UI element on page with keyboard.
func moveToNextUIElement(ctx context.Context, s *testing.State) {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	if err := kb.Accel(ctx, "tab"); err != nil {
		s.Fatal("Failed to use keyboard: ", err)
	}
	if err := kb.Accel(ctx, "enter"); err != nil {
		s.Fatal("Failed to use keyboard: ", err)
	}
}

// selectCACertificate selects CA on CA tab and open/close list of certificates
// for an organization.
func selectCACertificate(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	caCertOrg := nodewith.Name("org-TEST_CA_ORG").Role(role.StaticText)
	if err := uiauto.Combine("select CA from list",
		ui.DoDefault(nodewith.Name("Authorities").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Authorities").ClassName("tab selected")),
		ui.MakeVisible(caCertOrg),
		ui.LeftClick(caCertOrg),
	)(ctx); err != nil {
		s.Fatal("Failed to select CA from list: ", err)
	}
	// Open/close drop down list of certificates under selected CA.
	// The UI tree for these elements is not very convenient.
	// Use keyboard to navigate.
	moveToNextUIElement(ctx, s)
}

// selectEditCACertificate finds "Edit" button from certificate action menu and click on it.
func selectEditCACertificate(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	editButton := nodewith.Name("Edit").Role(role.MenuItem)
	if err := uiauto.Combine("press Edit button for certificate",
		ui.WaitUntilExists(editButton),
		ui.DoDefault(editButton),
		ui.WaitUntilExists(nodewith.NameContaining("Certificate authority").First()),
	)(ctx); err != nil {
		s.Fatal("Press Edit button for CA certificate: ", err)
	}
}

// openActionMenuForCACertificate selects specific CA certificate on CA tab and open actions menu for it.
func openActionMenuForCACertificate(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	caCertificateNode := nodewith.Name("TEST_CA_ORG").Role(role.StaticText)
	if err := uiauto.Combine("select CA cert from list",
		ui.WaitUntilExists(caCertificateNode),
		ui.LeftClick(caCertificateNode),
	)(ctx); err != nil {
		s.Fatal("Failed to select CA cert from list: ", err)
	}

	// Open menu for the selected certificate from 3 dots using keyboard.
	moveToNextUIElement(ctx, s)
}

// setTrustCheckboxAndSave sets CA certificate's trust checkbox to desired state based on
// provided `targetState` parameter and saves result.
func setTrustCheckboxAndSave(ctx context.Context, s *testing.State, ui *uiauto.Context, targetState checked.Checked) {
	checkbox := nodewith.Name(trustCheckboxText).Role(role.CheckBox)
	if err := uiauto.Combine("find CA trust checkbox",
		ui.WaitUntilExists(checkbox),
	)(ctx); err != nil {
		s.Fatal("Failed to find CA trust checkbox: ", err)
	}

	for {
		info, err := ui.Info(ctx, checkbox)
		if err != nil {
			s.Fatal("Failed to find CA trust checkbox status: ", err)
		}
		if info.Checked == targetState {
			break
		}
		if err := uiauto.Combine(("toggle checkbox value"),
			ui.WaitUntilExists(checkbox.Focusable()),
			ui.EnsureFocused(checkbox),
			ui.WaitForEvent(checkbox, event.CheckedStateChanged, ui.DoDefault(checkbox)),
		)(ctx); err != nil {
			s.Fatal("Failed to set CA trust checkbox: ", err)
		}
	}

	pressOkButton(ctx, s, ui, nodewith.Name("Settings - Manage certificates").Role("rootWebArea"))
}

// setCACertTrust sets Web trust setting for CA certificate to true or false.
func setCACertTrust(ctx context.Context, s *testing.State, ui *uiauto.Context, targetState checked.Checked) {
	selectCACertificate(ctx, s, ui)
	openActionMenuForCACertificate(ctx, s, ui)
	selectEditCACertificate(ctx, s, ui)
	setTrustCheckboxAndSave(ctx, s, ui, targetState)

	// Hide list of CA certificates by selecting CA again.
	selectCACertificate(ctx, s, ui)
}

// CertSettingsPage tests successful connection to the website using the client's
// certificate from the trusted CA. It also tests that a missed client's certificate
// or a missed CA certificate, or not trusted CA certificate will lead to the errors
// during connection. In addition it tests that the trust bit can be removed from
// the CA certificate and it can be added back to it.
func CertSettingsPage(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	browserType := browser.TypeLacros

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	browser, closeBrowser, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	s.Logf("Opening a new tab in %v browser", browserType)
	conn, err := browser.NewConn(ctx, "chrome://settings/certificates")
	if err != nil {
		s.Fatalf("Failed to open a new tab in %v browser: %v", browserType, err)
	}
	defer conn.Close()

	ui := uiauto.New(tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	// Copy all required for test certificates to Download.
	prepareCertificates(s, downloadsPath)

	// Try opening a website without any certs, that should fail with a CA error.
	createAndUseWebsite(ctx, s, browser, ui, false /*expectCertPopup*/, caInvalidErrorRegex)

	// Normal case - all certs are present and the website can be connected.
	{
		// Import CA and client certs.
		importCACert(ctx, s, ui, downloadsPath)
		importClientCert(ctx, s, ui, downloadsPath)
		waitForClientCert(ctx, s)

		// Try to open the website again, this time it should succeed.
		createAndUseWebsite(ctx, s, browser, ui, true /*expectCertPopup*/, pageLoadedRegex)
		// Also check that client cert is usable in system settings.
		useSystemSettings(ctx, s, cr, tconn)
	}

	// Test that certificates for not trusted CA will be not valid.
	{
		// Mark CA as not trusted for ssl and try connection to the website, it should fail.
		setCACertTrust(ctx, s, ui, checked.False)
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, false /*expectCertPopup*/, caInvalidErrorRegex)

		// Mark CA as trusted for ssl and try connection to the website, it should succeed.
		setCACertTrust(ctx, s, ui, checked.True)
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, true /*expectCertPopup*/, pageLoadedRegex)
	}

	// Delete and add certificates back, there should be no errors.
	{
		// Delete the client cert and check that now the website rejects the connection.
		deleteClientCert(ctx, s, ui)
		createAndUseWebsite(ctx, s, browser, ui, false /*expectCertPopup*/, connectionErrorRegex)

		// Import a client certs again and open the website, it should succeed.
		importClientCert(ctx, s, ui, downloadsPath)
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, true /*expectCertPopup*/, pageLoadedRegex)

		// Delete the CA cert and check that Chrome gets the CA error again.
		deleteCACert(ctx, s, ui)
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, false /*expectCertPopup*/, caInvalidErrorRegex)

		// Import the CA cert and try connection to the website, it should succeed.
		importCACert(ctx, s, ui, downloadsPath)
		// When the CA certificate was deleted, CA was selected and the certificate's list
		// was shown. After the CA certificate is imported again, the list will be opened
		// again automatically and it will break the next test. Page reload here will put
		// all elements to the default state.
		if reloadErr := browser.ReloadActiveTab(ctx); reloadErr != nil {
			s.Fatal("Failed to reload page after CA import", reloadErr)
		}
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, true /*expectCertPopup*/, pageLoadedRegex)
	}

	// Clean certificates should have no errors.
	{
		// Delete the client cert and check that now the website rejects the connection.
		deleteClientCert(ctx, s, ui)
		createAndUseWebsite(ctx, s, browser, ui, false /*expectCertPopup*/, connectionErrorRegex)

		// Delete the CA cert and check that Chrome gets the CA error again.
		deleteCACert(ctx, s, ui)
		createAndUseWebsite(ctx, s, browser, ui, false /*expectCertPopup*/, caInvalidErrorRegex)
	}
}
