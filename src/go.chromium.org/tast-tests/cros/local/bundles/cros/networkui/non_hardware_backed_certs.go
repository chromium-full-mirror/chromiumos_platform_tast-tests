// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/dropdown"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	certManager "go.chromium.org/tast-tests/cros/local/networkui/certificate"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Import certificate is a series of UI actions which could take a while.
	importCertsTimeout = time.Minute

	// Delete certificate is a series of UI actions which could take a while.
	deleteCertTimeout = time.Minute
)

type nonHardwareBackedCertsTestResource struct {
	cr     *chrome.Chrome
	br     *browser.Browser
	tconn  *chrome.TestConn
	outDir string

	certStore   certificate.CertStore
	clientCerts *certManager.CertData
	caCerts     *certManager.CertData
	certsName   string
}

type nonHardwareBackedCertsVerifyFunc func(context.Context, *nonHardwareBackedCertsTestResource) error

func init() {
	testing.AddTest(&testing.Test{
		Func:         NonHardwareBackedCerts,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the behavior of certificates which are non-hardware backed",
		Contacts: []string{
			// "cros-connectivity@google.com",
			// "chromeos-connectivity-engprod@google.com",
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Attr:           []string{"group:network", "network_e2e_unstable"},
		TestBedDeps:    []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		SoftwareDeps:   []string{"chrome"},
		Fixture:        fixture.ChromeLoggedIn,
		Params: []testing.Param{
			{
				Name: "are_disabled_from_ui",
				Val: []nonHardwareBackedCertsVerifyFunc{
					certsAreDisabledFromWifiDialog,
					certsAreDisabledFromVPNDialog,
				},
				Timeout: 2*time.Minute + importCertsTimeout + deleteCertTimeout,
			}, {
				Name: "can_be_exported",
				Val: []nonHardwareBackedCertsVerifyFunc{
					certsCanBeExported,
				},
				Timeout: 2*time.Minute + importCertsTimeout + deleteCertTimeout,
			},
		},
	})
}

// NonHardwareBackedCerts verifies the behavior of certificates which are non-hardware backed.
func NonHardwareBackedCerts(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve test API connection: ", err)
	}

	deleteCertsCtx := ctx
	ctx, cancelDeleteCertsCtx := ctxutil.Shorten(ctx, deleteCertTimeout)
	defer cancelDeleteCertsCtx()

	res := newNonHardwareBackedCertsTestResource(cr, cr.Browser(), tconn, certificate.TestCert1(), s.OutDir())
	if err := certManager.CreateCertAndImport(ctx, cr, tconn, browser.TypeAsh, certificate.TestCert1(), certManager.TypeImport, "" /* password */, 0 /* trustSettings */); err != nil {
		s.Fatal("Failed to create and import certificates: ", err)
	}
	defer certManager.DeleteCert(tconn, cr.Browser(), res.clientCerts, res.caCerts)(deleteCertsCtx)

	verifies := s.Param().([]nonHardwareBackedCertsVerifyFunc)
	for _, verify := range verifies {
		if err := verify(ctx, res); err != nil {
			s.Fatal("Failed to conduct the test: ", err)
		}
	}
}

func newNonHardwareBackedCertsTestResource(cr *chrome.Chrome, br *browser.Browser, tconn *chrome.TestConn, certs certificate.CertStore, outDir string) *nonHardwareBackedCertsTestResource {
	client := certManager.NewCertData(certs, certManager.TypeClient)
	ca := certManager.NewCertData(certs, certManager.TypeCA)
	return &nonHardwareBackedCertsTestResource{
		cr:          cr,
		br:          br,
		tconn:       tconn,
		outDir:      outDir,
		certStore:   certs,
		clientCerts: client,
		caCerts:     ca,
		certsName:   fmt.Sprintf("%s [%s]", ca.Name(), client.Name()),
	}
}

func certsAreDisabledFromWifiDialog(ctx context.Context, res *nonHardwareBackedCertsTestResource) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	settings, err := ossettings.OpenJoinWiFiDialog(ctx, res.cr, res.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to open join WiFi dialog")
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, res.cr, "wifi_dialog_ui_dump")

	// Perform the test through "EAP-TLS", note that this is only a random choice.
	dialogNodeFinder := nodewith.Ancestor(ossettings.JoinWiFiNetworkDialog)
	if err := uiauto.Combine("select EAP-TLS",
		dropdown.SelectDropDownOption(res.tconn, dialogNodeFinder.Name("Security").Role(role.ComboBoxSelect), "EAP"),
		dropdown.SelectDropDownOption(res.tconn, dialogNodeFinder.Name("EAP method").Role(role.ComboBoxSelect), "EAP-TLS"),
	)(ctx); err != nil {
		return err
	}

	const prompt = "User certificate is not available for network authentication."
	const dropdownBoxName = "User certificate"
	return uiauto.Combine("verify certs are disabled",
		// Expecting a prompt display directly on the dialog.
		settings.WaitUntilExists(dialogNodeFinder.Name(prompt).Role(role.StaticText)),
		// Expand the box just for capture screenshot on failure.
		dropdown.ExpandDropDown(res.tconn, dialogNodeFinder.Name(dropdownBoxName).Role(role.ComboBoxSelect)),
		verifyDropdownOptionIsDisabled(res.cr, settings, dropdownBoxName, res.certsName),
	)(ctx)
}

func certsAreDisabledFromVPNDialog(ctx context.Context, res *nonHardwareBackedCertsTestResource) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	settings, err := ossettings.OpenJoinVPNDialog(ctx, res.tconn, res.cr)
	if err != nil {
		return errors.Wrap(err, "failed to open join VPN dialog")
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, res.cr, "vpn_dialog_ui_dump")

	// Perform the test through "OpenVPN", note that this is only a random choice.
	dialogNodeFinder := nodewith.Ancestor(ossettings.JoinVPNNetworkDialog)
	if err := dropdown.SelectDropDownOption(res.tconn, dialogNodeFinder.Name("Provider type").Role(role.ComboBoxSelect), "OpenVPN")(ctx); err != nil {
		return errors.Wrap(err, "failed to select OpenVPN")
	}

	const dropdownBoxName = "User certificate"
	return uiauto.Combine("expand the dropdown box and verify",
		// Expand the box just for capture screenshot on failure.
		dropdown.ExpandDropDown(res.tconn, dialogNodeFinder.Name(dropdownBoxName).Role(role.ComboBoxSelect)),
		verifyDropdownOptionIsDisabled(res.cr, settings, dropdownBoxName, res.certsName),
	)(ctx)
}

func certsCanBeExported(ctx context.Context, res *nonHardwareBackedCertsTestResource) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	manager, err := certManager.Launch(ctx, res.tconn, res.br)
	if err != nil {
		return errors.Wrap(err, "failed to launch the certificates manager")
	}
	defer manager.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, res.cr, "certs_can_export_ui_dump")

	clientCertExportName := fmt.Sprintf("%s_test_exported", res.clientCerts.Name())
	if err := manager.ExportCert(res.clientCerts.Name(), clientCertExportName, res.clientCerts.Organization(), res.clientCerts.CertType())(ctx); err != nil {
		return errors.Wrap(err, "failed to export certificate")
	}

	path, err := cryptohome.DownloadsPath(ctx, res.cr.User())
	if err != nil {
		return err
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		outFiles, err := testexec.CommandContext(ctx, "ls", filepath.Join(path, clientCertExportName)).Output()
		if err != nil {
			return err
		}
		if !strings.Contains(string(outFiles), clientCertExportName) {
			return errors.New("didn't find the exported certificate file")
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second})
}

func verifyDropdownOptionIsDisabled(cr *chrome.Chrome, settings *ossettings.OSSettings, dropdownBoxName, optionName string) uiauto.Action {
	return func(ctx context.Context) error {
		// The attributes of a dropdown option are not available in the UI tree,
		// so they can only be accessed through JavaScript expressions.
		expr := fmt.Sprintf(`
			var selectNode = shadowPiercingQuery('select.md-select[aria-label="%s"]');
			var optionNode = Array.from(selectNode.options).find(option => option.text.includes('%s'));
			optionNode && optionNode.disabled;
		`, dropdownBoxName, optionName)

		var isOptionDisabled bool
		if err := settings.EvalJSWithShadowPiercer(ctx, cr, expr, &isOptionDisabled); err != nil {
			return errors.Wrap(err, "failed to verify if the option is disabled")
		}
		if !isOptionDisabled {
			return errors.New("the user certificate is not disabled")
		}
		return nil
	}
}
