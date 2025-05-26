// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package certpageutils implements miscellaneous and unsorted helpers used on
certificate's settings page (not certificate manager) for testing
policies/operations on users certificates and CA certificates.
*/
package certpageutils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// TrustCheckboxText is a text on CA ssl trust checkbox.
const TrustCheckboxText = "Trust this certificate for identifying websites"

// FailedToCheckForNewUI is error message for failed check if new UI is used by cert manager.
const FailedToCheckForNewUI = "Failed to determine if new UI is used : "

// failedToSetupKeyboardErr is error message for not successful keyboard initialization.
const failedToSetupKeyboardErr = "failed to setup keyboard"

// failedToDeleteCertErr is error message for certificate deletion failure.
const failedToDeleteCertErr = "failed to delete cert"

// failedToPressOkErr is error message for failed interaction with "OK" button in UI.
const failedToPressOkErr = "failed to press OK button"

// failedToSelectNextUIElementErr is error message for failed interaction next UI element.
const failedToSelectNextUIElementErr = "failed to select next UI element"

// ContextKey is a key type to look up values in CTX contexts.
type ContextKey string

const (
	// KeyboardKey is a key name for the keyboard in ctx if it is provided.
	KeyboardKey ContextKey = "keyboard"

	// IsNewUI is a key name in ctx for the usage of new UI in certificate manager.
	IsNewUI ContextKey = "isNewUI"
)

// ManageCertSettingsWebArea is UI element finder for "Settings - Manage certificates" root web area.
var ManageCertSettingsWebArea = nodewith.Name("Settings - Manage certificates").Role("rootWebArea")

// CertificatesPageURL is a certificates page url.
const CertificatesPageURL = "chrome://settings/certificates"

// userCACertPage is a name for the page with CA certificates imported by user.
const userCACertPage = "userCA"

// adminCACertPage is a name for the page with CA certificates installed by admin via policy.
const adminCACertPage = "adminCA"

// userClientCertPage is a name for the page with client certificates installed by user.
const userClientCertPage = "userClient"

// PressOkButton presses the "OK" on the dialog with the provided `parent`.
// On some dialogs the "OK" button is generally a bit flaky, and on devices
// in the tablet mode the DoDefault/LeftClick methods don't work at all
// (while a real touch works). Send "enter" as a workaround.
func PressOkButton(ctx context.Context, ui *uiauto.Context, parent *nodewith.Finder) (retErr error) {
	okButton := nodewith.Name("OK").Role(role.Button).Ancestor(parent)
	// On slower boards OK button is shown with delay, need to retry this step.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if buttonErr := uiauto.Combine("press OK",
			ui.WaitUntilExists(okButton.Focusable()),
			ui.DoDefault(okButton))(ctx); buttonErr != nil {
			return errors.Wrap(buttonErr, "failed to focus on OK button")
		}
		return nil
	}, &testing.PollOptions{Timeout: 7 * time.Second}); err != nil {
		return err
	}

	if err := ui.WithTimeout(7 * time.Second).WaitUntilGone(okButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for OK button has gone")
	}
	return nil
}

// PressTabAndEnter presses several keys to move into desired position.
func PressTabAndEnter(ctx context.Context) (retErr error) {
	kb, kbCleanup, err := getKeyboard(ctx)
	defer kbCleanup(ctx)
	if err != nil {
		return errors.Wrap(err, failedToSetupKeyboardErr)
	}

	kb.Accel(ctx, "Tab")
	kb.Accel(ctx, "Enter")
	return nil
}

// CloseCurrentPage presses "ctrl+w" to close the current active window / tab.
func CloseCurrentPage(ctx context.Context) (retErr error) {
	kb, kbCleanup, err := getKeyboard(ctx)
	defer kbCleanup(ctx)
	if err != nil {
		return errors.Wrap(err, failedToSetupKeyboardErr)
	}

	if err := kb.Accel(ctx, "ctrl+w"); err != nil {
		return errors.Wrap(err, "failed to close the page")
	}
	return nil
}

// CopyToDownloads copies the test data file with `fileName` into the Downloads
// directory, so it can be picked from the ChromeOS file picker.
func CopyToDownloads(downloadsPath, fileDataPath, fileName string) (retErr error) {
	newPath := filepath.Join(downloadsPath, fileName)
	if err := fsutil.CopyFile(fileDataPath, newPath); err != nil {
		return errors.Wrapf(err, "failed to move file %s ", fileName)
	}
	// Without this the test data files don't have enough permissions and Chrome
	// fails to open them.
	if err := os.Chown(newPath, int(sysutil.ChronosUID), int(sysutil.ChronosGID)); err != nil {
		return errors.Wrapf(err, "failed to chown file %s ", fileName)
	}
	return nil
}

// RemoveFromDownloads removes file with `fileName` from the Downloads
// directory, so Downloads can be cleaned up after the test.
func RemoveFromDownloads(downloadsPath, fileDataPath, fileName string) (retErr error) {
	filePath := filepath.Join(downloadsPath, fileName)
	if err := os.Remove(filePath); err != nil {
		return errors.Wrapf(err, "failed to remove file %s ", fileName)
	}
	return nil
}

// ImportCACertNewUI uses the Import button on the chrome://certificate-manager
// page to manually import `caCertFileName` file to CA certificates.
func ImportCACertNewUI(ctx context.Context, ui *uiauto.Context, caCertFileName string) (retErr error) {
	if err := OpenUserInstalledCACertsNewUI(ctx, ui); err != nil {
		return errors.Wrap(err, "failed to select Installed by you tab")
	}
	if err := uiauto.Combine("import CA cert",
		ui.WaitUntilExists(nodewith.NameStartingWith("Import to Trusted Certificates").Role(role.Button)),
		ui.DoDefault(nodewith.NameStartingWith("Import to Trusted Certificates").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name(caCertFileName).Role(role.StaticText)),
		ui.DoDefault(nodewith.Name(caCertFileName).Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name("Open").Role(role.Button).State("focusable", true)),
		ui.DoDefault(nodewith.Name("Open").Role(role.Button)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to import CA cert")
	}

	return nil
}

// ImportCACert uses the Import button on the chrome://settings/certificates
// page to manually import `caCertFileName` file to CA certificates.
func ImportCACert(ctx context.Context, ui *uiauto.Context, caCertFileName string) (retErr error) {
	if err := uiauto.Combine("import CA cert",
		ui.WaitUntilExists(nodewith.Name("Authorities").Role(role.Tab)),
		ui.DoDefault(nodewith.Name("Authorities").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Authorities").ClassName("tab selected")),
		ui.DoDefault(nodewith.Name("Import").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name(caCertFileName).Role(role.StaticText)),
		ui.DoDefault(nodewith.Name(caCertFileName).Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name("Open").Role(role.Button).State("focusable", true)),
		ui.DoDefault(nodewith.Name("Open").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name(TrustCheckboxText).Role(role.CheckBox)),
		ui.DoDefault(nodewith.Name(TrustCheckboxText).Role(role.CheckBox)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to import CA cert")
	}
	if err := PressOkButton(ctx, ui, ManageCertSettingsWebArea); err != nil {
		return errors.Wrap(err, failedToPressOkErr)
	}

	return nil
}

// IsClientCertImported checks if client certificate is present in the list of imported certificates.
func IsClientCertImported(ctx context.Context, ui *uiauto.Context, clientOrg string) (status bool) {
	if err := uiauto.Combine("check client cert",
		ui.DoDefault(nodewith.Name("Your certificates").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Your certificates").ClassName("tab selected")),
		ui.WithTimeout(15*time.Second).WaitUntilExists(nodewith.Name(clientOrg).First()),
	)(ctx); err != nil {
		return false
	}
	return true
}

// IsCACertOrgExistsNewUI checks if CA certificate is present in the list of certificates.
func IsCACertOrgExistsNewUI(ctx context.Context, ui *uiauto.Context, caOrg string) (status bool, err error) {
	if err := OpenUserInstalledCACertsNewUI(ctx, ui); err != nil {
		return false, errors.Wrap(err, "failed to select 'Installed by you' tab")
	}

	caCertOrgText := nodewith.NameContaining(caOrg).Role(role.StaticText)
	if err := uiauto.Combine("Find org name on open tab",
		ui.WithTimeout(3*time.Second).WaitUntilExists(caCertOrgText),
	)(ctx); err != nil {
		return false, nil
	}
	return true, nil
}

// IsCACertOrgExists checks if CA certificate's org is present in the list of CA certificates orgs.
func IsCACertOrgExists(ctx context.Context, ui *uiauto.Context, caOrg string) (status bool, err error) {
	caCertOrgText := nodewith.Name(caOrg).Role(role.StaticText)
	if err := uiauto.Combine("Find CA org in orgs list",
		ui.DoDefault(nodewith.Name("Authorities").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Authorities").ClassName("tab selected")),
	)(ctx); err != nil {
		return false, err
	}

	if err := uiauto.Combine("Find CA org in orgs list",
		ui.WithTimeout(10*time.Second).WaitUntilExists(caCertOrgText),
	)(ctx); err != nil {
		return false, nil
	}
	return true, nil
}

// ImportClientCertImpl uses the provided buttonName on the
// chrome://settings/certificates page to manually import
// the client certificate from file.
func ImportClientCertImpl(ctx context.Context, ui *uiauto.Context, clientCertFileName, certFilePassword, buttonName string) (retErr error) {
	kb, kbCleanup, err := getKeyboard(ctx)
	defer kbCleanup(ctx)
	if err != nil {
		return errors.Wrap(err, failedToSetupKeyboardErr)
	}

	passwordDialog := nodewith.Name("Enter your certificate password").Role(role.Dialog)
	passwordTextBox := nodewith.Role(role.TextField).Editable()

	if err := uiauto.Combine("import client cert",
		ui.DoDefault(nodewith.Name("Your certificates").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Your certificates").ClassName("tab selected")),
		ui.DoDefault(nodewith.Name(buttonName).Role(role.Button)),
		ui.DoDefault(nodewith.Name(clientCertFileName).Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name("Open").Role(role.Button).State("focusable", true)),
		ui.DoDefault(nodewith.Name("Open").Role(role.Button)),
		ui.WaitUntilExists(passwordTextBox.Ancestor(passwordDialog).State("focusable", true)),
		ui.DoDefault(passwordTextBox.Ancestor(passwordDialog)),
		kb.TypeAction(certFilePassword),
		kb.AccelAction("enter"),
		ui.WithTimeout(3*time.Second).WaitUntilGone(passwordTextBox.Ancestor(passwordDialog)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to import client certificate")
	}

	return nil
}

// ImportClientCert uses the Import and Bind button on the chrome://settings/certificates page
// to manually import the client certificate from file. It is default action for importing certificates,
// so Bind is not used in name.
func ImportClientCert(ctx context.Context, ui *uiauto.Context, clientCertFileName, certFilePassword string) (retErr error) {
	buttonNameForImport := "Import and Bind"
	if err := ImportClientCertImpl(ctx, ui, clientCertFileName, certFilePassword, buttonNameForImport); err != nil {
		return err
	}
	return nil
}

// ImportNoBindClientCert uses the Import button on the chrome://settings/certificates
// page to manually import the client certificate from file.
func ImportNoBindClientCert(ctx context.Context, ui *uiauto.Context, clientCertFileName, certFilePassword string) (retErr error) {
	buttonNameForImport := "Import"
	if err := ImportClientCertImpl(ctx, ui, clientCertFileName, certFilePassword, buttonNameForImport); err != nil {
		return err
	}
	return nil
}

// WaitForClientCert calls pkcs11-tool in a loop to determine when the client
// certificate gets propagated into chaps (and can be actually used by ChromeOS).
// This test assumes that the client certificate was imported last, so when it
// is ready, all the certificates should be usable.
func WaitForClientCert(ctx context.Context, clientOrgName string) (retErr error) {
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

		// Look for the org name of the client's certificate.
		if !strings.Contains(outStr, clientOrgName) {
			return errors.New("certificate not installed")
		}

		return nil

	}, nil); err != nil {
		return errors.Wrap(err, "could not verify that client certificate was installed")
	}
	return nil
}

// OpenActionMenuForClientCertificate opens action menu for the clients certificate.
func OpenActionMenuForClientCertificate(ctx context.Context, ui *uiauto.Context, clientOrg string) (retErr error) {
	if err := uiauto.Combine("open client cert actions menu",
		ui.DoDefault(nodewith.Name("Your certificates").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Your certificates").ClassName("tab selected")),
		ui.WaitUntilExists(nodewith.Name(clientOrg).First()),
		ui.DoDefault(nodewith.Name("Show certificates for organization").Role(role.Button)),
		ui.DoDefault(nodewith.Name("More actions").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name("View").Role(role.MenuItem)),
	)(ctx); err != nil {
		return err
	}
	return nil
}

// DeleteClientCert uses the Chrome's cert settings page to delete the client cert.
func DeleteClientCert(ctx context.Context, ui *uiauto.Context, clientOrg string) (retErr error) {
	if err := OpenActionMenuForClientCertificate(ctx, ui, clientOrg); err != nil {
		return errors.Wrap(err, "failed to open action menu for client certificate")
	}

	if err := ui.DoDefault(nodewith.Name("Delete").Role(role.MenuItem))(ctx); err != nil {
		return errors.Wrap(err, failedToDeleteCertErr)
	}

	if err := PressOkButton(ctx, ui, ManageCertSettingsWebArea); err != nil {
		return errors.Wrap(err, failedToPressOkErr)
	}

	if err := ui.WaitUntilGone(nodewith.Name(clientOrg))(ctx); err != nil {
		errors.Wrap(err, failedToDeleteCertErr)
	}
	return nil
}

// DeleteClientCertWithRetry calls DeleteClientCert for multiple times and tries
// to dismiss the "Certificate Delete Error" popup on failures, which
// occasionally happens on entry-level platforms.
func DeleteClientCertWithRetry(ctx context.Context, ui *uiauto.Context, clientOrg string) (retErr error) {
	// If a "Certificate Delete Error" dialog pops off, the UI could be stuck in a
	// weird state and not responding. Somehow it works for the 3rd attempt.
	maxAttempts := 3
	lastError := error(nil)
	for i := 0; i < maxAttempts; i++ {
		if err := DeleteClientCert(ctx, ui, clientOrg); err != nil {
			lastError = err
			// Dismiss the potential "Certificate Delete Error" popup.
			if err := PressOkButton(ctx, ui, ManageCertSettingsWebArea); err != nil {
				// Continue on errors to workaounrd the UI issue mentioned above.
				lastError = errors.Wrap(err, failedToPressOkErr)
			}
			continue
		}
		return nil
	}

	return lastError
}

// DeleteCACertNewUI selects and deletes specific CA certificate on CA tab.
func DeleteCACertNewUI(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, userCaCertName, caCertName string) (retErr error) {
	if err := OpenUserInstalledCACertsNewUI(ctx, ui); err != nil {
		return errors.Wrap(err, "failed to select CA certificates tab")
	}

	deleteNamePrefix := "Delete certificate " + userCaCertName
	deleteButton := nodewith.NameStartingWith(deleteNamePrefix)
	dialogMessage := nodewith.NameStartingWith("If you delete a server")
	deletePopupDialog := nodewith.NameStartingWith("Delete ").Role(role.Dialog)
	if err := uiauto.Combine("delete CA cert dialog start",
		ui.WaitUntilExists(deleteButton.Role(role.Button)),
		ui.DoDefault(deleteButton.Role(role.Button)),
		ui.WaitUntilExists(deletePopupDialog),
		ui.WaitUntilExists(dialogMessage.Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name("Cancel").Role(role.Button).Ancestor(deletePopupDialog)),
		ui.WaitUntilExists(nodewith.Name("OK").Role(role.Button).Ancestor(deletePopupDialog)),
		ui.DoDefault(nodewith.Name("OK").Role(role.Button).Ancestor(deletePopupDialog)),
		ui.WaitUntilGone(deletePopupDialog),
	)(ctx); err != nil {
		return errors.Wrap(err, failedToDeleteCertErr)
	}

	return nil
}

// DeleteCACert selects and deletes specific CA certificate on CA tab.
func DeleteCACert(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, caOrg, caCertName string) (retErr error) {
	if err := SelectCACertificate(ctx, ui, conn, caOrg, caCertName); err != nil {
		return errors.Wrap(err, "failed to select CA certificate")
	}
	if err := OpenActionMenuForCACertificate(ctx, ui, conn, caCertName); err != nil {
		return errors.Wrap(err, "failed to open action menu for CA certificate")
	}

	deleteButton := nodewith.Name("Delete").Role(role.MenuItem)
	if err := uiauto.Combine("delete CA cert",
		ui.WaitUntilExists(deleteButton),
		ui.DoDefault(deleteButton),
		ui.WaitUntilExists(nodewith.NameContaining("Delete CA certificate").First()),
	)(ctx); err != nil {
		return errors.Wrap(err, failedToDeleteCertErr)
	}

	if err := PressOkButton(ctx, ui, ManageCertSettingsWebArea); err != nil {
		return errors.Wrap(err, failedToPressOkErr)
	}

	if err := ui.WaitUntilGone(nodewith.Name(caCertName))(ctx); err != nil {
		return errors.Wrap(err, failedToDeleteCertErr)
	}
	return nil
}

// getKeyboard will try to get keyboard from ctx before initializing it.
func getKeyboard(ctx context.Context) (*input.KeyboardEventWriter, func(context.Context), error) {
	emptyCleanup := func(ctx context.Context) {}

	if ctx.Value(KeyboardKey) != nil {
		kb := ctx.Value(KeyboardKey).(*input.KeyboardEventWriter)
		return kb, emptyCleanup, nil
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, emptyCleanup, errors.Wrap(err, failedToSetupKeyboardErr)
	}
	cleanup := func(ctx context.Context) { kb.Close(ctx) }
	return kb, cleanup, nil
}

// expandCertOrganizationBox expands the certificate list of the organization.
// There's no unique identifier on the UI tree for the organization box, invoking the click action by JavaScript.
func expandCertOrganizationBox(organizationName string, conn *chrome.Conn, ui *uiauto.Context,
	certificateText *nodewith.Finder) uiauto.Action {
	expandCertList := uiauto.NamedAction("expand the certificate list of the organization",
		func(ctx context.Context) error {
			expr := ClickElementWithConditionJSExpr("cr-expand-button", "certificate-entry", "div.flex", organizationName)
			return webutil.EvalWithShadowPiercer(ctx, conn, expr, nil)
		},
	)
	// If the certificate text visible, it indicates the certificate list is already expanded.
	return uiauto.IfFailThen(
		ui.WithTimeout(3*time.Second).WaitUntilExists(certificateText),
		expandCertList,
	)
}

// OpenCertsPageNewUI opens specific certificate's page in certificate manager.
// func OpenUserInstalledCACertsNewUI(ctx context.Context, ui *uiauto.Context) (retErr error) {
func OpenCertsPageNewUI(ctx context.Context, ui *uiauto.Context, pageType string) (retErr error) {
	var menuItemName, pageLinkName, subpageButtonName string
	switch pageType {
	case userCACertPage:
		menuItemName = "Local certificates"
		pageLinkName = "Installed by you "
		subpageButtonName = "Installed by you subpage back button"
	case adminCACertPage:
		menuItemName = "Local certificates"
		pageLinkName = "Installed by your Administrator"
		subpageButtonName = "Installed by your Administrator subpage back button"
	case userClientCertPage:
		menuItemName = "Your certificates"
		pageLinkName = "View imported certificates"
		subpageButtonName = "Client certificates from platform subpage back button"
	default:
		return errors.Errorf("invalid pageType %q", pageLinkName)
	}

	if err := uiauto.Combine(fmt.Sprintf("Open '%s' page", pageLinkName),
		ui.WaitUntilExists(nodewith.NameStartingWith(menuItemName).Role(role.MenuItem)),
		ui.DoDefault(nodewith.NameStartingWith(menuItemName).Role(role.MenuItem)),
		ui.WaitUntilExists(nodewith.NameStartingWith(pageLinkName).Role(role.Link)),
		ui.DoDefault(nodewith.NameStartingWith(pageLinkName).Role(role.Link)),
		ui.WaitUntilExists(nodewith.Name(subpageButtonName).Role(role.Button)),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to open '%q' page", pageLinkName)
	}
	return nil
}

// OpenUserInstalledCACertsNewUI opens "Installed by you" page in certificate
// manager with CA certs.
func OpenUserInstalledCACertsNewUI(ctx context.Context, ui *uiauto.Context) (retErr error) {
	return OpenCertsPageNewUI(ctx, ui, userCACertPage)
}

// OpenAdminInstalledCACertsNewUI opens "Installed by your Administrator" page
// in certificate manager with CA certs.
func OpenAdminInstalledCACertsNewUI(ctx context.Context, ui *uiauto.Context) (retErr error) {
	return OpenCertsPageNewUI(ctx, ui, adminCACertPage)
}

// OpenUserInstalledClientCertsNewUI opens "View imported certificates" page in certificate
// manager with client certs.
func OpenUserInstalledClientCertsNewUI(ctx context.Context, ui *uiauto.Context) (retErr error) {
	return OpenCertsPageNewUI(ctx, ui, userClientCertPage)
}

// SelectCACertificateImpl selects CA on CA tab and open/close list of certificates
// for the organization.
func SelectCACertificateImpl(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, caOrg, caCertName string) (retErr error) {
	caCertOrg := nodewith.Name(caOrg).Role(role.StaticText)
	caCertNameFinder := nodewith.Ancestor(ManageCertSettingsWebArea).Name(caCertName).Role(role.StaticText)
	if err := uiauto.Combine("select CA from list",
		ui.DoDefault(nodewith.Name("Authorities").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Authorities").ClassName("tab selected")),
		ui.WaitUntilExists(caCertOrg),
		ui.MakeVisible(caCertOrg),
		expandCertOrganizationBox(caOrg, conn, ui, caCertNameFinder),
		ui.WaitUntilExists(caCertNameFinder),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to select CA from list")
	}
	return nil
}

// SelectCACertificate selects CA on CA tab and open/close list of certificates
// for the organization.
func SelectCACertificate(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, caOrg, caCertName string) (retErr error) {
	if err := SelectCACertificateImpl(ctx, ui, conn, caOrg, caCertName); err != nil {
		return errors.Wrap(err, failedToSelectNextUIElementErr)
	}
	return nil
}

// SelectPolicyProvidedCACertificate selects CA on CA tab and open/close list of certificates
// for the organization.
func SelectPolicyProvidedCACertificate(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, caOrg, caCertName string) (retErr error) {
	if err := SelectCACertificateImpl(ctx, ui, conn, caOrg, caCertName); err != nil {
		return errors.Wrap(err, failedToSelectNextUIElementErr)
	}
	return nil
}

// PressEscape is pressing Esc button on keyboard, so some popups can be dismissed.
func PressEscape(ctx context.Context) (retErr error) {
	kb, kbCleanup, err := getKeyboard(ctx)
	defer kbCleanup(ctx)
	if err != nil {
		return errors.Wrap(err, failedToSetupKeyboardErr)
	}

	if err := kb.Accel(ctx, "Esc"); err != nil {
		return errors.Wrap(err, "failed to press Esc")
	}
	return nil
}

// OpenCertificateViewNewUI finds "View" button for the certificate and click on it.
func OpenCertificateViewNewUI(ctx context.Context, ui *uiauto.Context, caOrg string) (retErr error) {
	viewNamePrefix := "View certificate details for " + caOrg
	viewButton := nodewith.NameStartingWith(viewNamePrefix).Role(role.Button).First()
	if err := uiauto.Combine("Open 'View popup' for the certificate",
		ui.WaitUntilExists(viewButton),
		ui.DoDefault(viewButton),
		ui.WaitUntilExists(nodewith.NameStartingWith("Certificate Viewer:").Role(role.RootWebArea)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open 'View popup' for the certificate")
	}
	return nil
}

// ChangeCACertificateTrustNewUI opens "Modification" tab in View popup and flips
// trust value according to `isTrusted` parameter.
func ChangeCACertificateTrustNewUI(ctx context.Context, ui *uiauto.Context, isTrusted bool) (retErr error) {
	var prevTrustState string
	var newTrustState string
	if isTrusted {
		prevTrustState = "Distrusted"
		newTrustState = "Trusted"
	} else {
		prevTrustState = "Trusted"
		newTrustState = "Distrusted"
	}

	if err := uiauto.Combine("Open Modification tab and check trust settings",
		ui.WaitUntilExists(nodewith.Name("Modifications").Role(role.Tab)),
		ui.DoDefault(nodewith.Name("Modifications").Role(role.Tab)),
		ui.WaitUntilExists(nodewith.Name("Trust State").Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name(prevTrustState).Role(role.MenuListOption).Visible()),
		ui.WaitUntilExists(nodewith.Name(newTrustState).Role(role.MenuListOption).Invisible()),
		ui.WaitUntilExists(nodewith.Name("Hint").Role(role.MenuListOption).Invisible()),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open Modification tab and check trust settings")
	}

	// Open drop down list for  trust setting using keyboard. Usage of "DoDefault"
	// will break a visibility for the selected element. The other option was to
	// use LeftClick(), but it is not always working correctly within tablet mode.
	if err := PressTabAndEnter(ctx); err != nil {
		return err
	}

	if err := uiauto.Combine("Modify trust settings for the certificate",
		ui.DoDefault(nodewith.NameContaining(newTrustState).Role(role.MenuListOption)),
		ui.WaitUntilExists(nodewith.NameContaining(newTrustState).Role(role.MenuListOption).Visible()),
		ui.WaitUntilExists(nodewith.NameContaining(prevTrustState).Role(role.MenuListOption).Invisible()),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to modify trust settings for the certificate")
	}
	if err := PressEscape(ctx); err != nil {
		return err
	}
	return nil
}

// SelectEditCACertificate finds "Edit" button from certificate action menu and click on it.
func SelectEditCACertificate(ctx context.Context, ui *uiauto.Context) (retErr error) {
	editButton := nodewith.Name("Edit").Role(role.MenuItem)
	if err := uiauto.Combine("press Edit button for certificate",
		ui.WaitUntilExists(editButton),
		ui.DoDefault(editButton),
		ui.WaitUntilExists(nodewith.NameContaining("Certificate authority").First()),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to press Edit button for CA certificate")
	}
	return nil
}

// setTrustCheckboxAndSave sets CA certificate's trust checkbox to desired state based on
// provided `targetState` parameter and saves result.
func setTrustCheckboxAndSave(ctx context.Context, ui *uiauto.Context, targetState checked.Checked) (retErr error) {
	checkbox := nodewith.Name(TrustCheckboxText).Role(role.CheckBox)
	if err := uiauto.Combine("find CA trust checkbox",
		ui.WaitUntilExists(checkbox),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find CA trust checkbox")
	}

	for {
		info, err := ui.Info(ctx, checkbox)
		if err != nil {
			return errors.Wrap(err, "failed to find CA trust checkbox status")
		}
		if info.Checked == targetState {
			break
		}
		if err := uiauto.Combine(("toggle checkbox value"),
			ui.WaitUntilExists(checkbox.Focusable()),
			ui.EnsureFocused(checkbox),
			ui.WaitForEvent(checkbox, event.CheckedStateChanged, ui.DoDefault(checkbox)),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to set CA trust checkbox")
		}
	}

	if err := PressOkButton(ctx, ui, ManageCertSettingsWebArea); err != nil {
		return errors.Wrap(err, "failed to press OK after CA certificates trust settings")
	}
	return nil
}

// SetCACertTrustNewUI sets trust setting for CA certificate to true or false.
func SetCACertTrustNewUI(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, targetState bool, caOrg string) (retErr error) {
	if err := OpenUserInstalledCACertsNewUI(ctx, ui); err != nil {
		return errors.Wrap(err, "failed to open a page in cert manager")
	}

	if err := OpenCertificateViewNewUI(ctx, ui, caOrg); err != nil {
		return errors.Wrap(err, "failed to open 'View popup'")
	}

	if err := ChangeCACertificateTrustNewUI(ctx, ui, targetState); err != nil {
		return errors.Wrap(err, "failed to modify trust settings")
	}

	return nil
}

// SetCACertTrust sets Web trust setting for CA certificate to true or false.
func SetCACertTrust(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, targetState checked.Checked, caOrg, caCertName string) (retErr error) {
	if err := SelectCACertificate(ctx, ui, conn, caOrg, caCertName); err != nil {
		return errors.Wrap(err, "failed to select CA certificate")
	}
	if err := OpenActionMenuForCACertificate(ctx, ui, conn, caCertName); err != nil {
		return errors.Wrap(err, "failed to open action menu for the certificate")
	}
	if err := SelectEditCACertificate(ctx, ui); err != nil {
		return errors.Wrap(err, "failed to select edit CA certificate")
	}
	if err := setTrustCheckboxAndSave(ctx, ui, targetState); err != nil {
		return errors.Wrap(err, "failed to set CA trust")
	}

	// Hide a list of CA certificates by selecting CA again.
	if err := SelectCACertificate(ctx, ui, conn, caOrg, caCertName); err != nil {
		return errors.Wrap(err, "failed to select CA certificate")
	}
	return nil
}

// clickMoreActionsButton clicks the "More actions" button of the certificate.
// There's no unique identifier on the UI tree for the "More actions" button, invoking the click action by JavaScript.
func clickMoreActionsButton(certName string, conn *chrome.Conn) uiauto.Action {
	return uiauto.NamedAction("click the 'More actions' button of the certificate",
		func(ctx context.Context) error {
			expr := ClickElementWithConditionJSExpr("cr-icon-button#dots", "certificate-subentry", "div.name", certName)
			return webutil.EvalWithShadowPiercer(ctx, conn, expr, nil)
		},
	)
}

// chooseCACertificateImpl selects CA certificate from the list of certificates for CA org.
// It is expected that list of certificates is already expanded.
// Sometimes extra icons are shown before action menu button,
// tabsToPress parameter helps to move through all extra UI elements with "Tab" button.
func chooseCACertificateImpl(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, caCertName string) (retErr error) {
	caCertificateNode := nodewith.Name(caCertName).Role(role.StaticText)
	if err := uiauto.Combine("select CA cert from list",
		ui.WaitUntilExists(caCertificateNode),
		clickMoreActionsButton(caCertName, conn),
		ui.WaitUntilExists(nodewith.Name("View").Role(role.MenuItem)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to select CA cert from list")
	}
	return nil
}

// OpenActionMenuForCACertificate selects specific CA certificate on CA tab and open actions menu for it.
// This is default case when only 1 tab needs to be pressed and dropdown button will be selected.
func OpenActionMenuForCACertificate(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, caCertName string) (retErr error) {
	if err := chooseCACertificateImpl(ctx, ui, conn, caCertName); err != nil {
		return err
	}
	return nil
}

// OpenActionMenuForPolicyProvidedCACertificate selects specific CA certificate on CA tab and open actions menu for it.
// Policy management icon will be shows before the action menu, so "Tab" needs to be pressed 2 times.
func OpenActionMenuForPolicyProvidedCACertificate(ctx context.Context, ui *uiauto.Context, conn *chrome.Conn, caCertName string) (retErr error) {
	if err := chooseCACertificateImpl(ctx, ui, conn, caCertName); err != nil {
		return err
	}
	return nil
}

// ClickElementWithConditionJSExpr returns a JavaScript expression to find and
// click on an element given certain conditions. The expression will search for
// all elements on page using |entrySelector| which is expected to return zero
// or more elements, then it will select internal entries inside of elements
// using query selector |conditionSelector|. Next step expression will check if
// at least one of entries has |conditionText| inside its HTML, and if so will
// attempt to find the button matching |selector| and click it.
func ClickElementWithConditionJSExpr(selector, entrySelector, conditionSelector, conditionText string) string {
	return fmt.Sprintf(`(() => {
		let entries = shadowPiercingQueryAll(%[1]q);
		if (entries == null) { throw new Error("entry elements are not found"); }
		if (!entries.some(entry => {
			let divs = entry.shadowRoot.querySelectorAll(%[2]q)
			if (divs == null) {
				return false;
			}
			let divsArray = Array.from(divs);
			if (divsArray.some(div => { return div.innerHTML.includes(%[3]q) })) {
				let button = entry.shadowRoot.querySelector(%[4]q);
				if (button == null) {
					return false;
				}
				button.click();
				return true;
			}
		})) { throw new Error("no element with a matched condition found"); };
	})()`, entrySelector, conditionSelector, conditionText, selector)
}

// IsNewUIUsed checks that UI contains old "Authorities" header and returns false
// if it is found. All next functions calls will be operating with new UI or with
// old UI.
func IsNewUIUsed(ctx context.Context, ui *uiauto.Context, cr *chrome.Chrome) (status bool, retErr error) {
	// Opening a new tab in browser.
	conn, err := cr.NewConn(ctx, CertificatesPageURL)
	if err != nil {
		return false, err
	}
	defer conn.CloseTarget(ctx)

	if err := uiauto.Combine("test that Authorities are present in UI",
		ui.WithTimeout(3*time.Second).WaitUntilExists(nodewith.Name("Authorities").Role(role.Tab)),
	)(ctx); err != nil {
		return true, nil
	}
	return false, nil
}

// IsNewUIActive fetches isNewUI value from the ctx and returns it as a bool.
func IsNewUIActive(ctx context.Context) bool {
	return ctx.Value(IsNewUI).(bool)
}

// DeleteUserCertNewUI selects and deletes specific client's certificate from "Your certificates" tab.
func DeleteUserCertNewUI(ctx context.Context, ui *uiauto.Context, userCaCertName string) (retErr error) {
	if err := OpenUserInstalledClientCertsNewUI(ctx, ui); err != nil {
		return errors.Wrap(err, "failed to select client certificates tab")
	}

	dialogMessage := nodewith.NameStartingWith("If you delete one of").Role(role.StaticText)
	deleteNamePrefix := "Delete certificate " + userCaCertName
	deleteButton := nodewith.NameStartingWith(deleteNamePrefix).Role(role.Button)
	popupDialog := nodewith.NameStartingWith("Delete ").Role(role.Dialog)

	if err := uiauto.Combine("delete user cert",
		ui.WaitUntilExists(deleteButton),
		ui.DoDefault(deleteButton),
		ui.WaitUntilExists(popupDialog),
		ui.WaitUntilExists(dialogMessage),
		ui.WaitUntilExists(nodewith.Name("Cancel").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name("OK").Role(role.Button)),
		ui.DoDefault(nodewith.Name("OK").Role(role.Button)),
		ui.WaitUntilGone(popupDialog),
	)(ctx); err != nil {
		return errors.Wrap(err, failedToDeleteCertErr)
	}
	return nil
}

// ImportAndBindClientCertNewUI uses the Import and Bind button on the chrome://settings/certificates page
// to manually import the client certificate from file.
func ImportAndBindClientCertNewUI(ctx context.Context, ui *uiauto.Context, clientCertFileName, certFilePassword string) (retErr error) {
	buttonNameForImport := "Import and Bind to Client certificates from platform"
	return ImportClientCertImplNewUI(ctx, ui, clientCertFileName, certFilePassword, buttonNameForImport)
}

// ImportClientCertNewUI uses the Import button on the chrome://settings/certificates
// page to manually import the client certificate from file.
func ImportClientCertNewUI(ctx context.Context, ui *uiauto.Context, clientCertFileName, certFilePassword string) (retErr error) {
	buttonNameForImport := "Import to Client certificates from platform"
	return ImportClientCertImplNewUI(ctx, ui, clientCertFileName, certFilePassword, buttonNameForImport)
}

// ImportClientCertImplNewUI uses the provided buttonName on the
// chrome://settings/certificates page to manually import
// the client certificate from file.
func ImportClientCertImplNewUI(ctx context.Context, ui *uiauto.Context, clientCertFileName, certFilePassword, buttonName string) (retErr error) {
	kb, kbCleanup, err := getKeyboard(ctx)
	defer kbCleanup(ctx)
	if err != nil {
		return errors.Wrap(err, failedToSetupKeyboardErr)
	}

	passwordDialog := nodewith.Name("Enter certificate password").Role(role.Dialog)
	passwordTextBox := nodewith.Role(role.TextField).Editable()

	if err := OpenUserInstalledClientCertsNewUI(ctx, ui); err != nil {
		return errors.Wrap(err, "failed to select client certificates tab")
	}
	if err := uiauto.Combine("import client cert",
		ui.WaitUntilExists(nodewith.Name(buttonName).Role(role.Button)),
		ui.DoDefault(nodewith.Name(buttonName).Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name(clientCertFileName).Role(role.StaticText)),
		ui.DoDefault(nodewith.Name(clientCertFileName).Role(role.StaticText)),
		ui.WaitUntilExists(nodewith.Name("Open").Role(role.Button).State("focusable", true)),
		ui.DoDefault(nodewith.Name("Open").Role(role.Button)),
		ui.WaitUntilExists(passwordTextBox.Ancestor(passwordDialog).State("focusable", true)),
		ui.DoDefault(passwordTextBox.Ancestor(passwordDialog)),
		kb.TypeAction(certFilePassword),
		kb.AccelAction("enter"),
		ui.WithTimeout(3*time.Second).WaitUntilGone(passwordTextBox.Ancestor(passwordDialog)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to import client cert")
	}

	return nil
}

// IsClientCertImportedNewUI checks if client certificate is present in the list of imported certificates.
func IsClientCertImportedNewUI(ctx context.Context, ui *uiauto.Context, clientOrg string) (bool, error) {
	if err := OpenUserInstalledClientCertsNewUI(ctx, ui); err != nil {
		return false, err
	}

	if err := uiauto.Combine("check client cert",
		ui.WithTimeout(10*time.Second).WaitUntilExists(nodewith.NameContaining(clientOrg).First()),
	)(ctx); err != nil {
		return false, nil
	}
	return true, nil
}
