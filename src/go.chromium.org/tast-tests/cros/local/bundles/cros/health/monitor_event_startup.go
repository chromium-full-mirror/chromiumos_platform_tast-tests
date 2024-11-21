// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

type eventStartupParams struct {
	category string
	duration time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func: MonitorEventStartup,
		Desc: "Checks if cros_healthd can start up an event monitor for a period of time",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health
		BugComponent: "b:982097",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"diagnostics"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name: "audio_jack",
			Val: eventStartupParams{
				category: "audio_jack",
				duration: 3 * time.Second,
			},
		}, {
			Name: "touchpad",
			Val: eventStartupParams{
				category: "touchpad",
				duration: 3 * time.Second,
			},
		}, {
			Name: "external_display",
			Val: eventStartupParams{
				category: "external_display",
				duration: 3 * time.Second,
			},
		}, {
			Name: "sd_card",
			Val: eventStartupParams{
				category: "sd_card",
				duration: 3 * time.Second,
			},
		}, {
			Name: "touchscreen",
			Val: eventStartupParams{
				category: "touchscreen",
				duration: 3 * time.Second,
			},
		}, {
			Name: "stylus_garage",
			Val: eventStartupParams{
				category: "stylus_garage",
				duration: 3 * time.Second,
			},
		}, {
			Name: "stylus",
			Val: eventStartupParams{
				category: "stylus",
				duration: 3 * time.Second,
			},
		}},
	})
}

func MonitorEventStartup(ctx context.Context, s *testing.State) {
	testParam := s.Param().(eventStartupParams)
	categoryArg := "--category=" + testParam.category
	durationArg := "--length_seconds=" + strconv.Itoa(int(testParam.duration/time.Second))
	monitorCmd := testexec.CommandContext(ctx, "cros-health-tool", "event", categoryArg, durationArg)
	var stderrBuf bytes.Buffer
	monitorCmd.Stderr = &stderrBuf

	start := time.Now()
	if err := monitorCmd.Run(); err != nil {
		s.Fatal("Failed to run healthd monitor command: ", err)
	}

	elapsed := time.Since(start)
	stderr := stderrBuf.String()
	// It's possible that cros_healthd can't find the evdev target. In this case, "can't find target" will be reported.
	// This test focuses on catching the seccomp policy error or other crash, so "can't find target" is fine.
	if !strings.Contains(stderr, "can't find target") && elapsed < testParam.duration {
		s.Fatalf("Failed to monitor for %v seconds: %s", testParam.duration, stderr)
	}
}
