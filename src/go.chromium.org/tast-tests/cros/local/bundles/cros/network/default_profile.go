// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DefaultProfile,
		Desc:         "Checks shill's default network profile",
		BugComponent: "b:156085",
		Contacts: []string{
			"cros-networking@google.com",
			"stevenjb@chromium.org", // Connectivity team
			"nya@chromium.org",      // Tast port author
		},
		Attr:         []string{"group:mainline", "group:network", "network_cq"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("winky")), // b/182293895: winky DUTs are having USB Ethernet issues that surface during `restart shill`
	})
}

func DefaultProfile(ctx context.Context, s *testing.State) {
	expectedSettings := []string{
		"CheckPortalList=ethernet,wifi,cellular",
	}

	// We lose connectivity briefly. Tell recover_duts not to worry.
	unlock, err := network.LockCheckNetworkHook(ctx)
	if err != nil {
		s.Fatal("Failed to lock the check network hook: ", err)
	}
	defer unlock()

	// Stop shill temporarily and remove the default profile.
	if err := upstart.StopJob(ctx, shill.JobName); err != nil {
		s.Fatal("Failed stopping shill: ", err)
	}
	os.Remove(shillconst.DefaultProfilePath)
	if err := upstart.RestartJob(ctx, shill.JobName); err != nil {
		s.Fatal("Failed starting shill: ", err)
	}

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}

	// Wait for default profile creation.
	testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(shillconst.DefaultProfilePath); err != nil {
			return err
		}

		paths, err := manager.ProfilePaths(ctx)
		if err != nil {
			s.Fatal("Failed getting profiles: ", err)
		}

		for _, p := range paths {
			if p == shillconst.DefaultProfileObjectPath {
				return nil
			}
		}
		return errors.New("default profile object not ready yet")
	}, &testing.PollOptions{Timeout: 5 * time.Second})

	// Read the default profile and check expected settings.
	b, err := ioutil.ReadFile(shillconst.DefaultProfilePath)
	if err != nil {
		s.Fatal("Failed reading the default profile: ", err)
	}
	ioutil.WriteFile(filepath.Join(s.OutDir(), "default.profile"), b, 0644)
	profile := string(b)

	for _, es := range expectedSettings {
		if !strings.Contains(profile, es) {
			s.Error("Expected setting not found: ", es)
		}
	}
}
