// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/network"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ProxyRetainAfterReboot,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Tests that the proxy values remain the same after DUT reboots",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		// TODO(b/275127708): Move this test to network suite.
		Attr: []string{"group:wificell", "wificell_e2e"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.network.ProxySettingService",
			"tast.cros.ui.ChromeUIService",
			wificell.ShillServiceName,
		},
		// TODO (b/284498579): Remove hwdep once issue is resolved.
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Timeout:      10 * time.Minute,
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
	})
}

// resetProxyTimeout defines the timeout to reset proxy config since it's a combination of UI actions and could take a while.
const resetProxyTimeout = 35 * time.Second

// ProxyRetainAfterReboot tests that the proxy values remain the same after DUT reboots.
func ProxyRetainAfterReboot(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	manifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	proxyConfigs := wificell.DefaultProxyConfigForEthernet()

	resetProxyConfig := func(ctx context.Context, rpcClient *rpc.Client) error {
		proxySettingSvc := network.NewProxySettingServiceClient(rpcClient.Conn)
		_, err := proxySettingSvc.ResetConnectionType(ctx, &network.ResetConnectionTypeRequest{
			NetworkInfo: &network.NetworkInfo{
				Value: &network.NetworkInfo_Ethernet{},
			},
		})
		return err
	}

	setUpBeforeReboot := func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, resetProxyTimeout)
		defer cancel()

		rpcClient := tf.DUTRPC(wificell.DefaultDUT)
		crSvc := pb.NewChromeServiceClient(rpcClient.Conn)
		chromeUISvc := pb.NewChromeUIServiceClient(rpcClient.Conn)
		if err := bootToOOBE(ctx, crSvc, chromeUISvc, false /* keepState */, manifestKey); err != nil {
			s.Fatal("Failed to boot DUT to OOBE screen: ", err)
		}
		defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

		proxySettingSvc := network.NewProxySettingServiceClient(rpcClient.Conn)
		defer func(ctx context.Context) {
			if s.HasError() {
				resetProxyConfig(ctx, rpcClient)
			}
		}(cleanupCtx)

		if _, err := proxySettingSvc.SetProxySettings(ctx, &network.SetProxySettingsRequest{
			Configs:   proxyConfigs,
			LoginMode: network.LoginMode_OOBE,
		}); err != nil {
			return errors.Wrap(err, "failed to setup proxy")
		}
		return nil
	}

	if err := setUpBeforeReboot(ctx); err != nil {
		s.Fatal("Failed to set up proxy before reboot: ", err)
	}

	if err := tf.RebootDUT(ctx, wificell.DefaultDUT); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, resetProxyTimeout)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := pb.NewChromeServiceClient(rpcClient.Conn)
	chromeUISvc := pb.NewChromeUIServiceClient(rpcClient.Conn)
	if err := bootToOOBE(ctx, crSvc, chromeUISvc, true /* keepState */, manifestKey); err != nil {
		s.Fatal("Failed to boot DUT to OOBE screen: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	proxySettingSvc := network.NewProxySettingServiceClient(rpcClient.Conn)
	defer resetProxyConfig(cleanupCtx, rpcClient)

	returnedConfigs, err := proxySettingSvc.FetchProxySettings(ctx, &network.FetchProxySettingsRequest{
		NetworkInfo: &network.NetworkInfo{Value: &network.NetworkInfo_Ethernet{}},
		LoginMode:   network.LoginMode_OOBE,
	})
	if err != nil {
		s.Fatal("Failed to fetch proxy configurations: ", err)
	}

	if diff := cmp.Diff(returnedConfigs, proxyConfigs, protocmp.Transform()); diff != "" {
		s.Fatalf("Unexpected proxy values (-want +got): %s", diff)
	}
}

func bootToOOBE(ctx context.Context, crSvc pb.ChromeServiceClient, chromeUISvc pb.ChromeUIServiceClient, keepState bool, manifestKey string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if _, err := crSvc.New(ctx, &pb.NewRequest{
		LoginMode:                    pb.LoginMode_LOGIN_MODE_NO_LOGIN,
		KeepState:                    keepState,
		SigninProfileTestExtensionId: manifestKey,
	}); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer func() {
		if retErr != nil {
			crSvc.Close(cleanupCtx, &empty.Empty{})
		}
	}()

	// Waiting for the OOBE to be stabled and ready for test to avoid the unexpected display rendering event and
	// causes proxy-setting-service-client unable to complete its action.
	if _, err := chromeUISvc.WaitForWelcomeScreen(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to wait for OOBE is ready for testing")
	}
	return nil
}
