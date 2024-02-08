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
	"regexp"
	"time"

	utils "go.chromium.org/tast-tests/cros/local/certpageutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/ctxutil"
)

// Certificate and key pair (both in PEM format) for the test website.
const websiteCertFileName = "cert_settings_page_website_cert.crt"
const ecWebsiteCertFileName = "ec_cert_settings_page_website_cert.pem"
const websiteKeyFileName = "cert_settings_page_website_key.key"
const ecWebsiteKeyFileName = "ec_cert_settings_page_website_key.key"

// rootCertFileName is a file's name for the root certificate that
// is used to create a client and website certificates.
// Chrome will need to import it to trust that the website certificate is valid.
// Website server will need to use it to trust that the client certificate is valid.
const rootCertFileName = "cert_settings_page_root_cert.pem"
const ecRootCertFileName = "ec_cert_settings_page_root_cert.crt"

// clientCertFileName is name of file for the client certificate and key that will
// be used by Chrome to authenticate on the website.
// It is in a PKCS#12 format because that's what chrome supports for importing client certificates.
const clientCertFileName = "cert_settings_page_client_cert.p12"
const ecClientCertFileName = "ec_cert_settings_page_client_cert.p12"

// clientCertPassword is a password for the client cert PCKS#12 archive.
const clientCertPassword = "12345"

// caCertName is a name for the CA certificate, usually it is CN or OU in certificate.
const caCertName = "TEST_CA_ORG"

// caOrg is a name for the org which has issued CA certificate, it is visible in the list of Authorities.
const caOrg = "org-TEST_CA_ORG"

// clientCertName is a cert name in list of certificates which is also equal to the org name of client.
const clientCertName = "TEST_CLIENT_ORG"

// clientOrg is name for client's organization used in the list of client's certificates.
const clientOrg = "org-TEST_CLIENT_ORG"

// pageLoadedRegex is a regex which expected to be found on loaded webpage.
const pageLoadedRegex = ".*WEBSITE_LOADED.*"

// The ERR_BAD_SSL_CLIENT_AUTH_CERT is the actual correct error. For some reason Chrome
// also can return ERR_SOCKET_NOT_CONNECTED which effectively leads to the same end result
// (Chrome fails to open a page), so we also accept it.
const connectionErrorRegex = ".*(ERR_SOCKET_NOT_CONNECTED|ERR_BAD_SSL_CLIENT_AUTH_CERT).*"

const caInvalidErrorRegex = ".*ERR_CERT_AUTHORITY_INVALID.*"

// The message on the website that indicates that it successfully loaded.
const websiteGreeting = "WEBSITE_LOADED"

type filesConfig struct {
	websiteCertFileName string
	websiteKeyFileName  string
	rootCertFileName    string
	clientCertFileName  string
}

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
			websiteCertFileName, websiteKeyFileName, ecClientCertFileName,
			ecRootCertFileName, ecWebsiteCertFileName, ecWebsiteKeyFileName},
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
		Params: []testing.Param{
			// RSA key and certificates.
			{
				Name: "",
				Val: filesConfig{
					websiteCertFileName: websiteCertFileName,
					websiteKeyFileName:  websiteKeyFileName,
					rootCertFileName:    rootCertFileName,
					clientCertFileName:  clientCertFileName,
				},
			},
			// EC key and certificates.
			{
				Name: "ec_key",
				Val: filesConfig{
					websiteCertFileName: ecWebsiteCertFileName,
					websiteKeyFileName:  ecWebsiteKeyFileName,
					rootCertFileName:    ecRootCertFileName,
					clientCertFileName:  ecClientCertFileName,
				},
			},
		},
	})
}

// prepareCertificates copy certificates which are required for tests to the Downloads.
func prepareCertificates(s *testing.State, downloadsPath string) {
	rootCertFileName := s.Param().(filesConfig).rootCertFileName
	clientCertFileName := s.Param().(filesConfig).clientCertFileName
	if err := utils.CopyToDownloads(downloadsPath, s.DataPath(rootCertFileName), rootCertFileName); err != nil {
		s.Fatal("Failed to copy CA certificate file to Download: ", err)
	}
	if err := utils.CopyToDownloads(downloadsPath, s.DataPath(clientCertFileName), clientCertFileName); err != nil {
		s.Fatal("Failed to copy Client certificate file to Download: ", err)
	}
}

// importCACert imports CA certificate.
func importCACert(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	rootCertFileName := s.Param().(filesConfig).rootCertFileName
	if err := utils.ImportCACert(ctx, ui, rootCertFileName); err != nil {
		s.Fatal("Failed to import CA certificate: ", err)
	}
}

// importClientCert imports client certificate.
func importClientCert(ctx context.Context, s *testing.State, ui *uiauto.Context) {
	clientCertFileName := s.Param().(filesConfig).clientCertFileName
	if err := utils.ImportClientCert(ctx, ui, clientCertFileName, clientCertPassword); err != nil {
		s.Fatal("Can not import client certificate: ", err)
	}
}

// waitForClientCert calls pkcs11-tool in a loop to determine when the client
// certificate gets propagated into chaps (and can be actually used by ChromeOS).
// This test assumes that the client certificate was imported last, so when it
// is ready, all the certificates should be usable.
func waitForClientCert(ctx context.Context, s *testing.State) {
	if err := utils.WaitForClientCert(ctx, clientCertName); err != nil {
		s.Fatal("Failed to wait for client certificate: ", err)
	}
}

// createWebsite creates a website that requires a client certificate from its
// clients. Its server certificate will not be accepted by default Chrome, so it
// also requires clients to use a special CA certificate.
func createWebsite(s *testing.State) *httptest.Server {
	websiteCertFileName := s.Param().(filesConfig).websiteCertFileName
	rootCertFileName := s.Param().(filesConfig).rootCertFileName
	websiteKeyFileName := s.Param().(filesConfig).websiteKeyFileName
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
	ui *uiauto.Context, kb *input.KeyboardEventWriter, website *httptest.Server, expectCertPopup bool,
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
		certPopup := nodewith.Name("Select a certificate").First()
		okButton := nodewith.Name("OK").Role(role.Button).Ancestor(certPopup)
		if err := ui.EnsureFocused(okButton)(ctx); err != nil {
			return err
		}
		if err := kb.Accel(ctx, "enter"); err != nil {
			return err
		}
		if err := ui.WithTimeout(1 * time.Second).WaitUntilGone(okButton)(ctx); err != nil {
			return err
		}
	}

	if err := ui.WaitUntilExists(nodewith.NameRegex(regexp.MustCompile(expectedText)).First())(ctx); err != nil {
		return err
	}
	return nil
}

// createAndUseWebsite creates a new website that requires the client's and the CA's
// certificates, then opens this new website in the browser and checks if any error
// messages is shown.
func createAndUseWebsite(ctx context.Context, s *testing.State,
	browser *browser.Browser, ui *uiauto.Context, kb *input.KeyboardEventWriter, expectCertPopup bool, expectedText string) {

	var loopErr error

	// It can take a bit of time for the imported certs to propagate everywhere and
	// start working. Therefore try several times until the expected result is found.
	for i := 0; i < 3; i++ {
		website := createWebsite(s)
		defer website.Close()

		loopErr = useWebsite(ctx, s, browser, ui, kb, website, expectCertPopup, expectedText)
		if err := utils.CloseCurrentPage(ctx); err != nil {
			s.Fatal("Failed to close page: ", err)
		}
		if loopErr == nil {
			break
		}
	}

	if loopErr != nil {
		s.Fatal("createAndUseWebsite failed: ", loopErr)
	}
}

// checkCertInSystemSettings opens a system settings window and checks that the client cert
// is selectable there.
func checkCertInSystemSettings(ctx context.Context, s *testing.State,
	chrome *chrome.Chrome, tconn *chrome.TestConn) {
	if err := policyutil.CheckCertificateVisibleInSystemSettings(ctx, tconn, chrome, "TEST_CA_ORG"); err != nil {
		s.Fatal("Failed to select client certificate in system settings: ", err)
	}
	if err := utils.CloseCurrentPage(ctx); err != nil {
		s.Fatal("Failed to close page: ", err)
	}
	s.Log("Client cert is usable in system settings")
}

// deleteClientCert uses the Chrome's cert settings page to delete the client cert.
func deleteClientCert(ctx context.Context, s *testing.State, ui *uiauto.Context, ignoreErrors bool) {
	if err := utils.DeleteClientCert(ctx, ui, clientOrg); err != nil && !ignoreErrors {
		s.Fatal("Failed to delete client certificate: ", err)
	}
}

// deleteCACert selects and deletes specific CA certificate on CA tab.
func deleteCACert(ctx context.Context, s *testing.State, ui *uiauto.Context, ignoreErrors bool) {
	if err := utils.DeleteCACert(ctx, ui, caCertName, caOrg); err != nil && !ignoreErrors {
		s.Fatal("Failed to delete CA certificate: ", err)
	}
}

// setCACertTrust sets Web trust setting for CA certificate to true or false.
func setCACertTrust(ctx context.Context, s *testing.State, ui *uiauto.Context, targetState checked.Checked) {
	if err := utils.SetCACertTrust(ctx, ui, targetState, caCertName, caOrg); err != nil {
		s.Fatal("Failed to set CA trust: ", err)
	}
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

	// Virtual kb initiation in tablet mode takes 1 sec, so initiating it once
	// and passing it from here.
	kb, err := input.Keyboard(ctx)
	defer kb.Close(ctx)
	if err != nil {
		s.Fatal("Failed to setup keyboard: ", err)
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

	// Try to delete certs from the certificates manager at the end of test.
	// This will clean device in case test has failed early.
	defer deleteClientCert(ctx, s, ui, true /*ignoreErrors*/)
	defer deleteCACert(ctx, s, ui, true /*ignoreErrors*/)

	// Try opening a website without any certs, that should fail with a CA error.
	createAndUseWebsite(ctx, s, browser, ui, kb, false /*expectCertPopup*/, caInvalidErrorRegex)

	// Normal case - all certs are present and the website can be connected.
	{
		// Import CA and client certs.
		importCACert(ctx, s, ui)
		importClientCert(ctx, s, ui)
		waitForClientCert(ctx, s)

		// Try to open the website again, this time it should succeed.
		createAndUseWebsite(ctx, s, browser, ui, kb, true /*expectCertPopup*/, pageLoadedRegex)
		// Also check that client cert is usable in system settings.
		checkCertInSystemSettings(ctx, s, cr, tconn)
	}

	// Test that certificates for not trusted CA will be not valid.
	{
		// Mark CA as not trusted for ssl and try connection to the website, it should fail.
		setCACertTrust(ctx, s, ui, checked.False)
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, kb, false /*expectCertPopup*/, caInvalidErrorRegex)

		// Mark CA as trusted for ssl and try connection to the website, it should succeed.
		setCACertTrust(ctx, s, ui, checked.True)
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, kb, true /*expectCertPopup*/, pageLoadedRegex)
	}

	// Delete and add certificates back, there should be no errors.
	{
		// Delete the client cert and check that now the website rejects the connection.
		deleteClientCert(ctx, s, ui, false /*ignoreErrors*/)
		createAndUseWebsite(ctx, s, browser, ui, kb, false /*expectCertPopup*/, connectionErrorRegex)

		// Import a client certs again and open the website, it should succeed.
		importClientCert(ctx, s, ui)
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, kb, true /*expectCertPopup*/, pageLoadedRegex)

		// Delete the CA cert and check that Chrome gets the CA error again.
		deleteCACert(ctx, s, ui, false /*ignoreErrors*/)
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, kb, false /*expectCertPopup*/, caInvalidErrorRegex)

		// Import the CA cert and try connection to the website, it should succeed.
		importCACert(ctx, s, ui)
		// When the CA certificate was deleted, CA was selected and the certificate's list
		// was shown. After the CA certificate is imported again, the list will be opened
		// again automatically and it will break the next test. Page reload here will put
		// all elements to the default state.
		if reloadErr := browser.ReloadActiveTab(ctx); reloadErr != nil {
			s.Fatal("Failed to reload page after CA import: ", reloadErr)
		}
		waitForClientCert(ctx, s)
		createAndUseWebsite(ctx, s, browser, ui, kb, true /*expectCertPopup*/, pageLoadedRegex)
	}

	// Clean certificates should have no errors.
	{
		// Delete the client cert and check that now the website rejects the connection.
		deleteClientCert(ctx, s, ui, false /*ignoreErrors*/)
		createAndUseWebsite(ctx, s, browser, ui, kb, false /*expectCertPopup*/, connectionErrorRegex)

		// Delete the CA cert and check that Chrome gets the CA error again.
		deleteCACert(ctx, s, ui, false /*ignoreErrors*/)
		createAndUseWebsite(ctx, s, browser, ui, kb, false /*expectCertPopup*/, caInvalidErrorRegex)
	}
}
