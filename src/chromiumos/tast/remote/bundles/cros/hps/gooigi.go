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

	"chromiumos/tast/common/hps/hpsutil"
	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/bundles/cros/hps/utils"
	"chromiumos/tast/rpc"
	pb "chromiumos/tast/services/cros/hps"
	"chromiumos/tast/ssh/linuxssh"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

type testParamForGoo struct {
	leftOn  bool
	rightOn bool
	backOn  bool
	persons int
	power   int
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
			"alonusem@google.com",
			"eunicesun@google.com",
			"mblsha@google.com",
		},
		BugComponent: "b:1140302",
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.HPS()),
		Data:         []string{"gooigi_mansion.py", "gooigi_dmatrix.py", "gooigi_PWM_PCA9685.py"},
		SoftwareDeps: []string{"hps", "chrome", caps.BuiltinCamera},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.hps.HpsService"},
		Params: []testing.Param{
			{
				Name: "side_50",
				Val: testParamForGoo{
					leftOn:  true,
					rightOn: true,
					backOn:  false,
					persons: 2,
					power:   50,
				},
			},
			{
				Name: "back_50",
				Val: testParamForGoo{
					leftOn:  false,
					rightOn: false,
					backOn:  true,
					persons: 2,
					power:   50,
				},
			},
			{
				Name: "side_100",
				Val: testParamForGoo{
					leftOn:  true,
					rightOn: true,
					backOn:  false,
					persons: 2,
					power:   100,
				},
			},
			{
				Name: "back_100",
				Val: testParamForGoo{
					leftOn:  false,
					rightOn: false,
					backOn:  true,
					persons: 2,
					power:   100,
				},
			},
		},
	})
}

func Gooigi(ctx context.Context, s *testing.State) {
	param := s.Param().(testParamForGoo)
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

	files := [3]string{"gooigi_mansion.py", "gooigi_dmatrix.py", "gooigi_PWM_PCA9685.py"}
	filesMap := map[string]string{}

	for _, file := range files {
		filesMap[s.DataPath(file)] = "/tmp/" + file
	}

	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), filesMap, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to copy files: ", err)
	}

	args := make([]string, 1)
	args[0] = "/tmp/" + files[0]

	if param.backOn {
		args = append(args, "-b")
	}
	if param.leftOn {
		args = append(args, "-l")
	}
	if param.rightOn {
		args = append(args, "-r")
	}

	args = append(args, fmt.Sprintf("-d %d", duration+5))
	args = append(args, fmt.Sprintf("-p %d", param.power))

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

	result, err := utils.RetrieveHpsSenseSignal(ctx, client)
	testing.ContextLog(ctx, "sense: ", result)

	status.sense = result

	result, err = utils.RetrieveHpsNotifySignal(ctx, client)
	testing.ContextLog(ctx, "notify: ", result)

	status.notify = result

	if status.sense != true && param.persons > 0 {
		s.Fatal("Failed to detect person")
	}

	if status.notify != false && param.persons < 2 {
		s.Fatal("Incorrectly identified extra people")
	}

	if status.notify != true && param.persons >= 2 {
		s.Fatal("Failed to detect both people")
	}
}
