// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	types "go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/local/network/netconfig"
	"go.chromium.org/tast-tests/cros/local/shill"
	f "go.chromium.org/tast-tests/cros/local/wifi"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const networkSSID = "NonExistentNetwork"

// We use command line flags to cause wrongly hidden networks to be removed every 1 second.
// We wait 5 seconds to guarantee that we will always have this migration triggered at least once during each test.
const migrateTimeout = time.Second * 5

type testConfig struct {
	hidden bool
	shared bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HiddenNetworksAreMigrated,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that hidden networks are migrated, for more details see go/cros-hidden-ssid-dd-software",
		Contacts:     []string{"cros-connectivity@google.com", "chadduffin@google.com"},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "hiddenNetworkMigration",
		Params: []testing.Param{{
			Name: "not_shared_and_not_hidden",
			Val: &testConfig{
				hidden: false,
				shared: false,
			},
		}, {
			Name: "not_shared_and_hidden",
			Val: &testConfig{
				hidden: true,
				shared: false,
			},
		}, {
			Name: "shared_and_not_hidden",
			Val: &testConfig{
				hidden: false,
				shared: true,
			},
		}, {
			Name: "shared_and_hidden",
			Val: &testConfig{
				hidden: true,
				shared: true,
			},
		}},
		Timeout: time.Second * 60,
	})
}

func HiddenNetworksAreMigrated(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*f.HiddenNetworkMigrationFixtureData).Chrome

	// We interact with Shill directly to handle checking whether a network is still known,
	// and to make sure that we forget the network we configured before completing the test.
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}

	wifiManager, err := shill.NewWifiManager(ctx, manager)
	if err != nil {
		s.Fatal("Could not get a WiFi interface: ", err)
	}

	// Use a test profile so that we don't need to clean up the network afterwards.
	popFunc, err := manager.PushTestProfile(ctx)
	if err != nil {
		s.Fatal("Failed to push test profile: ", err)
	}
	defer popFunc(ctx)
	// Shorten deadline to leave time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	networkConfig, err := netconfig.CreateOobeCrosNetworkConfig(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get CrosNetworkConfig connection before logging in")
	}

	isHidden := s.Param().(*testConfig).hidden
	isShared := s.Param().(*testConfig).shared

	var wifiConfigProperties *types.WiFiConfigProperties
	if isHidden {
		wifiConfigProperties = &types.WiFiConfigProperties{
			Ssid:       networkSSID,
			Security:   types.None,
			HiddenSsid: types.Enabled,
		}
	} else {
		wifiConfigProperties = &types.WiFiConfigProperties{
			Ssid:       networkSSID,
			Security:   types.None,
			HiddenSsid: types.Disabled,
		}
	}

	// One of the requirements for a network to be eligible to be migrated is that it must never have been connected to,
	// so next we configure a network that does not actually exist.
	if _, err := networkConfig.ConfigureNetwork(ctx, types.ConfigProperties{
		TypeConfig: types.NetworkTypeConfigProperties{
			Wifi: wifiConfigProperties,
		}}, isShared /*shared*/); err != nil {
		s.Fatal("Failed to configure network: ", err)
	}

	// Hidden networks should be migrated, and not hidden networks should be left alone.
	if isHidden {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if found, err := wifiManager.HasMatchingAP(ctx, networkSSID); err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to determine whether network has been forgotten"))
			} else if found {
				return errors.New("Network has not been forgotten")
			}
			return nil
		}, &testing.PollOptions{Timeout: migrateTimeout, Interval: time.Second}); err != nil {
			s.Fatal("Failed to migrate the hidden network: ", err)
		}
	} else {
		// GoBigSleepLint: We sleep instead of polling since we expect that the network is not forgetten by the migration code.
		// The migration code is triggered once every minute, and since we don't know when the last time it was triggered was
		// we must wait at least one full minute to guarantee it has run.
		testing.Sleep(ctx, migrateTimeout)

		if found, err := wifiManager.HasMatchingAP(ctx, networkSSID); err != nil {
			s.Fatal("Failed to determine whether netowrk has been forgotten: ", err)
		} else if !found {
			s.Fatal("Network has been forgotten")
		}
	}
}
