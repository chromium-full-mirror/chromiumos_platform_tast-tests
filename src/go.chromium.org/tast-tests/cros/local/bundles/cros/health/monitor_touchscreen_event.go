// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type touchscreenConnectedEvent struct {
	MaxX        jsontypes.Uint32 `json:"max_x"`
	MaxY        jsontypes.Uint32 `json:"max_y"`
	MaxPressure jsontypes.Uint32 `json:"max_pressure"`
}

type touchscreenTouchPointInfo struct {
	TrackingID jsontypes.Uint32  `json:"tracking_id"`
	X          jsontypes.Uint32  `json:"x"`
	Y          jsontypes.Uint32  `json:"y"`
	Pressure   *jsontypes.Uint32 `json:"pressure"`
	TouchMajor *jsontypes.Uint32 `json:"touch_major"`
	TouchMinor *jsontypes.Uint32 `json:"touch_minor"`
}

type touchscreenTouchPointEvent struct {
	TouchPoints []touchscreenTouchPointInfo `json:"touch_points"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MonitorTouchscreenEvent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Monitors the touchscreen event detected properly or not",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"diagnostics"},
		HardwareDeps: hwdep.D(hwdep.TouchScreen()),
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			// Stable models.
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("bard")),
		}, {
			// TODO(b/300852245): Promote to critical when the issue on nami(bard) is fixed.
			Name:              "unstable",
			ExtraHardwareDeps: hwdep.D(hwdep.Model("bard")),
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func triggerTouchscreenTouchpointEvent(ctx context.Context) error {
	// Reserve time for cleanup.
	ctxForCleanUpTouchscreen := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	touchscreen, err := input.Touchscreen(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a touchscreen device")
	}
	defer touchscreen.Close(ctxForCleanUpTouchscreen)

	stw, err := touchscreen.NewSingleTouchWriter()
	if err != nil {
		return errors.Wrap(err, "failed to create a single touch writer")
	}
	defer stw.Close()

	stw.Move(touchscreen.Width()/2, touchscreen.Height()/2)
	stw.End()

	return nil
}

func verifyTouchscreenConnectedEvent(eventLine string) error {
	prefix := "Touchscreen connected event received:"
	eventJSON, eventFound := strings.CutPrefix(eventLine, prefix)
	if !eventFound {
		return errors.Errorf("failed to report device attributes: %s", eventLine)
	}

	decoder := json.NewDecoder(strings.NewReader(eventJSON))
	decoder.DisallowUnknownFields()
	var event touchscreenConnectedEvent
	if err := decoder.Decode(&event); err != nil {
		return errors.Wrapf(err, "unable to parse connected event %s", eventJSON)
	}

	if event.MaxX == 0 {
		return errors.New("the max_x for connected event should be non-zero")
	}
	if event.MaxY == 0 {
		return errors.New("the max_y for connected event should be non-zero")
	}
	// TODO(b/297848185): Verify the maximum pressure on non-reven boards.

	return nil
}

func verifyTouchscreenTouchpointEvent(eventLine string) error {
	prefix := "Touchscreen touch event received:"
	eventJSON, eventFound := strings.CutPrefix(eventLine, prefix)
	if !eventFound {
		return errors.Errorf("failed to detect touch point: %s", eventLine)
	}

	decoder := json.NewDecoder(strings.NewReader(eventJSON))
	decoder.DisallowUnknownFields()
	var event touchscreenTouchPointEvent
	if err := decoder.Decode(&event); err != nil {
		return errors.Wrapf(err, "unable to parse touch point event %s", eventJSON)
	}

	if numberOfTouchPoint := len(event.TouchPoints); numberOfTouchPoint != 1 {
		return errors.Errorf("unexpected number of touch point: got %d; want 1", numberOfTouchPoint)
	}

	return nil
}

func MonitorTouchscreenEvent(ctx context.Context, s *testing.State) {
	// Run monitor command in background.
	monitorCmd := testexec.CommandContext(ctx, "cros-health-tool", "event", "--category=touchscreen", "--length_seconds=10")
	stdoutPipe, err := monitorCmd.StdoutPipe()
	if err != nil {
		s.Fatal("Failed to create stdout pipe: ", err)
	}
	if err := monitorCmd.Start(); err != nil {
		s.Fatal("Failed to run healthd monitor command: ", err)
	}
	defer monitorCmd.Wait(testexec.DumpLogOnError)

	scanner := bufio.NewScanner(stdoutPipe)

	if !scanner.Scan() {
		s.Fatal("Failed to scan next line for success message: ", scanner.Err())
	} else if line := scanner.Text(); !strings.HasPrefix(line, "Subscribe to touchscreen events successfully") {
		s.Fatal("Failed to subscribe event in healthd:", line)
	}

	// The connected event must be the first event.
	if !scanner.Scan() {
		s.Fatal("Failed to scan next line for connected event: ", scanner.Err())
	} else if err := verifyTouchscreenConnectedEvent(scanner.Text()); err != nil {
		s.Fatal("Connected event verification failed: ", err)
	}

	// We must trigger the touchpoint event only after the connected event is
	// received (i.e., healthd bind to the underlying evdev node successfully).
	// Otherwise, we won't see the touchpoint event from cros-health-tool.
	if err := triggerTouchscreenTouchpointEvent(ctx); err != nil {
		s.Fatal("Failed to trigger touchscreen event: ", err)
	}

	if !scanner.Scan() {
		s.Fatal("Failed to scan next line for touchpoint event: ", scanner.Err())
	} else if err := verifyTouchscreenTouchpointEvent(scanner.Text()); err != nil {
		s.Fatal("Touchpoint event verification failed: ", err)
	}

	if err := monitorCmd.Kill(); err != nil {
		s.Fatal("Failed to kill the event monitoring process: ", err)
	}
}
