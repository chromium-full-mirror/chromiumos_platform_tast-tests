// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	cert "go.chromium.org/tast-tests/cros/remote/network"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           CertsUsableAndPersist,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that installed certificates are usable and persist after different test scenarios",
		Contacts: []string{
			"cros-networking@google.com",
			"miersh@google.com",
		},
		BugComponent: "b:156085", // ChromeOS > Platform > baseOS > Networking
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:         []string{"group:wificell_network_cellular"}, // Add "group:release-health", "release-health_wifi" after stabilized.
		TestBedDeps: []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.networkui.CertificateService",
			"tast.cros.wifi.WifiService",
			wificell.ShillServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      certsUsableAndPersistVars(),
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Params: []testing.Param{
			{
				Name: "suspend",
				Val: &certsUsableTestParams{
					testScenario: suspendAndRestore,
					numTrials:    1,
					// WiFi should be auto-reconnected on resume, wait for it to reconnect.
					shouldAutoConnect: true,
				},
			}, {
				Name: "relogin",
				Val: &certsUsableTestParams{
					testScenario: reLoginAndRestore,
					numTrials:    1,
				},
			}, {
				Name: "relogin_stress",
				Val: &certsUsableTestParams{
					testScenario: reLoginAndRestore,
					numTrials:    3,
				},
			},
		},
		Timeout: 15 * time.Minute,
	})
}

type certsUsableTestParams struct {
	// testScenario is the action to be tested, certificate should be usable and persist after this action.
	testScenario func(ctx context.Context, tf *wificell.TestFixture) error

	// numTrials is a number of the trials for the test scenario.
	numTrials int

	shouldAutoConnect bool
}

type certificateDetail struct {
	*networkui.Certificate
	certificate.CertStore
}

type certificates map[networkui.Certificate_Type]*certificateDetail

// CertsUsableAndPersist verifies that installed certificates are usable and persist after different test scenarios.
func CertsUsableAndPersist(ctx context.Context, s *testing.State) {
	testCerts := cert.LoadCertsFromVars()

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

	params := s.Param().(*certsUsableTestParams)

	startChromeReq := &ui.NewRequest{}

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, startChromeReq); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	certSvc := networkui.NewCertificateServiceClient(rpcClient.Conn)
	if _, err := certSvc.Init(ctx, &networkui.InitRequest{
		InitType: networkui.InitRequest_LAUNCH,
	}); err != nil {
		s.Fatal("Failed to initialize the certificate service: ", err)
	}
	defer certSvc.Close(cleanupCtx, &emptypb.Empty{})

	for _, test := range []struct {
		name        string
		certDetails map[networkui.Certificate_Type]*certificateDetail
	}{
		{
			name: "password-protected certificate",
			certDetails: map[networkui.Certificate_Type]*certificateDetail{
				networkui.Certificate_CLIENT: {
					Certificate: &networkui.Certificate{
						Type:         networkui.Certificate_CLIENT,
						Name:         testCerts.ClientCred.Info.CommonName,
						Organization: testCerts.ClientCred.Info.Organization,
						Password:     "12345",
					},
					CertStore: testCerts,
				},
				networkui.Certificate_CA: {
					Certificate: &networkui.Certificate{
						Type:         networkui.Certificate_CA,
						Name:         testCerts.CACred.Info.CommonName,
						Organization: testCerts.CACred.Info.Organization,
					},
					CertStore: testCerts,
				},
			},
		}, {
			name: "non-password-protected certificate",
			certDetails: map[networkui.Certificate_Type]*certificateDetail{
				networkui.Certificate_CLIENT: {
					Certificate: &networkui.Certificate{
						Type:         networkui.Certificate_CLIENT,
						Name:         testCerts.ClientCred.Info.CommonName,
						Organization: testCerts.ClientCred.Info.Organization,
					},
					CertStore: testCerts,
				},
				networkui.Certificate_CA: {
					Certificate: &networkui.Certificate{
						Type:         networkui.Certificate_CA,
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

			if err := certsImported(ctx, certSvc, test.certDetails); err != nil {
				s.Fatal("Failed to verify certificates are all imported: ", err)
			}

			joinEAPNetworkReq := &wifi.JoinWifiRequest{
				Ssid: ap.Config().SSID,
				Security: &wifi.JoinWifiRequest_EapTls{
					EapTls: &wifi.JoinWifiRequest_SecurityEapTls{
						ClientCert: fmt.Sprintf("%s [%s]",
							test.certDetails[networkui.Certificate_CA].Name,
							test.certDetails[networkui.Certificate_CLIENT].Name),
						CaCert: fmt.Sprintf("%s [%s]",
							test.certDetails[networkui.Certificate_CA].Name,
							test.certDetails[networkui.Certificate_CA].Name),
					},
				},
			}
			defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

			if err := verifyCertUsable(ctx, tf, joinEAPNetworkReq, ap.Config().SSID); err != nil {
				s.Fatal("Failed to verify that the certificates are usable: ", err)
			}

			for i := 0; i < params.numTrials; i++ {
				if err := params.testScenario(ctx, tf); err != nil {
					s.Fatal("Failed to complete the test scenario: ", err)
				}
			}

			if params.shouldAutoConnect {
				wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
				if err := wifiClient.WaitForConnected(ctx, ap.Config().SSID, true); err != nil {
					s.Fatalf("Failed to wait for WiFi %q to connect: %v", ap.Config().SSID, err)
				}

				// Disconnect from WiFi and cleanup configurations to further verify that certificates are usable and persist.
				if err := tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT); err != nil {
					s.Fatal("Failed to disconnect from WiFi and cleanup WiFi configurations: ", err)
				}
			}

			// Verify the certificates are still persist.
			if err := certsImported(ctx, certSvc, test.certDetails); err != nil {
				s.Fatal("Failed to verify certificates are all imported: ", err)
			}

			// Verify the certificates are still usable.
			if err := verifyCertUsable(ctx, tf, joinEAPNetworkReq, ap.Config().SSID); err != nil {
				s.Fatal("Failed to verify that the certificates are usable and persist: ", err)
			}
		})
	}
}

// suspendAndRestore suspends the DUT for a while and then
// restores resources associate with chrome.Chrome instance,
// including chrome.Chrome instance and certificate manager.
func suspendAndRestore(ctx context.Context, tf *wificell.TestFixture) error {
	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiClient.Suspend(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.Reconnect(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to reconnect to the Chrome session")
	}

	certSvc := networkui.NewCertificateServiceClient(rpcClient.Conn)
	if _, err := certSvc.Init(ctx, &networkui.InitRequest{
		InitType: networkui.InitRequest_CONNECT,
	}); err != nil {
		return errors.Wrap(err, "failed to reconnect to the certificate service")
	}

	return nil
}

// reLoginAndRestore re-login the chrome.Chrome session by starting a new one.
// This method also ensure resources are all well-managed by closing them before re-login.
func reLoginAndRestore(ctx context.Context, tf *wificell.TestFixture) error {
	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	certSvc := networkui.NewCertificateServiceClient(rpcClient.Conn)
	if _, err := certSvc.Close(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to close the certificate manager")
	}

	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.Close(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to close the chrome session")
	}

	startChromeReq := &ui.NewRequest{KeepState: true}

	if _, err := crSvc.New(ctx, startChromeReq); err != nil {
		return errors.Wrap(err, "failed to re-login to the chrome")
	}

	if _, err := certSvc.Init(ctx, &networkui.InitRequest{
		InitType: networkui.InitRequest_LAUNCH,
	}); err != nil {
		return errors.Wrap(err, "failed to initialize the certificate service")
	}

	return nil
}

// verifyCertUsable verifies that the certificates are available and usable by trying to join to the network.
// Joining a WEP network requires a certificate, so the join attempt ensures that certificates are available.
// This method also expects the network to be connected, which means that the certificates are usable.
func verifyCertUsable(ctx context.Context, tf *wificell.TestFixture, joinEAPNetworkReq *wifi.JoinWifiRequest, ssid string) error {
	wifiUIClient := wifi.NewWifiServiceClient(tf.DUTRPC(wificell.DefaultDUT).Conn)
	if _, err := wifiUIClient.JoinWifiFromQuickSettings(ctx, joinEAPNetworkReq); err != nil {
		return errors.Wrapf(err, "failed to connect to WiFi %q", ssid)
	}

	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiClient.WaitForConnected(ctx, ssid, true); err != nil {
		return errors.Wrapf(err, "failed to wait for WiFi %q to connect", ssid)
	}
	return nil
}

// certsImported verifies that |certificates| are all imported.
func certsImported(ctx context.Context, certSvc networkui.CertificateServiceClient, certificates certificates) error {
	for _, cert := range certificates {
		if response, err := certSvc.IsCertImported(ctx, cert.Certificate); err != nil {
			return errors.Wrapf(err, "failed to verify certificate %+v is imported", cert.Certificate)
		} else if !response.IsImported {
			return errors.Errorf("certificate %+v is not imported", cert.Certificate)
		}
	}
	return nil
}

func importCert(ctx context.Context, dutConn *ssh.Conn, certSvc networkui.CertificateServiceClient, certDetail *certificateDetail) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	req := &networkui.ImportRequest{Certificate: certDetail.Certificate}

	switch certDetail.Type {
	case networkui.Certificate_CLIENT:
		dest := certificate.NewRemoteDestination(dutConn, "/tmp", "test_cert_client.p12")
		cleanUp, err := certificate.WriteClientCertWithPassword(ctx, dest, certDetail.CertStore, certDetail.Password)
		if err != nil {
			return errors.Wrap(err, "failed to create the client certificate file")
		}
		defer cleanUp(cleanupCtx)
		req.FilePath = dest.FullPath()

		req.ImportDetail = &networkui.ImportRequest_Client{
			Client: &networkui.ImportRequest_ClientImportDetail{
				ImportType: networkui.ImportRequest_ClientImportDetail_IMPORT_AND_BIND,
			},
		}
	case networkui.Certificate_CA:
		dest := certificate.NewRemoteDestination(dutConn, "/tmp", "test_cert_root.crt")
		cleanUp, err := certificate.WriteCACert(ctx, dest, certDetail.CertStore)
		if err != nil {
			return errors.Wrap(err, "failed to create the CA certificate file")
		}
		defer cleanUp(cleanupCtx)
		req.FilePath = dest.FullPath()

		req.ImportDetail = &networkui.ImportRequest_Ca{
			Ca: &networkui.ImportRequest_CaImportDetail{},
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
