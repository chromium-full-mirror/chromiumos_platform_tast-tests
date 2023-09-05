// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	types "go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	nc "go.chromium.org/tast-tests/cros/local/network/netconfig"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type testParam struct {
	login bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConfigBaseline,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Use the CrosNetworkConfig API during OOBE and after login to configure a basic network and check it is set as expected",
		Contacts: []string{
			"cros-connectivity@google.com",
			"crisguerrero@chromium.org", // Test author
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			Name: "oobe",
			Val: testParam{
				login: false,
			},
			Timeout: 1 * time.Minute,
		}, {
			Name: "login",
			Val: testParam{
				login: true,
			},
			Timeout: chrome.GAIALoginTimeout + 30*time.Second,
		},
		},
	})
}

// ConfigBaseline tests a basic network configuration during OOBE or after login
// using the CrosNetworkConfig API. This test is intended as a baseline example
// of how to use such API and make sure it works.
// Various more complex tests rely on the network API to work, so it is
// important it runs in the CQ to detect possible issues with it on time.
func ConfigBaseline(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	param := s.Param().(testParam)
	var opts []chrome.Option
	if !param.login {
		// Defer login to use the API during OOBE.
		opts = []chrome.Option{
			chrome.DeferLogin(),
		}
	}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// How we connect to the API depends on if we are in the OOBE or have access
	// to a chrome instance after login.
	var api *nc.CrosNetworkConfig
	if param.login {
		// Connect using chrome.
		api, err = netconfig.CreateLoggedInCrosNetworkConfig(ctx, cr)
	} else {
		// Connect using OOBE.
		api, err = netconfig.CreateOobeCrosNetworkConfig(ctx, cr)
	}
	if err != nil {
		s.Fatal("Failed to get network Mojo Object: ", err)
	}
	defer api.Close(cleanupCtx)

	var configProperties = types.ConfigProperties{
		TypeConfig: types.NetworkTypeConfigProperties{
			Wifi: &types.WiFiConfigProperties{
				Ssid:       "basicWifi",
				Security:   types.None,
				HiddenSsid: types.Automatic}}}

	guid, err := api.ConfigureNetwork(ctx, configProperties, true /*shared*/)
	if err != nil {
		s.Fatal("Failed to configure network: ", err)
	}
	defer func(ctx context.Context) {
		if success, err := api.ForgetNetwork(ctx, guid); err != nil {
			s.Fatalf("Failed to forget network with guid %s: %v", guid, err)
		} else if success != true {
			s.Fatalf("Request to forget network with guid %s was not successful", guid)
		}
	}(cleanupCtx)

	managedProperties, err := api.GetManagedProperties(ctx, guid)
	if err != nil {
		s.Fatalf("Failed to get managed properties for guid %s: %v", guid, err)
	}

	if managedProperties.TypeProperties.Wifi.Security !=
		configProperties.TypeConfig.Wifi.Security ||
		managedProperties.TypeProperties.Wifi.Ssid.ActiveValue !=
			configProperties.TypeConfig.Wifi.Ssid {
		s.Errorf("Set and expected config of test network do not match. Got: %+v. Want: %+v", managedProperties.TypeProperties.Wifi, configProperties.TypeConfig.Wifi)
	}
}
