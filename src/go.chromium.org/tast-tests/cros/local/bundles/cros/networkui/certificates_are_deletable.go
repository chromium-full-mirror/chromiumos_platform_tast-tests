// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	certManager "go.chromium.org/tast-tests/cros/local/networkui/certificate"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CertificatesAreDeletable,
		// This test launches a web page so there should be a lacros variant.
		// Lacros test will be added once the issue(crbug/1366609) is fixed.
		LacrosStatus:   testing.LacrosVariantNeeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that ChromeOS User/CA certificates can be deleted",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		Attr:         []string{"group:network", "network_e2e"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Fixture: fixture.ChromeLoggedIn,
				Val:     browser.TypeAsh,
			},
			// TODO(crbug/1366609): Enable lacros test once the bug is fixed.
		},
	})
}

const certsDeletablePassword = "" // No password needed

// CertificatesAreDeletable verifies that ChromeOS User/CA certificates can be deleted.
func CertificatesAreDeletable(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	testCert := &testCert{
		cert: certificate.TestCert1(),
		client: &certInfo{
			fileName: "test_cert_client.p12",
			certName: certificate.TestCert1().ClientCred.Info.CommonName,
			// Organization info isn't available in this cert, ChromeOS will use its common name instead.
			org:      certManager.Organization{Name: certificate.TestCert1().ClientCred.Info.CommonName},
			certType: certManager.TypeClient,
		},
		ca: &certInfo{
			fileName: "test_cert_root.crt",
			certName: certificate.TestCert1().CACred.Info.CommonName,
			// Organization info isn't available in this cert, ChromeOS will use its common name instead.
			org:      certManager.Organization{Name: certificate.TestCert1().CACred.Info.CommonName},
			certType: certManager.TypeCA,
		},
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanup, err := generateCerts(ctx, cr, testCert)
	if err != nil {
		s.Fatal("Failed to prepare certificates: ", err)
	}
	defer cleanup(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	browser, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	manager, err := certManager.Launch(ctx, tconn, browser)
	if err != nil {
		s.Fatal("Failed to launch the certificates manager: ", err)
	}
	defer manager.Close(cleanupCtx)

	for _, importType := range []certManager.ImportType{
		certManager.TypeImportAndBind,
		certManager.TypeImport,
	} {
		s.Run(ctx, string(importType), func(ctx context.Context, s *testing.State) {
			// certSvc.DeleteCert is a combination of UI actions, which could take a while.
			const deleteCertTimeout = 35 * time.Second

			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, deleteCertTimeout)
			defer cancel()

			if err := uiauto.Combine("import certificates",
				testCert.importClientCert(manager, importType),
				testCert.importCaCert(manager),
			)(ctx); err != nil {
				s.Fatal("Failed to import certificates: ", err)
			}
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump_"+string(importType))
			// This test requires operation on deleting certificate, but it is also a cleanup.
			// Leveraging the `defer` pattern to ensure the certificates won't be deleted multiple times.
			defer func(ctx context.Context) {
				if err := uiauto.Combine("delete certificates",
					testCert.client.deleteCert(manager),
					testCert.ca.deleteCert(manager),
				)(ctx); err != nil {
					s.Fatal("Failed to delete certificates: ", err)
				}

				// Verifying that the certificates should be not imported after deleting them.
				if err := verifyCertsImportedState(ctx, manager, testCert, false /* expectImported */); err != nil {
					s.Fatal("Failed to verify certificates' imported state: ", err)
				}
			}(cleanupCtx)

			// Ensuring the certificates are imported to perform the upcoming tests.
			if err := verifyCertsImportedState(ctx, manager, testCert, true /* expectImported */); err != nil {
				s.Fatal("Failed to verify certificates' imported state: ", err)
			}
		})
	}
}

func generateCerts(ctx context.Context, cr *chrome.Chrome, testCert *testCert) (cleanup func(context.Context), retErr error) {
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve user's Downloads path")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	caCertDest := certificate.NewLocalDestination(downloadsPath, testCert.ca.fileName)
	cleanUpCaCert, err := certificate.WriteCACert(ctx, caCertDest, testCert.cert)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the CA certificate file")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			cleanUpCaCert(ctx)
		}
	}(cleanupCtx)

	clientCertDest := certificate.NewLocalDestination(downloadsPath, testCert.client.fileName)
	cleanUpClientCert, err := certificate.WriteClientCertWithPassword(ctx, clientCertDest, testCert.cert, certsDeletablePassword)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the client certificate file")
	}

	return func(ctx context.Context) {
		cleanUpCaCert(ctx)
		cleanUpClientCert(ctx)
	}, nil
}

func verifyCertsImportedState(ctx context.Context, manager *certManager.Manager, testCert *testCert, expectImported bool) error {
	msg := map[bool]string{
		true:  "imported",
		false: "not imported",
	}

	clientCertImported, err := testCert.client.isImported(ctx, manager)
	if err != nil {
		return errors.Wrap(err, "failed to verify if the client certificate is shown")
	} else if clientCertImported != expectImported {
		return errors.Errorf("unexpected client certificate imported state; got: %q, want: %q", msg[clientCertImported], msg[expectImported])
	}

	caCertImported, err := testCert.ca.isImported(ctx, manager)
	if err != nil {
		return errors.Wrap(err, "failed to verify if the CA certificate is shown")
	} else if caCertImported != expectImported {
		return errors.Errorf("unexpected client certificate imported state; got: %q, want: %q", msg[caCertImported], msg[expectImported])
	}

	return nil
}

type testCert struct {
	cert   certificate.CertStore
	client *certInfo
	ca     *certInfo
}

func (c *testCert) importClientCert(manager *certManager.Manager, importType certManager.ImportType) uiauto.Action {
	return manager.ImportClientCert(c.client.fileName, certsDeletablePassword, c.client.certName, c.client.org, importType)
}

func (c *testCert) importCaCert(manager *certManager.Manager) uiauto.Action {
	return manager.ImportCACert(c.ca.fileName, c.ca.org, 0 /* No trust settings needed */)
}

type certInfo struct {
	fileName string
	certName string
	org      certManager.Organization
	certType certManager.CertType
}

func (c *certInfo) deleteCert(manager *certManager.Manager) uiauto.Action {
	return manager.DeleteCert(c.certName, c.org, c.certType)
}

func (c *certInfo) isImported(ctx context.Context, manager *certManager.Manager) (bool, error) {
	return manager.IsCertImported(ctx, c.certName, c.org, c.certType)
}
