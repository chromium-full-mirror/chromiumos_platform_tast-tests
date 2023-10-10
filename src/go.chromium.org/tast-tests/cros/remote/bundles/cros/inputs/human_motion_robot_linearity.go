// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/xmlrpc"
	inputspb "go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	baseFileName     = "linearity"
	gcodeFileName    = baseFileName + ".nc"
	touchLogFileName = baseFileName + ".csv"

	xIdx        = 0
	yIdx        = 1
	pressureIdx = 2
	timeIdx     = 3

	maxNumParts = 5
)

var (
	hmrTouchhostHostname = testing.RegisterVarString(
		"inputs.hmr_touchhost_hostname",
		"",
		"Hostname for HMR Touchhost Device")

	hmrTouchhostPort = testing.RegisterVarString(
		"inputs.hmr_touchhost_port",
		"9992",
		"Port for xmlrpc server on HMR Touchhost")
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HumanMotionRobotLinearity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run a linearity HMR Test",
		Contacts: []string{
			"chromeos-tango@google.com",
			"wmahon@google.com", // Test author
		},
		BugComponent: "b:189315", // ChromeOS > Platform > System > Input > Stylus
		Attr:         []string{"group:human_motion_robot", "human_motion_robot_linearity"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.inputs.StylusEvtestCaptureService"},
		Timeout:      15 * time.Minute,
	})
}

// HumanMotionRobotLinearity runs a set of stylus touch motions on a DUT, then captures
// the DUT's evtest stylus output.
func HumanMotionRobotLinearity(ctx context.Context, s *testing.State) {
	var touchhostHostname string
	if hmrTouchhostHostname.Value() == "" {
		hostName := s.DUT().HostName()
		splitName := strings.Split(hostName, ".")
		splitName[0] = strings.Split(splitName[0], ":")[0]
		splitName[0] = splitName[0] + "-touchhost"
		touchhostHostname = strings.Join(splitName, ".")
	} else {
		touchhostHostname = hmrTouchhostHostname.Value()
	}

	touchhostPort, err := strconv.Atoi(hmrTouchhostPort.Value())
	if err != nil {
		s.Fatalf("Failed to convert inputs.hmr_touchhost_port with value: %s to integer: %v",
			hmrTouchhostPort.Value(), err)
	}

	hostTouchLogFilePath := filepath.Join(s.OutDir(), touchLogFileName)
	hostRawTouchLogFilePath := filepath.Join(s.OutDir(), "raw_"+touchLogFileName)

	// Create a SSH Tunnel to connect to the TouchHost device from the remote drone.
	sshOptions := &ssh.Options{
		KeyDir:  s.DUT().KeyDir(),
		KeyFile: s.DUT().KeyFile(),
	}

	err = ssh.ParseTarget(touchhostHostname, sshOptions)
	if err != nil {
		s.Fatalf("Failed to parse ssh target touchhost host (%s) - %s", touchhostHostname, err)
	}
	sshConn, err := ssh.New(ctx, sshOptions)
	if err != nil {
		s.Fatalf("Failed to connect to touchhost host over ssh %s - %s", touchhostHostname, err)
	}

	onFwdError := func(err error) {
		testing.ContextLogf(ctx, "ssh forwarding error for touchhostd host %s: %s", touchhostHostname, err)
	}

	// Forwards TouchHostD's port on TouchHost to 9992 on localhost.
	hmrSSHForwarder, err := sshConn.ForwardLocalToRemote("tcp", "127.0.0.1:9992", "127.0.0.1:"+hmrTouchhostPort.Value(), onFwdError)
	if err != nil {
		s.Fatalf("Failed to port forward touchhost %s port %d - %s", touchhostHostname, touchhostPort, err)
	}

	hmrInterface, err := newHMRInterface(ctx, "127.0.0.1", 9992)
	if err != nil {
		s.Fatal("Error generating new HMR interface: ", err)
	}

	client, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer client.Close(ctx)

	// Checks that the GCode file is already loaded on HMR).
	fileExists, err := hmrInterface.RPC("FileExists").Args(gcodeFileName).CallForBool(ctx)
	if err != nil {
		s.Fatal("Failed to call FileExists: ", err)
	}
	if !fileExists {
		s.Fatalf("Gcode file (%s) not found on HMR", gcodeFileName)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	DutEvtestService := inputspb.NewStylusEvtestCaptureServiceClient(client.Conn)

	go func() {
		// Start recording evtest stylus touch data from DUT.
		dutResponse, err := DutEvtestService.StartStylusDataCapture(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("Failed to run StartStylusDataCapture : ", err)
		}
		// Copy file from DUT to Host machine.
		dutTouchLogFilePath := dutResponse.GetStylusLogPath()
		err = linuxssh.GetFile(ctx, s.DUT().Conn(), dutTouchLogFilePath, hostRawTouchLogFilePath, linuxssh.PreserveSymlinks)
		if err != nil {
			s.Fatal("Failed to copy file from DUT to Host: ", err)
		}
		wg.Done()
	}()

	// Begins executing HMR motions on DUT.
	err = hmrInterface.RPC("StartJob").Args(gcodeFileName).Call(ctx)
	if err != nil {
		s.Fatal("Failed to start job: ", err)
	}

	// Polls TouchHost for progress of HMR job. Blocks until the HMR job is complete.
	prevProgress := -1.0
	err = action.Retry(100, func(ctx context.Context) error {
		progress, err := hmrInterface.RPC("GetProgress").Args().CallForFloat64(ctx)
		if err != nil {
			s.Fatal("Failed to poll progress: ", err)
			return err
		}
		// progress resets to 0 when job is complete.
		if prevProgress <= progress {
			prevProgress = progress
			return errors.Errorf("HMR job is still in progress (%f%%)", progress*100)
		}
		return nil
	}, 3*time.Second)(ctx)

	if err != nil {
		s.Fatal("HMR Job did not complete in time")
		hmrInterface.RPC("StopJob").Call(ctx)
	}

	hmrSSHForwarder.Close()

	// Stop DUT evtest stylus touch data capture.
	if _, err = DutEvtestService.StopStylusDataCapture(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to run StopStylusDataCapture: ", err)
	}
	// Wait until stylus touch data file has been copied from DUT to Host.
	wg.Wait()

	// Delete stylus touch data file from DUT.
	if _, err = DutEvtestService.CleanUp(ctx, &empty.Empty{}); err != nil {
		s.Error("Failed to run CleanUp: ", err)
	}

	// Clean stylus touch data of common errors.
	err = removeCommonDataErrorsFromStylusLogFile(hostRawTouchLogFilePath, hostTouchLogFilePath)
	if err != nil {
		s.Error("Failed to clean raw touchlog file: ", err)
	}
}

func newHMRInterface(ctx context.Context, host string, port int) (*xmlrpc.CommonRPCInterface, error) {
	hmrInterface := xmlrpc.NewCommonRPCInterface(xmlrpc.New(host, port), "")

	err := action.Retry(5, func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Attempting to connect to HMR Touchhost: %s:%d", host, port)
		deadline, _ := ctx.Deadline()
		timeUntil := time.Until(deadline)
		testing.ContextLogf(ctx, "time until deadline: %s", timeUntil)
		err := hmrInterface.RPC("TestConnection").Timeout(30 * time.Second).Call(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to communicate with HMR device with a call to TestConnection on HMR Touchhost")
		}
		return nil
	}, 30*time.Second)(ctx)

	return hmrInterface, err
}

// removeCommonDataErrorsFromStylusLogFile checks for common touch log data errors and generates a cleaned touch log file with these errors removed.
func removeCommonDataErrorsFromStylusLogFile(rawFilePath, cleanedFilePath string) error {
	readFile, err := os.Open(rawFilePath)
	if err != nil {
		return err
	}
	defer readFile.Close()

	writeFile, err := os.Create(cleanedFilePath)
	if err != nil {
		return err
	}
	defer writeFile.Close()

	scanner := bufio.NewScanner(readFile)
	writer := bufio.NewWriter(writeFile)

	// All touch log csv files must contain "x,y,pressure,time" as their header.
	scanner.Scan()
	line := scanner.Text()
	if line != "x,y,pressure,time" {
		return errors.New(rawFilePath + " does not contain csv header")
	}
	_, err = writer.WriteString(line + "\n")
	if err != nil {
		return err
	}

	// There are two common data errors that are being checked for:

	// The prefix error occurs when an number of rows at the start of a touch log file are missing elements.
	// Example:
	// 	10216,5164,,1691552057.683170,
	//	10216,,,1691552067.683172,
	// These errors must occur before the first row with all elements.
	// prefixDataChecked refers to whether a row with all elements has been read yet.
	// If a row missing elements is read after a row with all elements has been read, a fatal error is triggered.

	// The suffix error occurs when the final row at the end of a touch log file excludes some delimiters & elements. (The last element may also be only partially written)
	// Example:
	// 8200,680
	// suffixDataChecked refers to whether this row has been read yet.
	// If any row is read after this row, a fatal error is triggered.

	prefixDataChecked := false
	suffixDataChecked := false
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		splitLine := strings.Split(line, ",")
		if len(splitLine) == maxNumParts {
			if splitLine[xIdx] == "" || splitLine[yIdx] == "" || splitLine[pressureIdx] == "" || splitLine[timeIdx] == "" {
				if !prefixDataChecked {
					continue
				} else {
					return errors.New(rawFilePath + " contains rows with empty elements")
				}
			} else {
				prefixDataChecked = true
			}
		} else if len(splitLine) < maxNumParts {
			if !suffixDataChecked {
				suffixDataChecked = true
				continue
			} else {
				return errors.New(rawFilePath + " contains rows with less than 4 elements")
			}
		} else {
			return errors.New(rawFilePath + " contains a row with excess columns")
		}
		_, err = writer.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}
	writer.Flush()
	return nil
}
