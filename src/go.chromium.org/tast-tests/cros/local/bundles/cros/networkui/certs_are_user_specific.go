// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	certManager "go.chromium.org/tast-tests/cros/local/bundles/cros/networkui/certificate"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/dropdown"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type certsAreUserSpecificTestParams struct {
	surfaceUnderTest certsAreUserSpecificTestUI
	browserType      browser.Type
}

const (
	// In each test scenario, login and import certificates take 1 minute and up to 1 time,
	// login and verify the surface take 2 minutes each and up to 2 times.
	// Therefore, each scenario requires a 5 minute execution time.
	testScenarioTimeout = 5 * time.Minute
	// The clean up action requires a longer time due to it involves chrome login and series of UI operations.
	cleanUpReserveTimeout = 2 * time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CertsAreUserSpecific,
		// This test launches a web page so there should be a lacros variant.
		// Lacros test will be added once the issue(crbug/1366609) is fixed.
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verify that the imported certificates are user specific",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent:   "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:network", "network_e2e"},
		SoftwareDeps:   []string{"chrome"},
		// Import binding and non-binding certs from regular user.
		// Import non-binding certs from guest user.
		Timeout: 3 * (testScenarioTimeout + cleanUpReserveTimeout),
		Params: []testing.Param{
			{
				Name: "certificate_manager",
				Val: &certsAreUserSpecificTestParams{
					surfaceUnderTest: &uiCertManager{},
					browserType:      browser.TypeAsh,
				},
			}, {
				Name: "join_vpn_dialog",
				Val: &certsAreUserSpecificTestParams{
					surfaceUnderTest: &uiJoinVPN{},
					browserType:      browser.TypeAsh,
				},
			}, {
				Name: "join_wifi_dialog",
				Val: &certsAreUserSpecificTestParams{
					surfaceUnderTest: &uiJoinWiFi{},
					browserType:      browser.TypeAsh,
				},
			},
		},
	})
}

// CertsAreUserSpecific verifies that the imported certificates are user specific.
func CertsAreUserSpecific(ctx context.Context, s *testing.State) {
	var (
		params            = s.Param().(*certsAreUserSpecificTestParams)
		primaryTestUser   = &certsAreUserSpecificTestUser{loginOption: chrome.FakeLogin(chrome.Creds{User: "testuser1@gmail.com", Pass: "testpass"})}
		secondaryTestUser = &certsAreUserSpecificTestUser{loginOption: chrome.FakeLogin(chrome.Creds{User: "testuser2@gmail.com", Pass: "testpass"})}
		testGuestUser     = &certsAreUserSpecificTestUser{loginOption: chrome.GuestLogin(), isGuest: true}
	)

	for _, test := range []struct {
		name       string
		importType certManager.ImportType
		user       *certsAreUserSpecificTestUser
	}{
		{
			name:       "Import from regular user",
			importType: certManager.TypeImport,
			user:       primaryTestUser,
		}, {
			name:       "Import and Bind from regular user",
			importType: certManager.TypeImportAndBind,
			user:       primaryTestUser,
		}, {
			name:       "Import from guest user",
			importType: certManager.TypeImport,
			user:       testGuestUser,
		},
	} {
		s.Run(ctx, test.name, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			// The clean up action requires a longer time due to it involves chrome login and series of UI operations.
			ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
			defer cancel()

			certs, err := test.user.loginAndImportCerts(ctx, certificate.TestCert1(), params.browserType, test.importType)
			if err != nil {
				s.Fatal("Failed to import certs: ", err)
			}
			defer test.user.loginAndDeleteCerts(cleanupCtx, certs, params.browserType)

			res := &certsAreUserSpecificTestResource{
				testCerts:   certs,
				browserType: params.browserType,
				outDir:      s.OutDir(),
			}

			for _, user := range []*certsAreUserSpecificTestUser{secondaryTestUser, testGuestUser} {
				// Isolate the step to leverage `defer` pattern.
				func(ctx context.Context) {
					cleanupCtx := ctx
					ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
					defer cancel()

					var err error
					if res.cr, err = browserfixt.NewChrome(ctx, res.browserType, lacrosfixt.NewConfig(), user.loginOption, chrome.KeepState()); err != nil {
						s.Fatal("Failed to start Chrome: ", err)
					}
					defer res.cr.Close(cleanupCtx)

					if res.tconn, err = res.cr.TestAPIConn(ctx); err != nil {
						s.Fatal("Failed to retrieve test API connection: ", err)
					}

					surface := params.surfaceUnderTest
					if err := surface.launchUI(ctx, res); err != nil {
						s.Fatalf("Failed to launch %q: %v", surface.uiName(), err)
					}
					defer surface.closeUI(cleanupCtx)

					if err := surface.verifyNotInstalled(ctx, res); err != nil {
						s.Fatal("Failed to verify the certificates are not installed: ", err)
					}
				}(ctx)
			}
		})
	}
}

// verifyNotInstalledByUI verifies that the certificates are not installed in the UI.
// This function will not open any UI/app, the UI/app has to be opened in advance.
func verifyNotInstalledByUI(ctx context.Context, tconn *chrome.TestConn, certs []*certManager.CertData, uiRoot *nodewith.Finder) error {
	certDropdownMenuNames := map[certManager.CertType]string{
		certManager.TypeCA:     "Server CA certificate",
		certManager.TypeClient: "User certificate",
	}

	for _, cert := range certs {
		dropdownNode := nodewith.Name(certDropdownMenuNames[cert.CertType()]).Role(role.ComboBoxSelect).Ancestor(uiRoot)
		dropDownItems, err := dropdown.Values(ctx, tconn, dropdownNode)
		if err != nil {
			return errors.Wrap(err, "failed to get dropdown values")
		}

		for _, itemName := range dropDownItems {
			if strings.Contains(itemName, cert.Name()) {
				return errors.Errorf("certificate %q is installed", itemName)
			}
		}
	}
	return nil
}

type certsAreUserSpecificTestResource struct {
	cr          *chrome.Chrome
	tconn       *chrome.TestConn
	testCerts   []*certManager.CertData
	browserType browser.Type
	outDir      string
}

type certsAreUserSpecificTestUI interface {
	uiName() string
	uiRoot() *nodewith.Finder
	launchUI(context.Context, *certsAreUserSpecificTestResource) error
	verifyNotInstalled(context.Context, *certsAreUserSpecificTestResource) error
	closeUI(context.Context) error
}

type uiJoinVPN struct{ *ossettings.OSSettings }

func (uiJoinVPN) uiName() string { return "Join VPN dialog" }
func (uiJoinVPN) uiRoot() *nodewith.Finder {
	return nodewith.NameContaining("Join VPN network").Role(role.Dialog)
}

func (n *uiJoinVPN) launchUI(ctx context.Context, resource *certsAreUserSpecificTestResource) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	var err error
	if n.OSSettings, err = ossettings.Launch(ctx, resource.tconn); err != nil {
		return errors.Wrap(err, "failed to launch OS settings at network page")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			faillog.DumpUITreeWithScreenshotOnError(ctx, resource.outDir, func() bool { return retErr != nil }, resource.cr, "join_VPN_dialog")
			n.closeUI(ctx)
		}
	}(cleanupCtx)

	return uiauto.Combine("open VPN dialog",
		// Using DoDefault to avoid inaccurate location issue.
		n.DoDefault(nodewith.Name("Add network connection").Role(role.Button)),
		n.DoDefault(nodewith.NameContaining("Add built-in VPN").Role(role.Button)),
		n.WaitForLocation(n.uiRoot()),
	)(ctx)
}

func (n *uiJoinVPN) verifyNotInstalled(ctx context.Context, resource *certsAreUserSpecificTestResource) error {
	return verifyNotInstalledByUI(ctx, resource.tconn, resource.testCerts, n.uiRoot())
}

func (n *uiJoinVPN) closeUI(ctx context.Context) error {
	return n.Close(ctx)
}

type uiJoinWiFi struct{ *chrome.Conn }

func (uiJoinWiFi) uiName() string { return "Join Wi-Fi dialog" }
func (uiJoinWiFi) uiRoot() *nodewith.Finder {
	return nodewith.NameContaining("Join Wi-Fi network").Role(role.Dialog)
}

func (n *uiJoinWiFi) launchUI(ctx context.Context, resource *certsAreUserSpecificTestResource) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, resource.tconn); err != nil {
		return errors.Wrap(err, "failed to navigate to network detailed view within the Quick Settings")
	}
	defer quicksettings.Hide(cleanupCtx, resource.tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, resource.outDir, func() bool { return retErr != nil }, resource.cr, "quick_settings")

	ui := uiauto.New(resource.tconn)
	joinWiFiButton := nodewith.Name("Join Wi-Fi network").Role(role.Button).Ancestor(quicksettings.QsRootFinder)
	if err := uiauto.Combine("click join WiFi button to launch the join WiFi dialog",
		ui.DoDefault(joinWiFiButton),
		ui.WaitUntilExists(n.uiRoot()),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the join WiFi button")
	}

	// Leverage `conn.Close()` to close the "Join Wi-Fi network" dialog.
	conn, err := resource.cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix("chrome://internet-config-dialog"))
	if err != nil {
		return errors.Wrap(err, "failed to create new connection")
	}
	n.Conn = conn

	defer func(ctx context.Context) {
		if retErr != nil {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, resource.outDir, func() bool { return true }, resource.cr, "join_WiFi_dialog")
			n.closeUI(ctx)
		}
	}(cleanupCtx)

	// EAP-TLS network requires both client and CA certificates.
	return uiauto.Combine("set security and EAP method",
		dropdown.SelectDropDownOption(resource.tconn, nodewith.Name("Security").Role(role.ComboBoxSelect).Ancestor(n.uiRoot()), "EAP"),
		dropdown.SelectDropDownOption(resource.tconn, nodewith.Name("EAP method").Role(role.ComboBoxSelect).Ancestor(n.uiRoot()), "EAP-TLS"),
	)(ctx)
}

func (n *uiJoinWiFi) verifyNotInstalled(ctx context.Context, resource *certsAreUserSpecificTestResource) error {
	return verifyNotInstalledByUI(ctx, resource.tconn, resource.testCerts, n.uiRoot())
}

func (n *uiJoinWiFi) closeUI(ctx context.Context) error {
	defer n.Close()
	return n.CloseTarget(ctx)
}

type uiCertManager struct{ *certManager.Manager }

func (uiCertManager) uiName() string           { return "Certificate Manager" }
func (uiCertManager) uiRoot() *nodewith.Finder { return nil }

func (n *uiCertManager) launchUI(ctx context.Context, resource *certsAreUserSpecificTestResource) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	browser, closeBrowser, err := browserfixt.SetUp(ctx, resource.cr, resource.browserType)
	if err != nil {
		return errors.Wrap(err, "failed to set up browser")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			closeBrowser(ctx)
		}
	}(cleanupCtx)

	n.Manager, err = certManager.Launch(ctx, resource.tconn, browser)
	return err
}

func (n *uiCertManager) verifyNotInstalled(ctx context.Context, resource *certsAreUserSpecificTestResource) error {
	for _, cert := range resource.testCerts {
		if isImported, err := n.IsCertImported(ctx, cert.Name(), cert.Organization(), cert.CertType()); err != nil {
			return errors.Wrapf(err, "failed to check if %q certificate is imported", cert.CertType())
		} else if isImported {
			return errors.Errorf("unexpected %q certificate imported state; got: %v, want: %v", cert.CertType(), isImported, false)
		}
	}
	return nil
}

func (n *uiCertManager) closeUI(ctx context.Context) error {
	return n.Close(ctx)
}

type certsAreUserSpecificTestUser struct {
	loginOption chrome.Option
	isGuest     bool
}

func (u *certsAreUserSpecificTestUser) loginAndImportCerts(ctx context.Context, certs certificate.CertStore, browserType browser.Type, importType certManager.ImportType) (_ []*certManager.CertData, retErr error) {
	// Reserve a longer time in case the certificate needs to be deleted.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr, browser, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, browserType, lacrosfixt.NewConfig(), u.loginOption)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome and launch browser")
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating test API connection")
	}

	manager, err := certManager.Launch(ctx, tconn, browser)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch certificate manager")
	}
	defer manager.Close(cleanupCtx)

	if err := manager.CreateCertAndImport(ctx, cr, browserType, certs, importType, "" /* password */, 0 /* trustSettings */); err != nil {
		return nil, errors.Wrap(err, "failed to import certificates")
	}

	return []*certManager.CertData{
		certManager.NewCertData(certs, certManager.TypeCA),
		certManager.NewCertData(certs, certManager.TypeClient),
	}, nil
}

func (u *certsAreUserSpecificTestUser) loginAndDeleteCerts(ctx context.Context, certs []*certManager.CertData, browserType browser.Type) error {
	if u.isGuest {
		// No further action is required as the certificates will become invalid upon exiting guest mode.
		return nil
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, browser, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, browserType, lacrosfixt.NewConfig(), u.loginOption, chrome.KeepState())
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome and launch browser")
	}
	defer cr.Close(cleanupCtx)
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed creating test API connection")
	}

	manager, err := certManager.Launch(ctx, tconn, browser)
	if err != nil {
		return errors.Wrap(err, "failed to launch certificate manager")
	}
	defer manager.Close(cleanupCtx)

	for _, cert := range certs {
		if err := manager.DeleteCert(cert.Name(), cert.Organization(), cert.CertType())(ctx); err != nil {
			return errors.Wrapf(err, "failed to delete certificate %q", cert.Name())
		}
	}
	return nil
}
