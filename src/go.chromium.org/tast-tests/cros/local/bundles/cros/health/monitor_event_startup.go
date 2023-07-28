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
			// TODO(b/279849605): Promote to critical.
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "sd_card",
			Val: eventStartupParams{
				category: "sd_card",
				duration: 3 * time.Second,
			},
			// TODO(b/279849703): Promote to critical.
			ExtraAttr: []string{"informational", "group:criticalstaging"},
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
	stderr := string(stderrBuf.Bytes())
	// It's possible that cros_healthd can't find the evdev target. In this case, "EvdevUtil can't find target" will be reported.
	// This test focuses on catching the seccomp policy error or other crash, so "EvdevUtil can't find target" is fine.
	if !strings.Contains(stderr, "EvdevUtil can't find target") && elapsed < testParam.duration {
		s.Fatalf("Failed to monitor for %v seconds: %s", testParam.duration, stderr)
	}
}
