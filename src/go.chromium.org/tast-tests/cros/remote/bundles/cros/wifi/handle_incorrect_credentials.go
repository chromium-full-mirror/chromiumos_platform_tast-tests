// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/tunneled1x"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
)

// handleCredentialTestParam is the parameter for the test.
type handleCredentialTestParam struct {
	// isLoggedIn indicates the login mode.
	isLoggedIn bool
	// isLacros records the browser type is Lacros or not.
	isLacros bool
	// securityConfig holds the configurations to configure the WiFi AP.
	securityConfig security.ConfigFactory
	// joinWifiOption defines the credential details of the WiFi in the "Join Wi-Fi" dialog.
	joinWifiOption wifiutil.Credential
	// expectFailureMessage defines the expected connection failure error message in the "Join Wi-Fi" dialog.
	expectFailureMessage string
	// verification specifies the verification actions of a test.
	verification func(context.Context, *grpc.ClientConn, string, string) error
}

const (
	handleCredentialNetworkIdentity          = "identity"
	handleCredentialNetworkPassword          = "password"
	handleCredentialNetworkIncorrectIdentity = "incorrect_identity"
	handleCredentialNetworkIncorrectPassword = "incorrect_password"

	errorMsgIdentityAndPasswordIncorrect = "Username/password incorrect or EAP-auth failed"
	errorMsgUserCertNonInstalled         = "Please install a user certificate."
	errorMsgAuthenticationRejected       = "Authentication certificate rejected locally"

	// The certificate will be deleted by a combination of UI actions, which could take a while.
	deleteCertTimeout = 30 * time.Second
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           HandleIncorrectCredentials,
		LacrosStatus:   testing.LacrosVariantExists,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify ChromeOS handles incorrect credentials gracefully and displays error messages correctly",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: append(
			wifiutil.JoinWifiServiceNames,
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.browser.LacrosService",
			"tast.cros.networkui.CertificateService",
			wifiutil.FaillogServiceName,
		),
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Params: []testing.Param{
			{
				Name: "oobe_peap",
				Val: &handleCredentialTestParam{
					securityConfig: tunneled1x.NewConfigFactory(
						certificate.TestCert1().CACred.Cert,
						certificate.TestCert1().ServerCred,
						certificate.TestCert1().CACred.Cert,
						handleCredentialNetworkIdentity,
						handleCredentialNetworkPassword,
					),
					joinWifiOption: &wifiutil.PEAP{
						CaCert:   wifiutil.CertificateComboBoxOptionDoNotCheck,
						Identity: handleCredentialNetworkIncorrectIdentity,
						Password: handleCredentialNetworkIncorrectPassword,
					},
					expectFailureMessage: errorMsgIdentityAndPasswordIncorrect,
					verification:         openJoinWiFiDialogAndVerifyErrorMessage,
				},
			}, {
				Name: "oobe_ttls",
				Val: &handleCredentialTestParam{
					securityConfig: tunneled1x.NewConfigFactory(
						certificate.TestCert1().CACred.Cert,
						certificate.TestCert1().ServerCred,
						certificate.TestCert1().CACred.Cert,
						handleCredentialNetworkIdentity,
						handleCredentialNetworkPassword,
						tunneled1x.OuterProtocol(tunneled1x.Layer1TypeTTLS),
					),
					joinWifiOption: &wifiutil.TTLS{
						CaCert:   wifiutil.CertificateComboBoxOptionDoNotCheck,
						Identity: handleCredentialNetworkIncorrectIdentity,
						Password: handleCredentialNetworkIncorrectPassword,
					},
					expectFailureMessage: errorMsgIdentityAndPasswordIncorrect,
					verification:         openJoinWiFiDialogAndVerifyErrorMessage,
				},
			}, {
				Name: "oobe_tls",
				Val: &handleCredentialTestParam{
					securityConfig: wpaeap.NewConfigFactory(
						certificate.TestCert1().CACred.Cert,
						certificate.TestCert1().ServerCred,
						wpaeap.ClientCACert(certificate.TestCert1().CACred.Cert),
						wpaeap.ClientCred(certificate.TestCert1().ClientCred),
					),
					joinWifiOption: &wifiutil.TLS{
						CaCert:     wifiutil.CertificateComboBoxOptionDefault,
						ClientCert: wifiutil.CertificateComboBoxOptionNoneInstalled, // The certificates couldn't be imported on OOBE.
						Identity:   handleCredentialNetworkIncorrectIdentity,
					},
					expectFailureMessage: errorMsgUserCertNonInstalled,
					// "Join WiFi Network" dialog cannot connect to EAP-TLS network if there is no
					// client certificate installed in the DUT. This should be forbidden by
					// "Join WiFi Network" dialog with "Please install a user certificate." message alerts.
					verification: verifyErrorMessageShown,
				},
			}, {
				Name: "logged_in_peap",
				Val: &handleCredentialTestParam{
					isLoggedIn: true,
					securityConfig: tunneled1x.NewConfigFactory(
						certificate.TestCert1().CACred.Cert,
						certificate.TestCert1().ServerCred,
						certificate.TestCert1().CACred.Cert,
						handleCredentialNetworkIdentity,
						handleCredentialNetworkPassword,
					),
					joinWifiOption: &wifiutil.PEAP{
						CaCert:   wifiutil.CertificateComboBoxOptionDoNotCheck,
						Identity: handleCredentialNetworkIncorrectIdentity,
						Password: handleCredentialNetworkIncorrectPassword,
					},
					expectFailureMessage: errorMsgIdentityAndPasswordIncorrect,
					verification:         openJoinWiFiDialogAndVerifyErrorMessage,
				},
			}, {
				Name: "logged_in_ttls",
				Val: &handleCredentialTestParam{
					isLoggedIn: true,
					securityConfig: tunneled1x.NewConfigFactory(
						certificate.TestCert1().CACred.Cert,
						certificate.TestCert1().ServerCred,
						certificate.TestCert1().CACred.Cert,
						handleCredentialNetworkIdentity,
						handleCredentialNetworkPassword,
						tunneled1x.OuterProtocol(tunneled1x.Layer1TypeTTLS),
					),
					joinWifiOption: &wifiutil.TTLS{
						CaCert:   wifiutil.CertificateComboBoxOptionDoNotCheck,
						Identity: handleCredentialNetworkIncorrectIdentity,
						Password: handleCredentialNetworkIncorrectPassword,
					},
					expectFailureMessage: errorMsgIdentityAndPasswordIncorrect,
					verification:         openJoinWiFiDialogAndVerifyErrorMessage,
				},
			}, {
				Name: "logged_in_tls",
				Val: &handleCredentialTestParam{
					isLoggedIn: true,
					securityConfig: wpaeap.NewConfigFactory(
						certificate.TestCert1().CACred.Cert,
						certificate.TestCert1().ServerCred,
						wpaeap.ClientCACert(certificate.TestCert1().CACred.Cert),
						wpaeap.ClientCred(certificate.TestCert1().ClientCred),
					),
					joinWifiOption: &wifiutil.TLS{
						CaCert: fmt.Sprintf("%s [%s]",
							certificate.TestCert2().CACred.Info.CommonName,
							certificate.TestCert2().CACred.Info.CommonName),
						ClientCert: fmt.Sprintf("%s [%s]",
							certificate.TestCert2().CACred.Info.CommonName,
							certificate.TestCert2().ClientCred.Info.CommonName),
						Identity: handleCredentialNetworkIncorrectIdentity,
					},
					expectFailureMessage: errorMsgAuthenticationRejected,
					// Using incorrect certificates to connect to EAP-TLS network should be
					// notified with a "Authentication certificate rejected locally" message
					// by "Join WiFi Network" dialog after reopen it.
					verification: openJoinWiFiDialogAndVerifyErrorMessage,
				},
			},
			// TODO(crbug/1366609): Enable lacros test once the bug is fixed.
		},
	})
}

// HandleIncorrectCredentials verifies ChromeOS handles incorrect credentials gracefully and displays error messages correctly.
func HandleIncorrectCredentials(ctx context.Context, s *testing.State) {
	var (
		tf    = s.FixtValue().(*wificell.TestFixture)
		param = s.Param().(*handleCredentialTestParam)
	)

	apOptions := append(wificell.DefaultOpenNetworkAPOptions(), hostapd.SSID(hostapd.RandomSSID("HandleIncorrectCredentials_")))
	ap, err := tf.ConfigureAP(ctx, apOptions, param.securityConfig)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer tf.DeconfigAP(ctx, ap)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	cleanupCtx := ctx
	// There are two certificates to be deleted.
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second+2*deleteCertTimeout)
	defer cancel()

	startChromeReq := &ui.NewRequest{}
	isLoggedIn := param.isLoggedIn
	if !isLoggedIn {
		startChromeReq.LoginMode = ui.LoginMode_LOGIN_MODE_NO_LOGIN
		startChromeReq.SigninProfileTestExtensionId = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	}

	isLacros := param.isLacros
	if isLacros {
		startChromeReq.Lacros = &ui.Lacros{}
	}

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, startChromeReq); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	if isLacros {
		lacros := ui.NewLacrosServiceClient(rpcClient.Conn)
		if _, err := lacros.Launch(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to launch Lacros: ", err)
		}
		defer lacros.Close(cleanupCtx, &emptypb.Empty{})
	}

	// The certificates couldn't be imported on OOBE, and the Certificates Manager is only available when DUT is logged in.
	if isLoggedIn {
		certSvc := networkui.NewCertificateServiceClient(rpcClient.Conn)
		if _, err := certSvc.Init(ctx, &networkui.InitRequest{
			IsLacros: isLacros,
			InitType: networkui.InitRequest_LAUNCH,
		}); err != nil {
			s.Fatal("Failed to set up resource and launch the certificates manager: ", err)
		}
		defer certSvc.Close(cleanupCtx, &emptypb.Empty{})

		cleanupCerts, err := importCerts(ctx, certSvc, tf.DUTConn(wificell.DefaultDUT))
		if err != nil {
			s.Fatal("Failed to import certificates: ", err)
		}
		defer cleanupCerts(cleanupCtx)
	}

	// Expecting the join WiFi attempt complete successfully,
	// the connection failure should be prompted later, in a different UI.
	cleanup, err := wifiutil.JoinWifiFromQuickSettings(ctx, rpcClient.Conn, ap.Config().SSID, param.joinWifiOption)
	defer cleanup(cleanupCtx)
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, s.TestName())
	if err != nil {
		s.Fatal("Failed to attempt to join WiFi: ", err)
	}

	wifiSvc := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiSvc.WaitForConnected(ctx, ap.Config().SSID, false /* expectedValue */); err != nil {
		s.Fatal("Failed to wait for the network connection failed: ", err)
	}

	if err := param.verification(ctx, rpcClient.Conn, ap.Config().SSID, param.expectFailureMessage); err != nil {
		s.Fatal("Failed to verify UI prompt correctly: ", err)
	}
}

// importCerts imports the CA and client certificates of certificate.TestCert2() to the DUT,
// and returns a cleanup function to remove them.
func importCerts(ctx context.Context, certSvc networkui.CertificateServiceClient, dutConn *ssh.Conn) (cleanup func(context.Context) error, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second+deleteCertTimeout)
	defer cancel()

	incorrectTestCert := certificate.TestCert2()

	caCertDest := certificate.NewRemoteDestination(dutConn, "/tmp", "handle_incorrect_credential_root_cert.crt")
	cleanUpCaCert, err := certificate.WriteCACert(ctx, caCertDest, incorrectTestCert)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the CA certificate file")
	}
	defer cleanUpCaCert(cleanupCtx)

	clientCertDest := certificate.NewRemoteDestination(dutConn, "/tmp", "handle_incorrect_credential_client_cert.p12")
	cleanUpClientCert, err := certificate.WriteClientCertWithPassword(ctx, clientCertDest, incorrectTestCert, "" /* password */)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the client certificate file")
	}
	defer cleanUpClientCert(cleanupCtx)

	caImportReq := &networkui.ImportRequest{
		ImportDetail: &networkui.ImportRequest_Ca{
			Ca: &networkui.ImportRequest_CaImportDetail{},
		},
		Certificate: &networkui.Certificate{
			Type: networkui.Certificate_CA,
			Name: certificate.TestCert2().CACred.Info.CommonName,
			// Organization info isn't available in this certs, ChromeOS will use its common name instead.
			Organization: certificate.TestCert2().CACred.Info.CommonName,
		},
		FilePath: caCertDest.FullPath(),
	}
	if _, err := certSvc.ImportCert(ctx, caImportReq); err != nil {
		return nil, errors.Wrapf(err, "failed to import the certificate %+v", caImportReq.GetCertificate())
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			certSvc.DeleteCert(ctx, caImportReq.GetCertificate())
		}
	}(cleanupCtx)

	clientImportReq := &networkui.ImportRequest{
		ImportDetail: &networkui.ImportRequest_Client{
			Client: &networkui.ImportRequest_ClientImportDetail{
				ImportType: networkui.ImportRequest_ClientImportDetail_IMPORT_AND_BIND,
			},
		},
		Certificate: &networkui.Certificate{
			Type: networkui.Certificate_CLIENT,
			Name: certificate.TestCert2().ClientCred.Info.CommonName,
			// Organization info isn't available in this certs, ChromeOS will use its common name instead.
			Organization: certificate.TestCert2().ClientCred.Info.CommonName,
		},
		FilePath: clientCertDest.FullPath(),
	}
	if _, err := certSvc.ImportCert(ctx, clientImportReq); err != nil {
		return nil, errors.Wrapf(err, "failed to import the certificate %+v", clientImportReq.GetCertificate())
	}

	return func(ctx context.Context) error {
		if _, err := certSvc.DeleteCert(ctx, caImportReq.GetCertificate()); err != nil {
			return errors.Wrapf(err, "failed to delete the certificate %+v", caImportReq.GetCertificate())
		}
		if _, err := certSvc.DeleteCert(ctx, clientImportReq.GetCertificate()); err != nil {
			return errors.Wrapf(err, "failed to delete the certificate %+v", clientImportReq.GetCertificate())
		}
		return nil
	}, nil
}

// openJoinWiFiDialogAndVerifyErrorMessage opens specific SSID 'Join Wi-Fi Network' dialog and
// verifies the error message should show in the dialog.
func openJoinWiFiDialogAndVerifyErrorMessage(ctx context.Context, conn *grpc.ClientConn, ssid, expectedErrorMsg string) error {
	quickSettingsSvc := quicksettings.NewQuickSettingsServiceClient(conn)
	if _, err := quickSettingsSvc.NavigateToNetworkDetailedView(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to navigate to network detailed view within the Quick Settings")
	}

	uiSvc := ui.NewAutomationServiceClient(conn)
	networkFinder := ui.Node().Name(fmt.Sprintf("Connect to %s", ssid)).Role(ui.Role_ROLE_BUTTON).Finder()
	// DoDefault is used to avoid clicking on the notification since the Quick Settings
	// could be overlapped by notification, and this test can potentially generate one.
	if _, err := uiSvc.DoDefault(ctx, &ui.DoDefaultRequest{Finder: networkFinder}); err != nil {
		return errors.Wrap(err, "failed to click the network from detail list")
	}

	return verifyErrorMessageShown(ctx, conn, ssid, expectedErrorMsg)
}

// verifyErrorMessageShown verifies the error message should show in the "Join WiFi Network" dialog.
func verifyErrorMessageShown(ctx context.Context, conn *grpc.ClientConn, _, expectedErrorMsg string) error {
	uiSvc := ui.NewAutomationServiceClient(conn)
	errorMsgTextFinder := ui.Node().Name(expectedErrorMsg).Role(ui.Role_ROLE_STATIC_TEXT).Ancestor(wifiutil.JoinWiFiNetworkDialogFinder).Finder()
	if _, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: errorMsgTextFinder}); err != nil {
		return errors.Wrapf(err, "failed to find the expected error message %q", expectedErrorMsg)
	}
	return nil
}
