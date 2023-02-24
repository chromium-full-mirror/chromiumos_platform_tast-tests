// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"chromiumos/tast/common/firmware/ti50"
	"chromiumos/tast/remote/firmware/ti50/fixture"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50EcReset,
		Desc:    "Test workaround for EC double reset",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		Fixture:      fixture.Ti50,
	})
}

func Ti50EcReset(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)

	board, err := f.DevBoard(ctx, 10000, time.Second)
	if err != nil {
		s.Fatal("Could not get board: ", err)
	}

	i := ti50.NewCrOSImage(board)

	if _, err = board.OpenTitanToolCommand(ctx, "transport", "init"); err != nil {
		s.Fatal("Failed to reset gpio to good state: ", err)
	}

	_, err = board.OpenTitanToolCommand(ctx, "gpio", "monitoring", "start", ti50.GpioTi50ResetL.GpioName(), ti50.GpioTi50EcRstL.GpioName(), ti50.GpioTi50EcRstFet.GpioName())
	if err != nil {
		s.Fatal("OpenTitanToolCommand: ", err)
	}

	testing.ContextLog(ctx, "Restarting ti50")
	if err = board.Reset(ctx); err != nil {
		s.Fatal("Failed to reset: ", err)
	}
	if err = i.WaitUntilBooted(ctx); err != nil {
		s.Fatal("Ti50 did revive after reboot: ", err)
	}

	eventData, err := board.OpenTitanToolCommand(ctx, "gpio", "monitoring", "read", ti50.GpioTi50ResetL.GpioName(), ti50.GpioTi50EcRstL.GpioName(), ti50.GpioTi50EcRstFet.GpioName())
	if err != nil {
		s.Fatal("OpenTitanToolCommand: ", err)
	}

	events := eventData["events"].([]interface{})
	if len(events) != 4 {
		s.Fatal("Unexpected number of events")
	}
	event1 := events[0].(map[string]interface{})
	if event1["signal_name"].(string) != ti50.GpioTi50ResetL.GpioName() || event1["edge"] != "Falling" {
		s.Error("Unexpected first event: ", event1)
	}
	event2 := events[1].(map[string]interface{})
	if event2["signal_name"].(string) != ti50.GpioTi50EcRstFet.GpioName() || event2["edge"] != "Rising" {
		s.Error("Unexpected second event: ", event2)
	}
	event3 := events[2].(map[string]interface{})
	if event3["signal_name"].(string) != ti50.GpioTi50ResetL.GpioName() || event3["edge"] != "Rising" {
		s.Error("Unexpected third event: ", event3)
	}
	event4 := events[3].(map[string]interface{})
	if event4["signal_name"].(string) != ti50.GpioTi50EcRstFet.GpioName() || event4["edge"] != "Falling" {
		s.Error("Unexpected fourth event: ", event4)
	}
}
