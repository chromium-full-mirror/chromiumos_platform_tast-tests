// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	cert "go.chromium.org/tast-tests/cros/remote/network"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CertsUsableAndPersist,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verify that installed certificates are usable after suspend and resume",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
			"edgar.chang@cienet.com",
			"cienet-development@googlegroups.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.browser.LacrosService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.network.CertificateService",
			"tast.cros.wifi.WifiService",
			wificell.ShillServiceName,
		},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("bruce", "sona", "syndra")),
		SoftwareDeps: []string{"chrome"},
		VarDeps:      certsUsableAndPersistVars(),
		Fixture:      "wificellFixt",
		Params: []testing.Param{
			{
				Val: false, /* isLacros */
			},
			// TODO(crbug/1366609): Enable lacros test once the bug is fixed.
		},
		Timeout: 7 * time.Minute,
	})
}

// CertsUsableAndPersist verifies that installed certificates are usable after suspend and resume.
func CertsUsableAndPersist(ctx context.Context, s *testing.State) {
	testCerts := cert.LoadCertsFromVars()
	const clientCertPassword = "12345"

	tf := s.FixtValue().(*wificell.TestFixture)

	opt := []hostapd.Option{hostapd.Mode(hostapd.Mode80211g), hostapd.Channel(1)}
	cfg := wpaeap.NewConfigFactory(
		testCerts.CACred.Cert,
		testCerts.ServerCred,
		wpaeap.ClientCACert(testCerts.CACred.Cert),
		wpaeap.ClientCred(testCerts.ClientCred),
	)

	ap, err := tf.ConfigureAP(ctx, opt, cfg)
	if err != nil {
		s.Fatal("Failed to configure ap: ", err)
	}
	defer tf.DeconfigAP(ctx, ap)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	isLacros := s.Param().(bool)
	startChromeReq := &ui.NewRequest{}
	if isLacros {
		startChromeReq.Lacros = &ui.Lacros{}
	}

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, startChromeReq); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	var lacrosSvc ui.LacrosServiceClient
	if isLacros {
		lacrosSvc = ui.NewLacrosServiceClient(rpcClient.Conn)
		if _, err := lacrosSvc.Launch(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to launch lacros: ", err)
		}
		defer lacrosSvc.Close(cleanupCtx, &emptypb.Empty{})
	}

	certSvc := network.NewCertificateServiceClient(rpcClient.Conn)
	if _, err := certSvc.Init(ctx, &network.InitRequest{
		IsLacros: isLacros,
		InitType: network.InitRequest_LAUNCH,
	}); err != nil {
		s.Fatal("Failed to initialize the certificate service: ", err)
	}
	defer certSvc.Close(cleanupCtx, &emptypb.Empty{})

	for _, test := range []struct {
		name        string
		certDetails map[network.Certificate_Type]*certificateDetail
	}{
		{
			name: "password-protected certificate",
			certDetails: map[network.Certificate_Type]*certificateDetail{
				network.Certificate_CLIENT: {
					Certificate: &network.Certificate{
						Type:         network.Certificate_CLIENT,
						Name:         testCerts.ClientCred.Info.CommonName,
						Organization: testCerts.ClientCred.Info.Organization,
						Password:     clientCertPassword,
					},
					CertStore: testCerts,
				},
				network.Certificate_CA: {
					Certificate: &network.Certificate{
						Type:         network.Certificate_CA,
						Name:         testCerts.CACred.Info.CommonName,
						Organization: testCerts.CACred.Info.Organization,
					},
					CertStore: testCerts,
				},
			},
		}, {
			name: "non-password-protected certificate",
			certDetails: map[network.Certificate_Type]*certificateDetail{
				network.Certificate_CLIENT: {
					Certificate: &network.Certificate{
						Type:         network.Certificate_CLIENT,
						Name:         testCerts.ClientCred.Info.CommonName,
						Organization: testCerts.ClientCred.Info.Organization,
					},
					CertStore: testCerts,
				},
				network.Certificate_CA: {
					Certificate: &network.Certificate{
						Type:         network.Certificate_CA,
						Name:         testCerts.CACred.Info.CommonName,
						Organization: testCerts.CACred.Info.Organization,
					},
					CertStore: testCerts,
				},
			},
		},
	} {
		s.Run(ctx, test.name, func(ctx context.Context, s *testing.State) {
			// certSvc.DeleteCert is a combination of UI actions, which could take a while.
			const deleteCertTimeout = 35 * time.Second

			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, deleteCertTimeout)
			defer cancel()

			for _, certDetail := range test.certDetails {
				if err := importCert(ctx, tf.DUTConn(wificell.DefaultDUT), certSvc, certDetail); err != nil {
					s.Fatal("Failed to import the certificate: ", err)
				}
				defer certSvc.DeleteCert(cleanupCtx, certDetail.Certificate)
			}
			defer dumpUITreeWithScreenshotOnError(cleanupCtx, rpcClient, s.HasError, "before_delete_certs")

			wifiUIClient := wifi.NewWifiServiceClient(rpcClient.Conn)
			if _, err := wifiUIClient.JoinWifiFromQuickSettings(ctx, &wifi.JoinWifiRequest{
				Ssid: ap.Config().SSID,
				Security: &wifi.JoinWifiRequest_EapTls{
					EapTls: &wifi.JoinWifiRequest_SecurityEapTls{
						ClientCert: fmt.Sprintf("%s [%s]",
							test.certDetails[network.Certificate_CA].Name,
							test.certDetails[network.Certificate_CLIENT].Name),
						CaCert: fmt.Sprintf("%s [%s]",
							test.certDetails[network.Certificate_CA].Name,
							test.certDetails[network.Certificate_CA].Name),
					},
				},
			}); err != nil {
				s.Fatalf("Failed to connect to wifi %q: %v", ap.Config().SSID, err)
			}
			defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

			wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
			if err := wifiClient.Suspend(ctx, 5*time.Second); err != nil {
				s.Fatal("Failed to suspend DUT: ", err)
			}

			if err := restoreAfterResume(ctx, crSvc, lacrosSvc, certSvc); err != nil {
				s.Fatal("Failed to reconnect to resources after resume: ", err)
			}

			// Verify the certificates are still imported.
			for _, certDetail := range test.certDetails {
				if response, err := certSvc.IsCertImported(ctx, certDetail.Certificate); err != nil {
					s.Fatalf("Failed to verify the certificate %+v has imported: %v", certDetail.Certificate, err)
				} else if !response.IsImported {
					s.Fatalf("The certificate %+v was not imported", certDetail.Certificate)
				}
			}

			// Verify the certificates are still valid.
			if err := wifiClient.WaitForConnected(ctx, ap.Config().SSID, true); err != nil {
				s.Fatalf("Failed to wait for wifi %q to connect: %v", ap.Config().SSID, err)
			}
		})
	}
}

// restoreAfterResume restores the services after DUT resumed from suspend.
// After suspend and resume, the connections built in the test would be invalid,
// reconnect to the services to restore control to the Certificates Manager.
func restoreAfterResume(ctx context.Context, crSvc ui.ChromeServiceClient, lacrosSvc ui.LacrosServiceClient, certSvc network.CertificateServiceClient) error {
	if _, err := crSvc.Reconnect(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to reconnect to the Chrome session")
	}

	isLacros := lacrosSvc != nil
	if isLacros {
		if _, err := lacrosSvc.Connect(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrap(err, "failed to reconnect to lacros")
		}
	}

	if _, err := certSvc.Init(ctx, &network.InitRequest{
		IsLacros: isLacros,
		InitType: network.InitRequest_CONNECT,
	}); err != nil {
		return errors.Wrap(err, "failed to reconnect to the certificate service")
	}

	return nil
}

type certificateDetail struct {
	*network.Certificate
	certificate.CertStore
}

func importCert(ctx context.Context, dutConn *ssh.Conn, certSvc network.CertificateServiceClient, certDetail *certificateDetail) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	req := &network.ImportRequest{Certificate: certDetail.Certificate}
	switch certDetail.Type {
	case network.Certificate_CLIENT:
		dest, cleanUp, err := certificate.WriteClientCertToFile(ctx, certDetail.CertStore, certDetail.Password, "test_cert_client", dutConn)
		if err != nil {
			return errors.Wrap(err, "failed to create the client certificate file")
		}
		defer cleanUp(cleanupCtx)
		req.FilePath = dest

		req.ImportDetail = &network.ImportRequest_Client{
			Client: &network.ImportRequest_ClientImportDetail{
				ImportType: network.ImportRequest_ClientImportDetail_IMPORT_AND_BIND,
			},
		}
	case network.Certificate_CA:
		dest, cleanUp, err := certificate.WriteCACertToFile(ctx, certDetail.CertStore, "test_cert_root", dutConn)
		if err != nil {
			return errors.Wrap(err, "failed to create the CA certificate file")
		}
		defer cleanUp(cleanupCtx)
		req.FilePath = dest

		req.ImportDetail = &network.ImportRequest_Ca{
			Ca: &network.ImportRequest_CaImportDetail{},
		}
	}

	if _, err := certSvc.ImportCert(ctx, req); err != nil {
		return errors.Wrapf(err, "failed to import the certificate %+v", certDetail.Certificate)
	}

	return nil
}

func dumpUITreeWithScreenshotOnError(ctx context.Context, rpcClient *rpc.Client, hasError func() bool, filePrefix string) {
	if hasError() {
		ui.NewChromeUIServiceClient(rpcClient.Conn).DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{FilePrefix: filePrefix})
	}
}

func certsUsableAndPersistVars() []string {
	variables := make([]string, 0, 12)

	variables = append(variables, cert.CaCertificateVariables...)
	variables = append(variables, cert.ServerCertificateVariables...)
	variables = append(variables, cert.ClientCertificateVariables...)

	return variables
}
