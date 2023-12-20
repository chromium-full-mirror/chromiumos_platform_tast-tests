// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefaultProfileServices,
		Desc:         "Checks configured services persist across shill reboot",
		BugComponent: "b:156085",
		Contacts: []string{
			"cros-networking@google.com",
			"stevenjb@chromium.org", // Connectivity team
			"oka@chromium.org",      // Tast port author
		},
		Attr:         []string{"group:mainline", "group:network", "network_cq"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("winky")), // b/182293895: winky DUTs are having USB Ethernet issues that surface during `restart shill`
	})
}

func DefaultProfileServices(ctx context.Context, s *testing.State) {
	const (
		// ssid is a fake service name chosen unlikely to match any SSID present over-the-air.
		ssid = "org.chromium.DfltPrflSrvcsTest"
	)

	// We lose connectivity along the way here, and if that races with the
	// recover_duts network-recovery hooks, it may interrupt us.
	unlock, err := network.LockCheckNetworkHook(ctx)
	if err != nil {
		s.Fatal("Failed to lock the check network hook: ", err)
	}
	defer unlock()

	func() {
		// Stop shill temporarily and remove the default profile.
		if err := upstart.StopJob(ctx, shill.JobName); err != nil {
			s.Fatal("Failed stopping shill: ", err)
		}
		defer func() {
			if err := upstart.RestartJob(ctx, shill.JobName); err != nil {
				s.Fatal("Failed starting shill: ", err)
			}
		}()
		// TODO(oka): It's possible that the default profile has been
		// removed by the previous test, and this test has started
		// before the default profile is created by the previous test's
		// (re)starting of Shill. It's a confusing race condition, so
		// fix it by making sure that the default profile exists here.
		if err := os.Remove(shillconst.DefaultProfilePath); err != nil && !os.IsNotExist(err) {
			s.Fatal("Failed removing default profile: ", err)
		}
	}()

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}
	if err := manager.PopAllUserProfiles(ctx); err != nil {
		s.Fatal("Failed to pop user profiles: ", err)
	}

	if _, err := manager.ConfigureService(ctx, map[string]interface{}{
		shillconst.ServicePropertyType:           "wifi",
		shillconst.ServicePropertyMode:           "managed",
		shillconst.ServicePropertySSID:           ssid,
		shillconst.ServicePropertyWiFiHiddenSSID: true,
		shillconst.ServicePropertySecurityClass:  "none",
	}); err != nil {
		s.Fatal("Failed to configure service: ", err)
	}

	// Restart shill to ensure that configurations persist across reboot.
	if err := upstart.StopJob(ctx, shill.JobName); err != nil {
		s.Fatal("Failed stopping shill: ", err)
	}
	if err := upstart.RestartJob(ctx, shill.JobName); err != nil {
		s.Fatal("Failed starting shill: ", err)
	}

	manager, err = shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}
	if err := manager.PopAllUserProfiles(ctx); err != nil {
		s.Fatal("Failed to pop user profiles: ", err)
	}

	expectProp := map[string]interface{}{
		shillconst.ServicePropertyName: ssid,
	}

	if _, err := manager.WaitForServiceProperties(ctx, expectProp, 5*time.Second); err != nil {
		s.Error("Network not found after restart: ", err)
	}
}
