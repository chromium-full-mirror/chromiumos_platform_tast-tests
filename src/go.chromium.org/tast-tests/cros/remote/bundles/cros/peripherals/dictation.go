// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package peripherals

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	dictationcommon "go.chromium.org/tast-tests/cros/common/dictation"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/robotics/arm/amber"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/peripherals/dictation"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/peripherals/utils"
	"go.chromium.org/tast-tests/cros/services/cros/peripherals"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Dictation,
		Desc: "Setup of dictation device, set/get event modes, test button/motion events and LED status",
		Contacts: []string{
			"cros-ent-peripherals-team@google.com",
			"rzakarian@google.com",
			"sudhirperka@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:885494", // ChromeOS > Platform > Enablement > Services > Peripherals
		Timeout:      12 * time.Minute,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.peripherals.PeriphService",
			"tast.cros.ui.ScreenRecorderService",
			utils.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"peripherals.username",
			"peripherals.password",
			"peripherals.record_screen",
			"peripherals.manual_test",
		},
		Fixture: fixture.CleanOwnership,
		Params: []testing.Param{
			{
				Name:      "philips",
				Val:       dictation.PhilipsTestParams,
				ExtraData: []string{dictation.PhilipsMotionData},
			},
		},
	})
}

func Dictation(ctx context.Context, s *testing.State) {
	testParams := s.Param().(dictation.TestParams)
	username := s.RequiredVar("peripherals.username")
	password := s.RequiredVar("peripherals.password")
	// Manually enroll into target organization.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect the DUT before manual enrollment: ", err)
	}
	defer cl.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	policyClient := ps.NewPolicyServiceClient(cl.Conn)
	if _, err := policyClient.GAIAEnrollAndLoginUsingChrome(ctx, &ps.GAIAEnrollAndLoginUsingChromeRequest{
		Username:    username,
		Password:    password,
		DmserverURL: policy.DMServerProdURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(cleanupCtx, &empty.Empty{})

	recordScreen := false
	if val, ok := s.Var("peripherals.record_screen"); ok {
		if recordScreen, err = strconv.ParseBool(val); err != nil {
			s.Fatal("Failed to parse argument 'peripherals.record_screen' of type bool: ", err)
		}
	}
	if recordScreen {
		s.Log("Screen recorder started")

		filePath := filepath.Join(s.OutDir(), s.TestName()+".webm")
		startRequest := pb.StartRequest{
			FileName: filePath,
		}
		screenRecorder := pb.NewScreenRecorderServiceClient(cl.Conn)
		if _, err := screenRecorder.Start(ctx, &startRequest); err != nil {
			s.Fatal("Failed to start recording: ", err)
		}
		defer func(ctx context.Context) {
			res, err := screenRecorder.Stop(ctx, &empty.Empty{})
			if err != nil {
				s.Log("Failed to stop recording: ", err)
			} else {
				s.Logf("Screen recording saved to %s", res.FileName)

				s.Log("Copying screen recording from DUT to local machine")
				destPath := filepath.Join(s.OutDir(), filepath.Base(res.FileName))
				if err := linuxssh.GetFile(ctx, s.DUT().Conn(), res.FileName, destPath, linuxssh.DereferenceSymlinks); err != nil {
					s.Log("Failed to copy screen recording to local machine: ", err)
				}
			}
		}(cleanupCtx)
	}

	manualTest := false
	if val, ok := s.Var("peripherals.manual_test"); ok {
		manualTest, err = strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Failed to parse argument 'peripherals.manual_test' of type bool: ", err)
		}
	}

	if err := performDictationOperations(ctx, cl, s.DataPath, testParams, manualTest); err != nil {
		s.Fatal("Failed to perform dictation operations: ", err)
	}

}

func performDictationOperations(ctx context.Context, cl *rpc.Client, dataPath func(string) string, params dictation.TestParams, manualTest bool) (retErr error) {
	const waitTimeout = 30 * time.Second
	deviceName := params.DeviceName

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	svc := peripherals.NewPeriphServiceClient(cl.Conn)
	if _, err := svc.NewDictationSupport(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to create new dictation support")
	}
	defer svc.CloseDictation(ctx, &empty.Empty{})
	defer utils.DumpUITreeWithScreenshotToFile(cleanupCtx, cl.Conn, func() bool { return retErr != nil }, "ui_dump")

	if _, err := svc.ConnectToDictationDevice(ctx, &peripherals.ConnectToDictationDeviceRequest{
		DeviceName: deviceName,
	}); err != nil {
		return errors.Wrapf(err, "failed to connect to device %v", deviceName)
	}
	eventModeHid := string(dictationcommon.EventModeHid)
	if _, err := svc.SetDictationEventMode(ctx, &peripherals.SetDictationEventModeRequest{
		EventMode: eventModeHid,
	}); err != nil {
		return errors.Wrapf(err, "failed to set event mode %v", eventModeHid)
	}

	res, err := svc.GetDictationEventMode(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get event mode")
	}
	eventMode := res.EventMode
	if !strings.EqualFold(eventMode, eventModeHid) {
		return errors.Errorf("unexpected event mode, got: %s, want: %s", eventMode, eventModeHid)
	}
	testing.ContextLog(ctx, "Event mode is as expected: ", eventMode)

	getDevicesRes, err := svc.GetDictationDevices(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get devices")
	}

	testing.ContextLog(ctx, "Devices: ", getDevicesRes.Devices)

	var (
		arm       *amber.Arm
		positions [][]float32
	)

	if !manualTest {
		cleanupArmCtx := ctx
		ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		arm, err = amber.NewArm(amber.DefaultReadWriteTimeout)
		if err != nil {
			return errors.Wrap(err, "failed to create arm helper")
		}
		defer arm.Close()
		defer arm.MoveToInitialPosition(cleanupArmCtx)

		positions, err = amber.ParsePositionsFromCSV(dataPath(params.MotionData))
		if err != nil {
			return errors.Wrap(err, "failed to read dictation motion data")
		}
		motionCount := len(params.MotionDataMap)
		if len(positions) != motionCount {
			return errors.Wrapf(err, "unexpected motion count: got %d, expected %d", len(positions), motionCount)
		}

		// Move to ready position.
		if err := arm.SingleMove(ctx, positions[dictation.ReadyPositionIndex], dictation.MotionDuration); err != nil {
			return errors.Wrap(err, "failed to move to ready position")
		}
	}

	if err := buttonEventsTesting(ctx, svc, params, arm, positions, waitTimeout, manualTest); err != nil {
		return errors.Wrap(err, "failed to test button events")
	}

	if err := setSimpleLEDStateTesting(ctx, svc); err != nil {
		return errors.Wrap(err, "failed to test simple LED state")
	}

	if err := setLEDStateTesting(ctx, svc); err != nil {
		return errors.Wrap(err, "failed to test LED state")
	}

	if err := motionEventsTesting(ctx, svc, waitTimeout, manualTest); err != nil {
		return errors.Wrap(err, "failed to test motion events")
	}

	return nil
}

func buttonEventsTesting(ctx context.Context, svc peripherals.PeriphServiceClient, params dictation.TestParams,
	arm *amber.Arm, positions [][]float32, waitTimeout time.Duration, manualTest bool) error {
	pressButtonAndVerify := func(ctx context.Context, button string) error {
		if manualTest {
			testing.ContextLogf(ctx, "Please press %q button in %v", button, waitTimeout)
		} else {
			testing.ContextLogf(ctx, "Start to move robotic arm to press %q button", button)
			motionDuration := dictation.MotionDuration
			index := params.MotionDataMap[button]
			if err := arm.SingleMove(ctx, positions[index], motionDuration); err != nil {
				return errors.Wrapf(err, "failed to move robotic arm to press %q button", button)
			}
			defer func() {
				// Return to ready position for the next verification.
				if err := arm.SingleMove(ctx, positions[dictation.ReadyPositionIndex], motionDuration); err != nil {
					testing.ContextLog(ctx, "Failed to move to ready position after verification: ", err)
				}
			}()
		}
		if _, err := svc.WaitDictationNewEvent(ctx, &peripherals.WaitDictationNewEventRequest{
			Event:   button,
			Timeout: waitTimeout.Milliseconds(),
		}); err != nil {
			return errors.Wrapf(err, "failed to wait for the new event %q", button)
		}
		return nil
	}
	for _, button := range params.ButtonTestList {
		if err := pressButtonAndVerify(ctx, button); err != nil {
			return errors.Wrapf(err, "failed to press button %q", button)
		}
	}
	return nil
}

func setSimpleLEDStateTesting(ctx context.Context, svc peripherals.PeriphServiceClient) error {
	simpleLEDStateList := []dictationcommon.SimpleLEDState{
		dictationcommon.SimpleLEDStateRecordInsert,
		dictationcommon.SimpleLEDStateRecordOverwrite,
		dictationcommon.SimpleLEDStateStandbyInsert,
		dictationcommon.SimpleLEDStateStandbyOverwrite,
		dictationcommon.SimpleLEDStateOff,
	}
	for _, state := range simpleLEDStateList {
		if _, err := svc.SetDictationSimpleLEDState(ctx, &peripherals.SetDictationSimpleLEDStateRequest{
			SimpleLedState: string(state),
		}); err != nil {
			return errors.Wrapf(err, "failed to set simple LED state to %q", state)
		}

		// TODO: Verify the LED state to the expected state.
	}
	return nil
}

func setLEDStateTesting(ctx context.Context, svc peripherals.PeriphServiceClient) error {
	ledIndexOrderedList := []dictationcommon.LEDIndex{
		dictationcommon.LEDInstrctionGreen,
		dictationcommon.LEDInstrctionRed,
		dictationcommon.LEDInsOwrButtonGreen,
		dictationcommon.LEDInsOwrButtonRed,
		dictationcommon.LEDF1Button,
		dictationcommon.LEDF2Button,
		dictationcommon.LEDF3Button,
		dictationcommon.LEDF4Button,
	}
	ledModeOrderedList := []dictationcommon.LEDMode{
		dictationcommon.LEDModeOn,
		dictationcommon.LEDModeBlinkFast,
		dictationcommon.LEDModeBlinkSlow,
		dictationcommon.LEDModeOff,
	}
	for _, index := range ledIndexOrderedList {
		for _, mode := range ledModeOrderedList {
			if _, err := svc.SetDictationLEDState(ctx, &peripherals.SetDictationLEDStateRequest{
				LedIndex: string(index),
				LedMode:  string(mode),
			}); err != nil {
				return errors.Wrapf(err, "failed to set LED state to index %v, mode %v", index, mode)
			}

			// TODO: Verify the LED status to the expected status.
		}
	}
	return nil
}

func motionEventsTesting(ctx context.Context, svc peripherals.PeriphServiceClient, waitTimeout time.Duration, manualTest bool) error {
	// Motion events are only tested manually now.
	// TODO: Do the motion events by robotic arm.
	if !manualTest {
		return nil
	}
	motionEventList := []string{
		dictationcommon.MotionEventPickedUp,
		dictationcommon.MotionEventLaidDown,
	}

	for _, event := range motionEventList {
		testing.ContextLogf(ctx, "Please do the motion event %q in %v", event, waitTimeout)
		if _, err := svc.WaitDictationNewEvent(ctx, &peripherals.WaitDictationNewEventRequest{
			Event:   event,
			Timeout: waitTimeout.Milliseconds(),
		}); err != nil {
			return errors.Wrapf(err, "failed to wait for the new event %q", event)
		}
	}
	return nil
}
