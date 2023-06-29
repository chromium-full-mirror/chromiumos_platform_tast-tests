// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/hps/hpsutil"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hps/utils"
	pb "go.chromium.org/tast-tests/cros/services/cros/hps"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParamForGoo struct {
	leftOn    bool // Enable/disable the lighting panel to the left of the subject.
	rightOn   bool // Enable/disable the lighting panel to the right of the subject.
	backOn    bool // Enable/disable the lighting panel to the rear of the subject.
	main      bool // Show/hide the main subject.
	secondary bool // Show/hide the secondary subject
	power     int  // Percentage strength of the light sources surrounding the subject.
}

type hpsResult struct {
	sense  bool
	notify bool
}

const duration = 20

func init() {
	testing.AddTest(&testing.Test{
		Func:         Gooigi,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the hps detects single person in backlit environments",
		Contacts: []string{
			"chromeos-hps-swe@google.com",
			"eunicesun@google.com",
			"mblsha@google.com",
		},
		BugComponent: "b:1140302",
		Timeout:      45 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.HPS()),
		Data:         []string{"gooigi_mansion.py", "gooigi_dmatrix.py", "gooigi_PWM_PCA9685.py"},
		SoftwareDeps: []string{"hps", "chrome", caps.BuiltinCamera},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.hps.HpsService"},
	})
}

func Gooigi(ctx context.Context, s *testing.State) {
	dut := s.DUT()

	// Creating hps context.
	hctx, err := hpsutil.NewHpsContext(ctx, "", hpsutil.DeviceTypeBuiltin, s.OutDir(), dut.Conn())
	if err != nil {
		s.Fatal("Error creating HpsContext: ", err)
	}

	// Connecting to Taeko.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to setup grpc: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Enable LoL in settings.
	client := pb.NewHpsServiceClient(cl.Conn)
	req := &pb.StartUIWithCustomScreenPrivacySettingRequest{
		Setting: utils.LockOnLeave,
		Enable:  true,
	}
	if _, err := client.StartUIWithCustomScreenPrivacySetting(hctx.Ctx, req, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to change setting: ", err)
	}

	// Enable SPA in settings.
	req = &pb.StartUIWithCustomScreenPrivacySettingRequest{
		Setting: utils.SecondPersonAlert,
		Enable:  true,
	}
	if _, err := client.StartUIWithCustomScreenPrivacySetting(hctx.Ctx, req, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to change setting: ", err)
	}

	// Render hps-internal page for debugging.
	if _, err := client.OpenHPSInternalsPage(hctx.Ctx, &empty.Empty{}); err != nil {
		s.Fatal("Error open hps-internals")
	}

	status := hpsResult{}
	// Wait for hpsd to finish starting the HPS peripheral and enabling the feature we requested.
	waitReq := &pb.WaitForHpsRequest{
		WaitForSense:  true,
		WaitForNotify: true,
	}

	if _, err := client.WaitForHps(ctx, waitReq); err != nil {
		s.Fatal("Failed to wait for HPS to be ready: ", err)
	}

	// Copy enclosure control files to the DUT.
	files := [3]string{"gooigi_mansion.py", "gooigi_dmatrix.py", "gooigi_PWM_PCA9685.py"}
	filesMap := map[string]string{}

	for _, file := range files {
		filesMap[s.DataPath(file)] = "/tmp/" + file
	}

	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), filesMap, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to copy files: ", err)
	}

	// Create slice of possible combinations of parameters for sub tests.
	parameters := make([]testParamForGoo, 0)

	for _, secondary := range []bool{true, false} {
		for _, back := range []bool{true, false} {
			for _, right := range []bool{true, false} {
				for _, left := range []bool{true, false} {
					for power := 20; power <= 100; power = power + 20 {
						parameters = append(parameters, testParamForGoo{
							leftOn:    left,
							rightOn:   right,
							backOn:    back,
							main:      true,
							secondary: secondary,
							power:     power,
						})
					}
				}
			}
		}
	}

	// Run every combination of testing environment.
	for _, tc := range parameters {
		s.Run(ctx, fmt.Sprintf("Left:%t Right:%t Back:%t Secondary:%t Power:%d", tc.leftOn, tc.rightOn, tc.backOn, tc.secondary, tc.power), func(ctx context.Context, s *testing.State) {

			// Define arguments for python script.
			args := make([]string, 1)
			args[0] = "/tmp/" + files[0]

			// Add arguments to enable the different lighting panels.
			if tc.backOn {
				args = append(args, "-b")
			}
			if tc.leftOn {
				args = append(args, "-l")
			}
			if tc.rightOn {
				args = append(args, "-r")
			}

			// Add arguments for which subjects should be visible to HPS.
			if tc.main {
				args = append(args, "-m")
			}
			if tc.secondary {
				args = append(args, "-s")
			}

			// Add arguments for the duration of the test and intensity of light panels.
			args = append(args, fmt.Sprintf("-d %d", duration))
			args = append(args, fmt.Sprintf("-p %d", tc.power))

			// Define, configure, and execute python script to control enclosure.
			pyCom := dut.Conn().CommandContext(ctx, "python", args...)

			logFilename := "gooigi_logs.txt"
			logFile, err := os.OpenFile(filepath.Join(s.OutDir(), logFilename),
				os.O_WRONLY|os.O_CREATE|os.O_APPEND,
				0644)
			if err != nil {
				s.Fatal(err, "cannot open logfile %s for the ml_benchmark to write to", logFilename)
			}

			pyCom.Stderr = logFile
			pyCom.Stdout = logFile

			testing.ContextLog(ctx, "Launching lighting control script with parameters: ", args)

			if err := pyCom.Start(); err != nil {
				s.Fatal("Failed to start python script: ", err)
			}

			defer func() {
				pyCom.Abort()
				pyCom.Wait()
			}()

			// Allow time for hardware to configure and start as well as HPS auto exposure to react to light conditions.
			if err := testing.Sleep(ctx, duration*time.Second); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}

			// Record results from HPS unit.
			result, err := utils.RetrieveHpsSenseSignal(ctx, client)
			testing.ContextLog(ctx, "sense: ", result)

			status.sense = result

			result, err = utils.RetrieveHpsNotifySignal(ctx, client)
			testing.ContextLog(ctx, "notify: ", result)

			status.notify = result

			if err := testing.Sleep(ctx, 5*time.Second); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}

			// Compare sense and notify signals with enclosure configuration.
			if status.sense != true && (tc.main || tc.secondary) {
				s.Fatal("Failed to detect person")
			}

			if status.notify != false && (tc.main || tc.secondary) && !(tc.main && tc.secondary) {
				s.Fatal("Incorrectly identified extra people")
			}

			if status.notify != true && (tc.main && tc.secondary) {
				s.Fatal("Failed to detect both people")
			}

		})
	}

}
