// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type signalStrengthStats struct {
	Average   float32 `json:"average"`
	Deviation float32 `json:"deviation"`
	Samples   []uint8 `json:"samples"`
}

type networkInfo struct {
	PortalState         string               `json:"portal_state"`
	State               string               `json:"state"`
	Type                string               `json:"type"`
	GUID                *string              `json:"guid"`
	Ipv4Address         *string              `json:"ipv4_address"`
	Ipv6Addresses       *string              `json:"ipv6_addresses"`
	MacAddress          *string              `json:"mac_address"`
	Name                *string              `json:"name"`
	SignalStrength      *jsontypes.Uint32    `json:"signal_strength"`
	SignalStrengthStats *signalStrengthStats `json:"signal_strength_stats"`
}

type networkResult struct {
	Networks []networkInfo `json:"networks"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeNetworkInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for network info",
		Contacts: []string{
			"cros-network-health-team@google.com",
			"cros-tdm-tpe-eng@google.com",
			"khegde@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
	})
}

func ProbeNetworkInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryNetwork}

	// If this test is run right after chrome is started, it's possible that the
	// network health information has not been populated. Poll the routine until
	// network information is present.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var result networkResult
		if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &result); err != nil {
			s.Fatal("Failed to run telem command: ", err)
		}

		// Every system should have at least one network device populated. If
		// not, re-poll the routine.
		if len(result.Networks) < 1 {
			return errors.New("no network info populated")
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Timed out waiting for network health info: ", err)
	}
}
