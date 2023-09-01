// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uwb

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/uwb"
	"go.chromium.org/tast/core/testing"
)

type phoneSmokeParam struct {
	minDeviceCount int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhoneSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that experimental testbeds with multiple Android peers can be accessed in lab infrastructure",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
			"jstanko@google.com",
		},
		BugComponent: "b:1133455",
		Attr:         []string{"group:uwb", "uwb_unstable"},
		Timeout:      10 * time.Minute,
		Fixture:      "uwbMultiAndroid",
		Params: []testing.Param{
			{
				Name:      "devices_1",
				ExtraAttr: []string{"uwb_android_peers_1"},
				Val:       &phoneSmokeParam{minDeviceCount: 1},
			},
			{
				Name:      "devices_2",
				ExtraAttr: []string{"uwb_android_peers_2"},
				Val:       &phoneSmokeParam{minDeviceCount: 2},
			},
		},
	})
}

func PhoneSmoke(ctx context.Context, s *testing.State) {
	fixtData := s.FixtValue().(*uwb.FixtData)
	p := s.Param().(*phoneSmokeParam)
	if len(fixtData.PhonePeers) < p.minDeviceCount {
		s.Fatalf("Failed to verify number of phone peers, expected at least %d, got %d", p.minDeviceCount, len(fixtData.PhonePeers))
	}
}
