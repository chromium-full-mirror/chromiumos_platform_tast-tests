// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type touchpadConnectedEvent struct {
	MaxX        jsontypes.Uint32 `json:"max_x"`
	MaxY        jsontypes.Uint32 `json:"max_y"`
	MaxPressure jsontypes.Uint32 `json:"max_pressure"`
	Buttons     []string         `json:"buttons"`
}

type touchpadTouchPointInfo struct {
	TrackingID jsontypes.Uint32  `json:"tracking_id"`
	X          jsontypes.Uint32  `json:"x"`
	Y          jsontypes.Uint32  `json:"y"`
	Pressure   *jsontypes.Uint32 `json:"pressure"`
	TouchMajor *jsontypes.Uint32 `json:"touch_major"`
	TouchMinor *jsontypes.Uint32 `json:"touch_minor"`
}

type touchpadTouchPointEvent struct {
	TouchPoints []touchpadTouchPointInfo `json:"touch_points"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MonitorTouchpadEvent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Monitors the touchpad event detected properly or not",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		HardwareDeps: hwdep.D(hwdep.Touchpad()),
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			// TODO(b/298304858): Promote to critical.
			// TODO(b/299568777): Enforce laptop mode to run th test on non-calmshell devices.
			Name:              "clamshell",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Clamshell)),
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
		}},
	})
}

func triggerTouchpointEvent(ctx context.Context) error {
	touchpad, err := input.Trackpad(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a trackpad device")
	}
	defer touchpad.Close(ctx)

	stw, err := touchpad.NewSingleTouchWriter()
	if err != nil {
		return errors.Wrap(err, "failed to create a single touch writer")
	}
	defer stw.Close()

	stw.Move(touchpad.Width()/2, touchpad.Height()/2)
	stw.End()

	return nil
}

func verifyConnectedEvent(eventLine string) error {
	prefix := "Touchpad connected event received:"
	eventJSON, eventFound := strings.CutPrefix(eventLine, prefix)
	if !eventFound {
		return errors.Errorf("failed to report device attributes: %s", eventLine)
	}

	decoder := json.NewDecoder(strings.NewReader(eventJSON))
	decoder.DisallowUnknownFields()
	var event touchpadConnectedEvent
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

func verifyTouchpointEvent(eventLine string) error {
	prefix := "Touchpad touch event received:"
	eventJSON, eventFound := strings.CutPrefix(eventLine, prefix)
	if !eventFound {
		return errors.Errorf("failed to detect touch point: %s", eventLine)
	}

	decoder := json.NewDecoder(strings.NewReader(eventJSON))
	decoder.DisallowUnknownFields()
	var event touchpadTouchPointEvent
	if err := decoder.Decode(&event); err != nil {
		return errors.Wrapf(err, "unable to parse touch point event %s", eventJSON)
	}

	if numberOfTouchPoint := len(event.TouchPoints); numberOfTouchPoint != 1 {
		return errors.Errorf("unexpected number of touch point: got %d; want 1", numberOfTouchPoint)
	}

	return nil
}

func MonitorTouchpadEvent(ctx context.Context, s *testing.State) {
	// Log tablet mode information for debugging b/296182230.
	if tabletModeAngle, err := testexec.CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle").Output(testexec.DumpLogOnError); err != nil {
		s.Log("Failed to read tablet_mode_angle: ", err)
	} else {
		s.Log("Tablet mode angle: ", string(tabletModeAngle))
	}
	if lidAngle, err := testexec.CommandContext(ctx, "ectool", "motionsense", "lid_angle").Output(testexec.DumpLogOnError); err != nil {
		s.Log("Failed to read lid_angle: ", err)
	} else {
		s.Log("Lid angle: ", string(lidAngle))
	}

	// Run monitor command in background.
	monitorCmd := testexec.CommandContext(ctx, "cros-health-tool", "event", "--category=touchpad", "--length_seconds=10")
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
	} else if line := scanner.Text(); !strings.HasPrefix(line, "Subscribe to touchpad events successfully") {
		s.Fatal("Failed to subscirbe event in healthd:", line)
	}

	// The connected event must be the first event.
	if !scanner.Scan() {
		s.Fatal("Failed to scan next line for connected event: ", scanner.Err())
	} else if err := verifyConnectedEvent(scanner.Text()); err != nil {
		s.Fatal("Connected event verification failed: ", err)
	}

	// We must trigger the touchpoint event only after the connected event is
	// received (i.e., healthd bind to the underlying evdev node successfully).
	// Otherwise, we won't see the touchpoint event from cros-health-tool.
	if err := triggerTouchpointEvent(ctx); err != nil {
		s.Fatal("Failed to trigger touchpad event: ", err)
	}

	if !scanner.Scan() {
		s.Fatal("Failed to scan next line for touchpoint event: ", scanner.Err())
	} else if err := verifyTouchpointEvent(scanner.Text()); err != nil {
		s.Fatal("Touchpoint event verification failed: ", err)
	}

	if err := monitorCmd.Kill(); err != nil {
		s.Fatal("Failed to kill the event monitoring process: ", err)
	}
}
