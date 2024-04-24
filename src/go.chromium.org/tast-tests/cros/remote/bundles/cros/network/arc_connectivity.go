// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/network/allowlist"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const arcConnectivityTestTimeout = 12 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         ArcConnectivity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that the PlayStore works behind a firewall configured according to our support page",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"acostinas@google.com",                      // Test author
		},
		BugComponent: "b:1000044",
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{"allowlist_ssl_inspection.json"},
		ServiceDeps: []string{"tast.cros.network.AllowlistService",
			"tast.cros.networkui.ProxyService",
			"tast.cros.tape.Service"},
		Timeout:      arcConnectivityTestTimeout,
		SoftwareDeps: []string{"reboot", "chrome", "chrome_internal", "tpm2"},
		Params: []testing.Param{{
			// TODO(b/335124753): mitmproxy occasionally fails to start on arm devices.
			// Remove architecture restrictions when the problem is solved.
			ExtraSoftwareDeps: []string{"android_vm", "no_arm"},
		}},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

// ArcConnectivity calls the AllowlistService to setup a firewall and verifies
// PlayStore connectivity.
func ArcConnectivity(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(cleanupCtx)

	if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Read the hostnames that need to be allowed for installing the PlayStore
	// and Android apps behind a firewall.
	allowlist, err := allowlist.ReadHostnames(ctx,
		s.DataPath("allowlist_ssl_inspection.json"), true, false)
	if err != nil {
		s.Fatal("Failed to read hostnames: ", err)
	}

	// Start an HTTP proxy instance on the DUT which only allows connections to
	// the allowlisted hostnames.
	proxyClient := networkui.NewProxyServiceClient(cl.Conn)
	response, err := proxyClient.StartServer(ctx,
		&networkui.StartServerRequest{
			Allowlist: allowlist,
		})

	if err != nil {
		s.Fatal("Failed to start a local proxy on the DUT: ", err)
	}
	defer func() {
		if _, err := proxyClient.StopServer(cleanupCtx, &empty.Empty{}); err != nil {
			s.Log("Failed to stop a local proxy on the DUT: ", err)
		}
	}()

	portStr := strings.Split(response.HostAndPort, ":")[1]
	port, err := strconv.ParseUint(portStr, 10, 32)

	if err != nil {
		s.Fatal("Failed to parse proxy server port: ", err)
	}

	al := network.NewAllowlistServiceClient(cl.Conn)
	if _, err := al.SetupFirewall(ctx, &network.SetupFirewallRequest{AllowedPort: uint32(port)}); err != nil {
		s.Fatal("Failed to setup a firewall on the DUT: ", err)
	}

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(arcConnectivityTestTimeout.Seconds())
	poolID := tape.DefaultManaged

	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true, /*lock*/
		tape.WithTimeout(timeout), tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	arcEnabledPolicy := &tape.AndroidAppsEnabledUsersAppsconfig{ArcEnabled: true, AckNoticeForArcEnabledSetToTrue: true}

	if err := tapeClient.SetPolicy(ctx, arcEnabledPolicy, []string{"arcEnabled",
		"ackNoticeForArcEnabledSetToTrue"}, nil, acc.RequestID); err != nil {
		s.Fatal("Failed to set the ARC enabled policy: ", err)
	}
	arcPolicy := &tape.InstallTypeUsersApps{
		AppInstallType: tape.USERAPPINSTALLTYPE_FORCED}
	if err := tapeClient.SetPolicy(ctx, arcPolicy, []string{"appInstallType"},
		tape.AppKey{AppID: "android:com.google.android.gm"}, acc.RequestID); err != nil {
		s.Fatal("Failed to set the force-installed APP policy: ", err)
	}

	// Deprovision the DUT at the end of the test. As devices might get
	// provisioned even when the enrollment fails we need to defer the
	// deprovisioning before enrolling.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(cleanupCtx, cl, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	if _, err := al.GaiaLogin(ctx, &network.GaiaLoginRequest{
		Username: acc.Username, Password: acc.Password, ProxyHostAndPort: response.HostAndPort}); err != nil {
		s.Fatal("Failed to login through the proxy: ", err)
	}

	defer policyClient.StopChrome(ctx, &empty.Empty{})

	// The user account allowlist.username/allowlist.password belongs to the OU
	// allowlist-tast-test on the production DMServer.
	// The OU is configured to force install the Gmail app via policy.
	if _, err := al.CheckArcAppInstalled(ctx, &network.CheckArcAppInstalledRequest{AppName: "com.google.android.gm"}); err != nil {
		s.Fatal("Failed to install ARC app: ", err)
	}

}
