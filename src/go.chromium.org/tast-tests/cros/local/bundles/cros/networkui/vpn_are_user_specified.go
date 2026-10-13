// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	vpnManager "go.chromium.org/tast-tests/cros/local/networkui/vpn"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: VPNAreUserSpecified,
		Desc: "Verify VPN networks added are user specific",
		Contacts: []string{
			"cros-networking@google.com",
			"jiejiang@google.com",
		},
		BugComponent:   "b:156085", // ChromeOS > Platform > baseOS > Networking
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:network", "network_e2e"},
		SoftwareDeps:   []string{"chrome"},
		Fixture:        "vpnEnv",
		// This test performs login 2 times.
		Timeout: 2*chrome.LoginTimeout + chrome.ResetTimeout + 2*time.Minute,
	})
}

// VPNAreUserSpecified verifies VPN networks added are user specific.
func VPNAreUserSpecified(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	res := newVPNAreUserSpecifiedTestResource(s.OutDir())
	defer res.cleanup(cleanupCtx)

	if err := loginAndDoAction(ctx, nil, joinVPN(res)); err != nil {
		s.Fatal("Failed to login primary user to prepare: ", err)
	}

	// Login as a different user and verify that another user does not have the VPN
	// network configured.
	if err := loginAndDoAction(ctx, []chrome.Option{
		chrome.KeepState(),
		chrome.GuestLogin(),
	}, verifyVPNNotExist(res)); err != nil {
		s.Fatal("Failed to verify VPN networks are user specified: ", err)
	}
}

type vpnAreUserSpecifiedTestResource struct {
	outDir string

	cleanupVPNEnv uiauto.Action
}

func newVPNAreUserSpecifiedTestResource(outDir string) *vpnAreUserSpecifiedTestResource {
	return &vpnAreUserSpecifiedTestResource{
		outDir: outDir,
	}
}

func (res *vpnAreUserSpecifiedTestResource) cleanup(ctx context.Context) {
	if res.cleanupVPNEnv != nil {
		res.cleanupVPNEnv(ctx)
		res.cleanupVPNEnv = nil
	}
}

type vpnAreUserSpecifiedTestAction func(context.Context, *chrome.Chrome, *chrome.TestConn) error

func loginAndDoAction(ctx context.Context, opts []chrome.Option, action vpnAreUserSpecifiedTestAction) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	return action(ctx, cr, tconn)
}

const vpnNetworkName = "VPNAreUserSpecified_testVPN"

func joinVPN(res *vpnAreUserSpecifiedTestResource) vpnAreUserSpecifiedTestAction {
	return func(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
		defer cancel()

		cfg := vpn.NewConfig(
			// Using L2TP/IPsec PSK to perform the test (this is only a random choice).
			// TODO(b/414664834): Restore certificate-based VPN coverage once
			// netcertstore.InstallCertKeyPair (p11_replay --import --type=cert) sets
			// NSS CKT_NSS_TRUSTED_DELEGATOR trust attributes required by Chrome's
			// NetworkCertLoader for Server CA certificates.
			vpn.TypeL2TPIPsec,
			vpn.WithIPsecAuthType(vpn.AuthTypePSK),
		)

		_, vpnHelper, cleanup, err := vpnManager.NewDialogHelperWithVPNServer(ctx, cfg, vpnNetworkName)
		if err != nil {
			return errors.Wrap(err, "failed to prepare vpn env for testing")
		}
		defer func(ctx context.Context) {
			if retErr != nil {
				cleanup(ctx)
			}
		}(cleanupCtx)

		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get keyboard")
		}
		defer kb.Close(cleanupCtx)

		settings, err := ossettings.OpenJoinVPNDialog(ctx, tconn, cr)
		if err != nil {
			return errors.Wrap(err, "failed to open VPN dialog")
		}
		defer settings.Close(cleanupCtx)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, cr, "vpn_settings_ui_dump")

		if err := vpnHelper.FillInConfigurations(ctx, cr, tconn, kb); err != nil {
			return errors.Wrap(err, "failed to configure VPN service")
		}

		if err := vpnHelper.ConnectAndWait(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to connect to VPN service")
		}

		manager, err := shill.NewManager(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create a manager object")
		}

		// Verify the profile of the VPN service is saved.
		if _, err := manager.WaitForServiceProperties(ctx, map[string]interface{}{
			shillconst.ServicePropertyType:        shillconst.TypeVPN,
			shillconst.ServicePropertyName:        vpnNetworkName,
			shillconst.ServicePropertyIsConnected: true,
		}, shillconst.DefaultTimeout); err != nil {
			return errors.Wrap(err, "failed to verify the VPN service is saved")
		}

		if err := ossettings.DisconnectVPN(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to disconnect from VPN service")
		}

		res.cleanupVPNEnv = cleanup
		return nil
	}
}

func verifyVPNNotExist(res *vpnAreUserSpecifiedTestResource) vpnAreUserSpecifiedTestAction {
	return func(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
		defer cancel()

		settings, err := ossettings.Launch(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to launch the OS settings")
		}
		defer settings.Close(cleanupCtx)
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, cr, "ossettings_ui_dump")

		if err := settings.NavigateToPageURL(ctx, cr, "internet", settings.Exists(ossettings.Internet)); err != nil {
			return errors.Wrap(err, "failed to navigate to network page")
		}

		ui := uiauto.New(tconn)
		arrowFinder := nodewith.Role(role.Button).HasClass("subpage-arrow")

		vpnNetworksPageArrow := arrowFinder.NameContaining("VPN")
		if err = ui.WaitUntilExists(vpnNetworksPageArrow)(ctx); err != nil {
			if nodewith.IsNodeNotFoundErr(err) {
				// VPN networks page is available when a VPN is configured, also,
				// 	1. VPN configured by other users should be invisible.
				// 	2. Second user has not configured any other VPN service.
				// Therefore, the absence of the subpage-arrow of the VPN networks
				// page is expected, no further actions are required.
				return nil
			}
			return errors.Wrap(err, "failed to determine whether the VPN networks page is enabled")
		}

		// If the user has a VPN configured, enter the VPN networks page to verify
		// whether the VPN configured by other users does exist.
		if err := ui.LeftClick(vpnNetworksPageArrow)(ctx); err != nil {
			return errors.Wrap(err, "failed to enter VPN networks page")
		}

		vpnServicePageArrow := arrowFinder.NameContaining(vpnNetworkName)
		if err := ui.WaitUntilExists(vpnServicePageArrow)(ctx); err != nil {
			if nodewith.IsNodeNotFoundErr(err) {
				// No further actions are required, as VPN services configured by
				// other users are expected to be invisible.
				return nil
			}
			return errors.Wrap(err, "failed to determine whether the VPN service page is found")
		}
		return errors.Wrap(err, "the VPN service is not user specific")
	}
}
