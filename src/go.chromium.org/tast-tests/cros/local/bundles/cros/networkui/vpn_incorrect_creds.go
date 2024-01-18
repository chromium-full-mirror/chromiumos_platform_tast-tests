// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/vpn"
	vpnManager "go.chromium.org/tast-tests/cros/local/networkui/vpn"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type vpnIncorrectCredsTestParam struct {
	// The error message is expected to be displayed when incorrect credentials are provided.
	errMsg string
	// The subject property should be fed with an incorrect value to trigger the "incorrect credentials" response.
	subjectProperty string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VPNIncorrectCreds,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that device fails to connect to VPN with incorrect credentials",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent:   "b:1131913", // ChromeOS > Software > System Services > Connectivity > VPN
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:network", "network_e2e"},
		SoftwareDeps:   []string{"chrome"},
		Fixture:        "vpnEnvWithCertsAndChromeLoggedIn",
		Timeout:        4 * time.Minute,
		Params: []testing.Param{{
			Name: "username",
			Val: &vpnIncorrectCredsTestParam{
				errMsg:          "PPP authentication failed due to an incorrect username or password",
				subjectProperty: "L2TPIPsec.User",
			},
		}, {
			Name: "password",
			Val: &vpnIncorrectCredsTestParam{
				errMsg:          "PPP authentication failed due to an incorrect username or password",
				subjectProperty: "L2TPIPsec.Password",
			},
		}, {
			Name: "psk",
			Val: &vpnIncorrectCredsTestParam{
				errMsg:          "Connect failed",
				subjectProperty: "L2TPIPsec.PSK",
			},
		}},
	})
}

// VPNIncorrectCreds verifies that device fails to connect to VPN with incorrect credentials.
func VPNIncorrectCreds(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fv := s.FixtValue().(vpn.FixtureEnv)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Prepare virtualnet environment for the VPN server.
	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology for VPN tests: ", err)
	}
	defer func(ctx context.Context) {
		if err := networkEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
	}(cleanupCtx)

	// Prepare VPN server.
	config := vpn.NewConfig(
		vpn.TypeL2TPIPsec,
		vpn.WithIPsecAuthType(vpn.AuthTypePSK),
	)
	vpnServer, err := vpn.StartServerWithConfig(ctx, networkEnv.Server1, config)
	if err != nil {
		s.Fatal("Failed to start VPN server: ", err)
	}
	defer vpnServer.Exit(cleanupCtx)

	// Retrieve the VPN server properties for configuration via UI.
	vpnProps, err := vpn.CreateProperties(vpnServer, nil /*secondServer*/)
	if err != nil {
		s.Fatal("Failed to generate D-Bus properties: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	const (
		// The VPN service name.
		vpnName          = "TestVPN"
		incorrectPropVal = "incorrect"
	)

	props := make(map[string]interface{})
	for key, value := range vpnProps.GetPropertiesMap() {
		props[key] = value
	}

	clientCertName := fmt.Sprintf("%s [%s]", fv.CertVals.Credentials.CACred.Info.CommonName, fv.CertVals.Credentials.ClientCred.Info.CommonName)
	tc := s.Param().(*vpnIncorrectCredsTestParam)
	// Update VPN properties based on the incorrect credential of the current test case.
	props[tc.subjectProperty] = incorrectPropVal
	vpnProps.SetPropertiesMap(props)

	settings, err := ossettings.OpenJoinVPNDialog(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open VPN dialog: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "vpn_settings_ui_dump")

	vpnHelper, err := vpnManager.NewDialogHelper(config.Type, &vpnServer.Config, vpnProps, vpnName, &clientCertName)
	if err != nil {
		s.Fatal("Failed to create a UI helper: ", err)
	}

	if err := vpnHelper.FillInConfigurations(ctx, cr, tconn, kb); err != nil {
		s.Fatal("Failed to configure vpn service: ", err)
	}

	if err := connectToVPNAndVerifyErrMsg(ctx, tconn, vpnName, tc.errMsg); err != nil {
		s.Fatal("Failed to verify the error message: ", err)
	}
}

// connectToVPNAndVerifyErrMsg connects to a VPN and verifies the error message is as expected.
func connectToVPNAndVerifyErrMsg(ctx context.Context, tconn *chrome.TestConn, vpnName, errMsg string) error {
	settings := ossettings.New(tconn)
	if err := settings.LeftClick(nodewith.Name("Connect").Role(role.Button))(ctx); err != nil {
		return errors.Wrap(err, "failed to click on the Connect button")
	}

	// The error message when using incorrect psk will appear within around 40 seconds.
	if _, err := ash.WaitForNotification(ctx, tconn, 40*time.Second, ash.WaitMessageContains(errMsg)); err != nil {
		return errors.Wrap(err, "failed to observe the expected error message")
	}

	return nil
}
