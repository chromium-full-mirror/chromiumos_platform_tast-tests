// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package labqual

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECControlRead,
		Desc: "Read various EC controls from servo",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		Timeout:      1 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func ECControlRead(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireRPCUtils(ctx); err != nil {
		s.Fatal("Requiring RPC utils: ", err)
	}

	for _, tc := range []struct {
		name    string
		control servo.StringControl // string control
	}{
		{"EC Chip", servo.ECChip},
		{"EC System Power State", servo.ECSystemPowerState},
		{"EC UART Cmd", servo.ECUARTCmd},
		{"Lid Open", servo.LidOpen},
		{"Servo DUT SBU1 MV", servo.ServoDUTSBU1MV},
		{"Servo DUT SBU2 MV", servo.ServoDUTSBU2MV},
	} {
		controlRead, err := h.Servo.GetString(ctx, tc.control)
		if err != nil {
			s.Errorf("Failed to retrieve %s: %v", tc.name, err)
		} else {
			s.Logf("%s: %s", tc.name, controlRead)
		}
	}

	for _, tc := range []struct {
		name    string
		control servo.OnOffControl // on-off control
	}{
		{"EC UART Flush", servo.ECUARTFlush},
	} {
		controlRead, err := h.Servo.GetOnOff(ctx, tc.control)
		if err != nil {
			s.Errorf("Failed to retrieve %s: %v", tc.name, err)
		} else {
			s.Logf("%s: %t", tc.name, controlRead)
		}
	}
}
