// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/shill"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillDisableEnableTechnology,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensures that the Ethernet technology can be disabled and enabled by Shill",
		Contacts: []string{
			"cros-network-health-team@google.com", // network-health team
			"khegde@chromium.org",                 // test maintainer
			"stevenjb@chromium.org",               // network-health tech lead
		},
		BugComponent: "b:1166446",
		SoftwareDeps: []string{"no_qemu"},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "shillReset",
	})
}

func ShillDisableEnableTechnology(ctx context.Context, s *testing.State) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
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
