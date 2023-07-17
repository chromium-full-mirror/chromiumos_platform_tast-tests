// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package certificate implements certificate-related operations through the Certificates Manager.
package certificate

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filepicker"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// Manager holds the resources required to operate the Certificates Manager.
type Manager struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
	conn  *chrome.Conn
}

// CertType describes the type of certificate.
type CertType string

// These are the certificate types supported by the Certificates Manager.
const (
	TypeClient CertType = "Your certificates"
	TypeCA     CertType = "Authorities"
)

// ImportType represents which action to do when importing a client certificate.
type ImportType string

const (
	// TypeImport represents only importing client certificates.
	TypeImport ImportType = "Import"
	// TypeImportAndBind represents importing client certificates, and binding them to the device.
	TypeImportAndBind ImportType = "Import and Bind"
)

// CATrustSettings represents the kinds of trust settings of a CA certificate.
type CATrustSettings int

// These are the trust-setting options when importing a CA certificate.
const (
	TrustForWebsites CATrustSettings = 1 << iota
	TrustForEmailUsers
	TrustForSoftwareMakers
)

var (
	certFinder = nodewith.Ancestor(nodewith.Name("Settings - Manage certificates").Role(role.RootWebArea))

	trustForWebsitesCheckBox       = certFinder.Name("Trust this certificate for identifying websites").Role(role.CheckBox)
	trustForEmailUsersCheckBox     = certFinder.Name("Trust this certificate for identifying email users").Role(role.CheckBox)
	trustForSoftwareMakersCheckBox = certFinder.Name("Trust this certificate for identifying software makers").Role(role.CheckBox)
)

func (s CATrustSettings) clickCheckBoxesAction(ui *uiauto.Context) uiauto.Action {
	var actions []uiauto.Action
	for trustSettings, checkbox := range map[CATrustSettings]*nodewith.Finder{
		TrustForWebsites:       trustForWebsitesCheckBox,
		TrustForEmailUsers:     trustForEmailUsersCheckBox,
		TrustForSoftwareMakers: trustForSoftwareMakersCheckBox,
	} {
		if (s & trustSettings) != 0 {
			actions = append(actions, ui.LeftClick(checkbox), ui.WaitUntilCheckedState(checkbox, true))
		}
	}
	return uiauto.Combine("click trust settings check boxes", actions...)
}

const certManagerURL = "chrome://settings/certificates"

// Launch launches the Certificates Manager and returns a Manager instance.
// Close should be explicitly called to close the Certificates Manager.
func Launch(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser) (*Manager, error) {
	conn, err := br.NewConn(ctx, certManagerURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch the Certificates Manager")
	}
	return &Manager{
		tconn: tconn,
		ui:    uiauto.New(tconn),
		conn:  conn,
	}, nil
}

// Connect connects to an existing Certificates Manager page.
// The Certificates Manager page should be opened before calling this function.
// The current connection to the Certificates Manager page would be closed before creating a new connection.
func (m *Manager) Connect(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser) error {
	conn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(certManagerURL))
	if err != nil {
		return errors.Wrap(err, "failed to connect to the Certificates Manager")
	}

	if m.conn != nil {
		m.conn.Close()
	}
	m.tconn, m.ui, m.conn = tconn, uiauto.New(tconn), conn
	return nil
}

// Close closes the Certificates Manager page and the connection to the page.
func (m *Manager) Close(ctx context.Context) error {
	if m.conn == nil {
		return errors.New("the connection is already closed")
	}

	if err := m.conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close the Certificates Manager page")
	}
	if err := m.conn.Close(); err != nil {
		return errors.Wrap(err, "failed to close the connection to the Certificates Manager page")
	}
	m.conn = nil

	return nil
}

// Organization represents the organization of a certificate.
type Organization struct{ Name string }

// displayName returns the organization name displayed in the Certificates Manager.
func (o Organization) displayName() string { return fmt.Sprintf("org-%s", o.Name) }

// ImportClientCert imports the client certificate from the "Downloads" folder with the specified import type in the Certificates Manager.
// The certificate file has to be located in the "Downloads" folder before calling this method.
func (m *Manager) ImportClientCert(fileName, password, certName string, org Organization, importType ImportType) uiauto.Action {
	organizationText := certFinder.Name(org.displayName()).Role(role.StaticText)
	certificateText := certFinder.Name(certName).Role(role.StaticText)

	if importType == TypeImportAndBind {
		// There will be "hardware backed" next to the certificate if it is bound.
		certificateText = certFinder.Name(fmt.Sprintf("%s (hardware-backed)", certName)).Role(role.StaticText)
	}

	return func(ctx context.Context) error {
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create keyboard")
		}
		defer kb.Close(ctx)

		return uiauto.Combine(fmt.Sprintf("import client certificate %q", org.Name),
			m.switchToCertTab(TypeClient),
			m.ui.LeftClick(certFinder.Name(string(importType)).Role(role.Button)),
			uploadFile(m.tconn, fileName),
			m.ui.LeftClick(certFinder.Name("Password").Role(role.TextField).Editable()),
			kb.TypeAction(password),
			// The OK button might be covered by the virtual keyboard on tablet devices.
			// Interact with the button through DoDefault to ensure the button is clicked.
			m.ui.DoDefault(certFinder.Name("OK").Role(role.Button)),
			m.ui.WaitUntilExists(organizationText),
			m.ui.MakeVisible(organizationText),
			m.expandCertOrganizationBox(org.displayName(), certificateText),
			m.ui.WaitUntilExists(certificateText),
		)(ctx)
	}
}

// ImportCACert imports the CA certificate from the "Downloads" folder by the "Import" button in the Certificates Manager.
// The certificate file has to be located in the "Downloads" folder before calling this method.
func (m *Manager) ImportCACert(fileName string, org Organization, trustSettings CATrustSettings) uiauto.Action {
	importButton := certFinder.Name("Import").Role(role.Button)
	okButton := certFinder.Name("OK").Role(role.Button)
	organizationText := certFinder.Name(org.displayName()).Role(role.StaticText)

	return uiauto.Combine(fmt.Sprintf("import CA certificate %q", org.Name),
		m.switchToCertTab(TypeCA),
		m.ui.LeftClick(importButton),
		uploadFile(m.tconn, fileName),
		trustSettings.clickCheckBoxesAction(m.ui),
		m.ui.LeftClick(okButton),
		m.ui.WaitUntilExists(organizationText),
	)
}

// DeleteCert deletes the certificate from the Certificates Manager.
func (m *Manager) DeleteCert(name string, org Organization, certType CertType) uiauto.Action {
	organizationText := certFinder.Name(org.displayName()).Role(role.StaticText)
	// There will be a suffix in the name of the bound client certificate.
	// Use NameStartingWith to cover all kinds of certificates.
	certificateText := certFinder.NameStartingWith(name).Role(role.StaticText)

	return uiauto.Combine(fmt.Sprintf("delete certificate %q in the organization %q", name, org.Name),
		m.switchToCertTab(certType),
		m.ui.WaitUntilExists(organizationText),
		m.ui.MakeVisible(organizationText),
		m.expandCertOrganizationBox(org.displayName(), certificateText),
		m.ui.WaitUntilExists(certificateText),
		m.clickMoreActionsButton(name),
		m.ui.LeftClick(nodewith.Name("Delete").Role(role.MenuItem)),
		m.ui.LeftClick(certFinder.Name("OK").Role(role.Button)),
		m.ui.WaitUntilGone(certificateText),
	)
}

// IsCertImported returns whether a specified certificate has been installed in the Certificates Manager.
func (m *Manager) IsCertImported(ctx context.Context, name string, org Organization, certType CertType) (bool, error) {
	if err := m.switchToCertTab(certType)(ctx); err != nil {
		return false, err
	}

	organizationText := certFinder.Name(org.displayName()).Role(role.StaticText)
	if err := m.ui.WaitUntilExists(organizationText)(ctx); err != nil {
		if nodewith.IsNodeNotFoundErr(err) {
			return false, nil
		}
		return false, err
	}

	certificateText := certFinder.NameStartingWith(name).Role(role.StaticText)
	if err := m.expandCertOrganizationBox(org.displayName(), certificateText)(ctx); err != nil {
		return false, err
	}

	err := m.ui.WaitUntilExists(certificateText)(ctx)
	if err != nil {
		return m.ui.IsNodeFound(ctx, certificateText)
	}
	return true, nil
}

// switchToCertTab switches to the specified tab in the Certificates Manager.
func (m *Manager) switchToCertTab(certType CertType) uiauto.Action {
	certTab := certFinder.Name(string(certType)).Role(role.Tab)
	certTabSelected := certTab.HasClass("selected")
	return uiauto.Combine(fmt.Sprintf("switch to %q tab", certType),
		m.ui.WaitUntilExists(certTab),
		m.ui.MakeVisible(certTab),
		m.ui.LeftClick(certTab),
		m.ui.WaitUntilExists(certTabSelected),
	)
}

// expandCertOrganizationBox expands the certificate list of the organization.
// There's no unique identifier on the UI tree for the organization box, invoking the click action by JavaScript.
func (m *Manager) expandCertOrganizationBox(organizationName string, certificateText *nodewith.Finder) uiauto.Action {
	expandCertList := uiauto.NamedAction("expand the certificate list of the organization",
		func(ctx context.Context) error {
			expr := clickElementWithConditionJSExpr("cr-expand-button", "certificate-entry", "div.flex", organizationName)
			return webutil.EvalWithShadowPiercer(ctx, m.conn, expr, nil)
		},
	)
	// If the certificate text exists, it indicates the certificate list is already expanded.
	return uiauto.IfFailThen(
		m.ui.WithTimeout(5*time.Second).WaitUntilExists(certificateText),
		expandCertList,
	)
}

// clickMoreActionsButton clicks the "More actions" button of the certificate.
// There's no unique identifier on the UI tree for the "More actions" button, invoking the click action by JavaScript.
func (m *Manager) clickMoreActionsButton(certName string) uiauto.Action {
	return uiauto.NamedAction("click the 'More actions' button of the certificate",
		func(ctx context.Context) error {
			expr := clickElementWithConditionJSExpr("cr-icon-button#dots", "certificate-subentry", "div.name", certName)
			return webutil.EvalWithShadowPiercer(ctx, m.conn, expr, nil)
		},
	)
}

// clickElementWithConditionJSExpr returns a JavaScript expression to find and click on an element given certain conditions.
// The expression will search for elements using |entrySelector| which is expected to return zero or more <div> elements.
// The expression will check if at least one of these elements has |conditionText| inside its HTML,
// and if so will attempt to find the button matching |selector| and click it.
func clickElementWithConditionJSExpr(selector, entrySelector, conditionSelector, conditionText string) string {
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

// uploadFile selects the specified file and uploads it to the Certificates Manager.
func uploadFile(tconn *chrome.TestConn, fileName string) uiauto.Action {
	return func(ctx context.Context) error {
		filePicker, err := filepicker.Find(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to find the file picker")
		}
		return uiauto.Combine(fmt.Sprintf("upload file %q", fileName),
			filePicker.OpenDir(filesapp.Downloads, filesapp.Downloads),
			filePicker.OpenFile(fileName),
		)(ctx)
	}
}
