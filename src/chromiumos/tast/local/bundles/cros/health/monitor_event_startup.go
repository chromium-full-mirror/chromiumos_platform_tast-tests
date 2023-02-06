// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"strconv"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/testing"
)

type eventStartupParams struct {
	category string
	duration time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MonitorEventStartup,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if cros_healthd can start up an event monitor for a period of time",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"kerker@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name: "audio_jack",
			Val: eventStartupParams{
				category: "audio_jack",
				duration: 3 * time.Second,
			},
			ExtraAttr: []string{"informational"},
		}},
	})
}

func MonitorEventStartup(ctx context.Context, s *testing.State) {
	testParam := s.Param().(eventStartupParams)
	categoryArg := "--category=" + testParam.category
	durationArg := "--length_seconds=" + strconv.Itoa(int(testParam.duration/time.Second))
	monitorCmd := testexec.CommandContext(ctx, "cros-health-tool", "event", categoryArg, durationArg)

	start := time.Now()
	if err := monitorCmd.Run(); err != nil {
		s.Fatal("Failed to run healthd monitor command: ", err)
	}

	elapsed := time.Since(start)
	if elapsed < testParam.duration {
		s.Fatalf("Failed to monitor for %v seconds", testParam.duration)
	}
}
