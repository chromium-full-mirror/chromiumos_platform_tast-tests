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

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/wificell"
	"chromiumos/tast/services/cros/network"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProxyRetainAfterReboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the proxy values remain the same after DUT reboots",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
			"cienet-development@googlegroups.com",
			"edgar.chang@cienet.com",
		},
		BugComponent: "b:1131775", // ChromeOS > Software > System Services > Connectivity
		Attr:         []string{"group:network", "network_e2e_unstable"},
		ServiceDeps: []string{
			"tast.cros.network.ProxySettingService",
			wificell.TFServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Timeout:      10 * time.Minute,
		Fixture:      "wificellFixt",
	})
}

// ProxyRetainAfterReboot tests that the proxy values remain the same after DUT reboots.
func ProxyRetainAfterReboot(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	manifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	proxyConfigs := &network.ProxyConfigs{
		HttpHost:  "localhost",
		HttpPort:  "123",
		HttpsHost: "localhost",
		HttpsPort: "456",
		SocksHost: "socks5://localhost",
		SocksPort: "8080",
	}

	setUpBeforeReboot := func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		rpcClient := tf.DUTRPC(wificell.DefaultDUT)
		proxySettingSvc := network.NewProxySettingServiceClient(rpcClient.Conn)
		if _, err := proxySettingSvc.New(ctx, &network.NewRequest{ManifestKey: manifestKey}); err != nil {
			return errors.Wrap(err, "failed to create a new proxy setting service")
		}
		defer proxySettingSvc.Close(cleanupCtx, &empty.Empty{})

		if _, err := proxySettingSvc.Setup(ctx, proxyConfigs); err != nil {
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
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	proxySettingSvc := network.NewProxySettingServiceClient(rpcClient.Conn)
	if _, err := proxySettingSvc.New(ctx, &network.NewRequest{ManifestKey: manifestKey}); err != nil {
		s.Fatal("Failed to create a new proxy setting service: ", err)
	}
	defer proxySettingSvc.Close(cleanupCtx, &empty.Empty{})

	returnedConfigs, err := proxySettingSvc.FetchConfigurations(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to fetch proxy configurations: ", err)
	}

	if diff := cmp.Diff(returnedConfigs, proxyConfigs, protocmp.Transform()); diff != "" {
		s.Fatalf("Unexpected proxy values (-want +got): %s", diff)
	}
}
