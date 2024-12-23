// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package peripherals

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/robotics/arm/amber"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/peripherals/citrix"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/peripherals/utils"
	"go.chromium.org/tast-tests/cros/services/cros/peripherals"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// footPedalMotionData is the motion data that controls the robotic arm
// to operate the foot pedal.
// The file should contain 5 positions in series:
// Center button, Left button, Right button, Top button, Ready position.
const footPedalMotionData = "citrix/foot_pedal_motion_data.csv"

var footPedalData = append(citrix.CitrixData, citrix.FootPedalData...)

func init() {
	testing.AddTest(&testing.Test{
		Func: FootPedal,
		Desc: "Verify foot pedal app can detect the peripheral input signal from Citrix remote desktop",
		Contacts: []string{
			"cros-ent-peripherals-team@google.com",
			"rzakarian@google.com",
			"sudhirperka@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:885494", // ChromeOS > Platform > Enablement > Services > Peripherals
		Timeout:      10 * time.Minute,
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
		Data:    append(footPedalData, footPedalMotionData),
		Fixture: fixture.CleanOwnership,
	})
}

func FootPedal(ctx context.Context, s *testing.State) {
	const (
		appName = citrix.FootPedalAppName
		// The app title is "Philips SpeechControl" and UI detection may not detect it.
		// So use "SpeechControl" instead.
		appTitle = "SpeechControl"

		motionDuration = 5 * time.Second
		// The file should contain 5 positions in series:
		// Center button, Left button, Right button, Top button, Ready position.
		motionCount        = 5
		readyPositionIndex = motionCount - 1
	)
	username := s.RequiredVar("peripherals.username")
	password := s.RequiredVar("peripherals.password")
	// Prepare data path on DUT.
	d := s.DUT()
	dataPath, err := utils.CopyFilesToRemote(ctx, s, d, footPedalData)
	if err != nil {
		s.Fatal("Failed to put files to remote: ", err)
	}
	defer d.Conn().CommandContext(ctx, "rm", "-r", dataPath).Output()

	// Manually enroll into target organization.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect the DUT before manual enrollment: ", err)
	}
	defer cl.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
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

	svc := peripherals.NewPeriphServiceClient(cl.Conn)
	if _, err := svc.NewCitrix(ctx, &peripherals.NewCitrixRequest{
		DataPath: dataPath,
	}); err != nil {
		s.Fatal("Failed to create new Citrix: ", err)
	}

	defer func(ctx context.Context) {
		utils.DumpUITreeWithScreenshotToFile(ctx, cl.Conn, s.HasError, "ui_dump")
		if _, err := svc.CloseCitrix(ctx, &empty.Empty{}); err != nil {
			s.Log("Failed to close Citrix app: ", err)
		}
	}(cleanupCtx)

	if _, err := svc.LoginCitrix(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to login Citrix: ", err)
	}

	if _, err := svc.OpenCitrixApp(ctx, &peripherals.OpenCitrixAppRequest{
		AppName:  string(appName),
		AppTitle: appTitle,
	}); err != nil {
		s.Fatalf("Failed to open Citrix app %v: %v", appName, err)
	}

	if _, err := svc.SetupFootPedalTest(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to set up foot pedal test: ", err)
	}
	manualTest := false
	if val, ok := s.Var("peripherals.manual_test"); ok {
		manualTest, err = strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Failed to parse argument 'peripherals.manual_test' of type bool: ", err)
		}
	}
	var (
		arm       *amber.Arm
		positions [][]float32
	)
	if !manualTest {
		cleanupArmCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		arm, err = amber.NewArm(amber.DefaultReadWriteTimeout)
		if err != nil {
			s.Fatal("Failed to create arm helper: ", err)
		}
		defer arm.Close()
		defer arm.MoveToInitialPosition(cleanupArmCtx)

		positions, err = amber.ParsePositionsFromCSV(s.DataPath(footPedalMotionData))
		if err != nil {
			s.Fatal("Failed to read foot pedal motion data: ", err)
		}
		if len(positions) != motionCount {
			s.Fatalf("Unexpected motion count: got %d, expected %d", len(positions), motionCount)
		}

		// Move to ready position for the verifications.
		if err := arm.SingleMove(ctx, positions[readyPositionIndex], motionDuration); err != nil {
			s.Fatal("Failed to move to ready position before verification: ", err)
		}
	}

	pressButtonAndVerify := func(ctx context.Context, button peripherals.FootPedalButton) {
		if manualTest {
			s.Logf("Please press the %v button manually", button)
		} else {
			s.Logf("Start to move robotic arm to press %v button", button)
			if err := arm.SingleMove(ctx, positions[button], motionDuration); err != nil {
				s.Fatalf("Failed to move robotic arm to press %v button: %v", button, err)
			}
			defer func() {
				// Return to ready position for the next verification.
				if err := arm.SingleMove(ctx, positions[readyPositionIndex], motionDuration); err != nil {
					s.Fatal("Failed to move to ready position after verification: ", err)
				}
			}()
		}

		if _, err := svc.VerifyFootPedalButtonPressed(ctx, &peripherals.VerifyFootPedalButtonPressedRequest{
			Button: button,
		}); err != nil {
			s.Fatalf("Failed to verify %v button pressed: %v", button, err)
		}
	}

	for _, button := range []peripherals.FootPedalButton{
		peripherals.FootPedalButton_CENTER,
		peripherals.FootPedalButton_LEFT,
		peripherals.FootPedalButton_RIGHT,
		peripherals.FootPedalButton_TOP,
	} {
		pressButtonAndVerify(ctx, button)
	}
}
