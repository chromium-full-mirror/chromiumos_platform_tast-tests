// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ShillDisableEnableTechnology,
		Desc: "Ensures that the Ethernet technology can be disabled and enabled by Shill",
		Contacts: []string{
			"cros-networking@google.com", // Platform networking team: owner/maintainer
			"stevenjb@chromium.org",      // author
			"khegde@chromium.org",        // author
		},
		BugComponent: "b:1493959", // ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		SoftwareDeps: []string{"no_qemu"},
		Attr:         []string{"group:mainline", "group:release-health", "release-health_network"},
		Fixture:      "shillReset.ehide",
	})
}

func ShillDisableEnableTechnology(ctx context.Context, s *testing.State) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	// Setup a virtual ethernet interface, because the physical ethernet
	// interface is hidden by the ehide fixture.
	pool := subnet.NewPool()
	opt := virtualnet.EnvOptions{
		EnableDHCP: true,
	}
	svc, router, err := virtualnet.CreateRouterEnv(ctx, m, pool, opt)
	if err != nil {
		s.Fatal("Failed to create router env: ", err)
	}
	defer router.Cleanup(ctx)
	if err := svc.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait router being connected: ", err)
	}

	if enabled, err := m.IsEnabled(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Error calling IsEnabled: ", err)
	} else if !enabled {
		s.Fatal("Ethernet not enabled")
	}
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := m.DisableTechnology(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Failed to disable Ethernet: ", err)
	}
	const interval = 100 * time.Millisecond
	testing.Poll(ctx, func(ctx context.Context) error {
		enabled, err := m.IsEnabled(ctx, shill.TechnologyEthernet)
		if err != nil {
			return errors.Wrap(err, "failed to get enabled state")
		}
		if enabled {
			return errors.New("ethernet not disabled")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  5 * time.Second,
		Interval: interval,
	})

	if err := m.EnableTechnology(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Failed to enable Ethernet: ", err)
	}
	testing.Poll(ctx, func(ctx context.Context) error {
		enabled, err := m.IsEnabled(ctx, shill.TechnologyEthernet)
		if err != nil {
			return errors.Wrap(err, "failed to get enabled state")
		}
		if !enabled {
			return errors.New("ethernet not enabled")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  5 * time.Second,
		Interval: interval,
	})
}
