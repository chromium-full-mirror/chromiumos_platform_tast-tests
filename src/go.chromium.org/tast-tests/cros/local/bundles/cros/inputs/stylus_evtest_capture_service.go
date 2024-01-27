// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/input"
	pb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

const (
	touchLogFileName = "touch_logs.csv"
	lowBatteryLevel  = 10
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterStylusEvtestCaptureServiceServer(srv, &StylusEvtestCaptureService{s: s})
		},
	})
}

// StylusEvtestCaptureService implements tast.cros.inputs.StylusEvtestCaptureService.
type StylusEvtestCaptureService struct {
	s               *testing.ServiceState
	wg              sync.WaitGroup
	touchLogTempDir string
}

// StartStylusDataCapture captures stylus evtest data and writes it to a file. It waits for a call to StopStylusDataCapture before exiting.
func (svc *StylusEvtestCaptureService) StartStylusDataCapture(ctx context.Context, req *empty.Empty) (*pb.StylusEvtestCaptureResponse, error) {
	svc.wg.Add(1)
	stylusFound, stylusPath, err := input.FindPhysicalStylus(ctx)
	if err != nil {
		return nil, err
	}
	if !stylusFound {
		return nil, errors.New("no stylus could be found")
	}

	svc.touchLogTempDir, err = os.MkdirTemp(os.TempDir(), "stylus_evtest_capture")
	if err != nil {
		return nil, err
	}
	touchLogFilePath := filepath.Join(svc.touchLogTempDir, touchLogFileName)

	touchLogFile, err := os.OpenFile(touchLogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer touchLogFile.Close()

	evtestCmd, awkCmd, err := startCaptureProcesses(ctx, stylusPath, touchLogFile)
	if err != nil {
		return nil, err
	}

	// Waits until StopStylusDataCapture is called.
	svc.wg.Wait()

	err = stopCaptureProcesses(evtestCmd, awkCmd)
	if err != nil {
		return nil, err
	}

	// The battery level is not updated until the stylus physically touches the DUT.
	// Therefore the battery level check has to occur after the motion has been completed.
	stylusHasBattery, batteryLevel, err := input.FindPhysicalStylusBatteryLevel(ctx)
	if err != nil {
		return nil, err
	}
	if stylusHasBattery {
		testing.ContextLogf(ctx, "Current stylus battery: %v%%", batteryLevel)
		// Fail all tests with a battery level below the low battery threshold.
		// 10% was chosen as the low battery threshold, however this was mostly arbitrary.
		if batteryLevel < lowBatteryLevel {
			return nil, errors.Errorf("stylus battery is %v%%, this is below the low battery threshold(%v%%)", batteryLevel, lowBatteryLevel)
		}
	}

	fileInfo, err := os.Stat(touchLogFilePath)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() == 0 {
		return nil, errors.New("no touch events were logged")
	}

	return &pb.StylusEvtestCaptureResponse{StylusLogPath: touchLogFilePath}, nil
}

// startCaptureProcesses starts an evtest and awk subprocess to capture the DUT's touch stylus events, and write them to a file.
func startCaptureProcesses(ctx context.Context, stylusPath string, outputFile *os.File) (*testexec.Cmd, *testexec.Cmd, error) {
	// Awk script is used to convert evtest output to gcode conversion input.
	// GCode Conversion Format is: x, y, pressure, time
	//
	// Sample evtest output:
	// (Note sample output is only showing relevant output from evtest)
	//
	// Event: time 1691552057.683170, type 3 (EV_ABS), code 0 (ABS_X), value 10216
	// Event: time 1691552057.683170, type 3 (EV_ABS), code 1 (ABS_Y), value 5164
	// Event: time 1691552057.683170, type 3 (EV_ABS), code 24 (ABS_PRESSURE), value 3078
	//
	// Event: time 1691552067.683172, type 3 (EV_ABS), code 0 (ABS_X), value 10216
	// Event: time 1691552067.683172, type 3 (EV_ABS), code 1 (ABS_Y), value 2361
	// Event: time 1691552067.683172, type 3 (EV_ABS), code 24 (ABS_PRESSURE), value 1024
	//
	// Awk script output from above sample:
	// 10216, 5164, 3078, 1691552057.683170
	// 10216, 2361, 1024, 1691552067.683172

	awkArgs := `BEGIN{print "x,y,pressure,time"; x=0; y=0; p=0; r=0} /BTN_TOOL_PEN.*1/{r=1} /BTN_TOOL_PEN.*0/{r=0} /ABS_X/{x=$11} /ABS_Y/{y=$11} /ABS_PRESSURE/{p=$11} /SYN_REPORT/{if (r=1) print x "," y "," p "," $3;}`

	evtestCmd := testexec.CommandContext(ctx, "evtest", stylusPath)
	awkCmd := testexec.CommandContext(ctx, "awk", awkArgs)

	evtestOut, err := evtestCmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	awkCmd.Stdin = evtestOut
	awkCmd.Stdout = outputFile

	err = evtestCmd.Start()
	if err != nil {
		return nil, nil, err
	}
	err = awkCmd.Start()
	if err != nil {
		return nil, nil, err
	}

	return evtestCmd, awkCmd, nil
}

// stopCaptureProcesses kills the evtest and awk subprocesses and waits for them to exit.
func stopCaptureProcesses(evtestCmd, awkCmd *testexec.Cmd) error {
	err := awkCmd.Kill()
	if err != nil {
		return err
	}
	err = evtestCmd.Kill()
	if err != nil {
		return err
	}

	awkCmd.Wait()
	evtestCmd.Wait()

	return nil
}

// StopStylusDataCapture informs the StartStylusDataCapture function to stop capturing stylus data and to return.
func (svc *StylusEvtestCaptureService) StopStylusDataCapture(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	svc.wg.Done()
	return &empty.Empty{}, nil
}

// CleanUp removes the stylus touch data file created by StartStylusDataCapture.
func (svc *StylusEvtestCaptureService) CleanUp(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	err := os.RemoveAll(svc.touchLogTempDir)
	if err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}
