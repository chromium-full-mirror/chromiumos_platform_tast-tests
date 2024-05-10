// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/pkcs11/netcertstore"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           VPNAreUserSpecified,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verify VPN networks added are user specific",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		Attr:         []string{"group:network", "network_e2e_unstable"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "vpnEnv",
		// This test performs login 2 times.
		Timeout: 2*chrome.LoginTimeout + chrome.ResetTimeout + 2*time.Minute,
	})
}

// VPNAreUserSpecified verifies VPN networks added are user specific.
func VPNAreUserSpecified(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	runner := hwsec.NewCmdRunner()
	certStore, err := netcertstore.CreateStore(ctx, runner)
	if err != nil {
		s.Fatal("Failed to create cert store: ", err)
	}
	defer certStore.Cleanup(cleanupCtx)

	testCert := certificate.TestCert1()
	// Prepare the certificates for the test.
	if _, err := certStore.InstallCertKeyPair(ctx, testCert.ClientCred.PrivateKey, testCert.ClientCred.Cert); err != nil {
		s.Fatal("Failed to install client cert: ", err)
	}
	if _, err := certStore.InstallCertKeyPair(ctx, "", testCert.CACred.Cert); err != nil {
		s.Fatal("Failed to install CA cert: ", err)
	}

	// Prepares virtualnet environment for the VPN server.
	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology: ", err)
	}
	defer func(ctx context.Context) {
		if err := networkEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
	}(cleanupCtx)

	// Prepares VPN server.
	config := vpn.NewConfig(
		vpn.TypeOpenVPN,
		vpn.WithOpenVPNUseUserPassword(),
	)

	vpnServer, err := vpn.StartServerWithConfig(ctx, networkEnv.Server1, config)
	if err != nil {
		s.Fatal("Failed to start VPN server: ", err)
	}
	defer vpnServer.Exit(cleanupCtx)

	res := &vpnAreUserSpecifiedResource{
		outDir:              s.OutDir(),
		vpnName:             "testVPN",
		vpnClientCertUIName: fmt.Sprintf("%s [%s]", testCert.CACred.Info.CommonName, testCert.ClientCred.Info.CommonName),
		vpnServer:           vpnServer,
		vpnConfig:           config,
	}

	// Use the user who has installed certificates through netcertstore package as
	// primary user responsible for configuring the VPN service.
	primaryUser := chrome.Creds{User: netcertstore.TestUsername, Pass: netcertstore.TestPassword}
	if err := loginAndDoAction(ctx, res, []chrome.Option{
		chrome.KeepState(), // Avoid resetting TPM.
		chrome.FakeLogin(primaryUser),
		chrome.DisableFeatures("LocalPasswordForConsumers"), // b/328576285
	}, joinVPN(res)); err != nil {
		s.Fatal("Failed to login primary user to prepare: ", err)
	}

	// Login as a different user and verify that another user does not have the VPN network configured.
	if err := loginAndDoAction(ctx, res, []chrome.Option{
		chrome.KeepState(),
		chrome.GuestLogin(),
		chrome.DisableFeatures("LocalPasswordForConsumers"), // b/328576285
	}, verifyVPNNotExist(res)); err != nil {
		s.Fatal("Failed to verify VPN networks are user specified: ", err)
	}
}

type vpnAreUserSpecifiedResource struct {
	outDir, vpnName, vpnClientCertUIName string

	cr    *chrome.Chrome
	tconn *chrome.TestConn

	vpnServer *vpn.Server
	vpnConfig *vpn.Config
}

func loginAndDoAction(ctx context.Context, res *vpnAreUserSpecifiedResource, opts []chrome.Option, action uiauto.Action) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	res.cr = cr
	defer func(ctx context.Context) {
		cr.Close(ctx)
		res.cr = nil
	}(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}
	res.tconn = tconn
	defer func() { res.tconn = nil }()

	return action(ctx)
}

func joinVPN(res *vpnAreUserSpecifiedResource) uiauto.Action {
	return func(ctx context.Context) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
		defer cancel()

		// Retrieve the VPN server properties for configuration via UI.
		vpnProps, err := vpn.CreateProperties(res.vpnServer, nil /* secondServer */)
		if err != nil {
			return errors.Wrap(err, "failed to generate D-Bus properties")
		}

		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get keyboard")
		}
		defer kb.Close(cleanupCtx)

		settings, err := ossettings.OpenJoinVPNDialog(ctx, res.tconn, res.cr)
		if err != nil {
			return errors.Wrap(err, "failed to open VPN dialog")
		}
		defer settings.Close(cleanupCtx)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, res.cr, "vpn_settings_ui_dump")

		vpnHelper, err := ossettings.NewVPNDialogHelper(res.vpnConfig.Type, vpnProps, res.vpnName, &res.vpnClientCertUIName)
		if err != nil {
			return errors.Wrap(err, "failed to create a UI helper")
		}

		if err := vpnHelper.FillInVPNConfigurations(ctx, res.cr, res.tconn, kb); err != nil {
			return errors.Wrap(err, "failed to configure VPN service")
		}

		if err := vpnHelper.ConnectAndWait(ctx, res.tconn); err != nil {
			return errors.Wrap(err, "failed to connect to VPN service")
		}

		manager, err := shill.NewManager(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create a manager object")
		}

		// Verify the profile of the VPN service is saved.
		if _, err := manager.WaitForServiceProperties(ctx, map[string]interface{}{
			shillconst.ServicePropertyType:        shillconst.TypeVPN,
			shillconst.ServicePropertyName:        res.vpnName,
			shillconst.ServicePropertyIsConnected: true,
		}, shillconst.DefaultTimeout); err != nil {
			return errors.Wrap(err, "failed to find the VPN service, VPN service is not saved")
		}

		if err := ossettings.DisconnectVPN(ctx, res.tconn); err != nil {
			return errors.Wrap(err, "failed to disconnect from VPN service")
		}

		return nil
	}
}

func verifyVPNNotExist(res *vpnAreUserSpecifiedResource) uiauto.Action {
	return func(ctx context.Context) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
		defer cancel()

		settings, err := ossettings.Launch(ctx, res.tconn)
		if err != nil {
			return errors.Wrap(err, "failed to launch the OS settings")
		}
		defer settings.Close(cleanupCtx)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, res.cr, "ossettings_ui_dump")

		if err := settings.NavigateToPageURL(ctx, res.cr, "internet", settings.Exists(ossettings.Internet)); err != nil {
			return errors.Wrap(err, "failed to navigate to network page")
		}

		ui := uiauto.New(res.tconn)
		arrowFinder := nodewith.Role(role.Button).HasClass("subpage-arrow")

		vpnNetworksPageArrow := arrowFinder.NameContaining("VPN")
		if err = ui.WaitUntilExists(vpnNetworksPageArrow)(ctx); err != nil {
			if nodewith.IsNodeNotFoundErr(err) {
				// VPN networks page is available only if a VPN is configured, also,
				//	1. VPN services configured by other users are expected to be
				//	invisible.
				//	2. Second user has not configured any other VPN service.
				// Therefore, the absence of the subpage-arrow of the VPN networks page
				// is expected, no further actions are required.
				return nil
			}
			return errors.Wrap(err, "failed to determine whether the VPN networks page is enabled")
		}

		// If the user has a VPN configured, enter the VPN networks page to verify if VPN
		// configured by other users exists.
		if err := ui.LeftClick(vpnNetworksPageArrow)(ctx); err != nil {
			return errors.Wrap(err, "failed to enter VPN networks page")
		}

		vpnServicePageArrow := arrowFinder.NameContaining(res.vpnName)
		if err := ui.WaitUntilExists(vpnServicePageArrow)(ctx); err != nil {
			if nodewith.IsNodeNotFoundErr(err) {
				// No further actions are required, as VPN services configured by other
				// users are expected to be invisible.
				return nil
			}
			return errors.Wrap(err, "failed to determine whether the VPN service page is found")
		}
		return errors.Wrap(err, "the VPN service is not user specific")
	}
}
