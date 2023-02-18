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

	board := f.DevBoard()
	i := ti50.NewCrOSImage(board)

	_, err := board.OpenTitanToolCommand(ctx, "gpio", "monitoring", "start", string(ti50.GpioTi50ResetL), string(ti50.GpioTi50EcRstL), string(ti50.GpioTi50EcRstFet))
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

	eventData, err := board.OpenTitanToolCommand(ctx, "gpio", "monitoring", "read", string(ti50.GpioTi50ResetL), string(ti50.GpioTi50EcRstL), string(ti50.GpioTi50EcRstFet))
	if err != nil {
		s.Fatal("OpenTitanToolCommand: ", err)
	}

	events := eventData["events"].([]interface{})
	if len(events) != 4 {
		s.Fatal("Unexpected number of events")
	}
	event1 := events[0].(map[string]interface{})
	if event1["signal_name"].(string) != string(ti50.GpioTi50ResetL) || event1["edge"] != "Falling" {
		s.Error("Unexpected first event: ", event1)
	}
	event2 := events[1].(map[string]interface{})
	if event2["signal_name"].(string) != string(ti50.GpioTi50EcRstFet) || event2["edge"] != "Rising" {
		s.Error("Unexpected second event: ", event2)
	}
	event3 := events[2].(map[string]interface{})
	if event3["signal_name"].(string) != string(ti50.GpioTi50ResetL) || event3["edge"] != "Rising" {
		s.Error("Unexpected third event: ", event3)
	}
	event4 := events[3].(map[string]interface{})
	if event4["signal_name"].(string) != string(ti50.GpioTi50EcRstFet) || event4["edge"] != "Falling" {
		s.Error("Unexpected fourth event: ", event4)
	}
}
