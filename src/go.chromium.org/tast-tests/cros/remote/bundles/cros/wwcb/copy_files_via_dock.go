// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook
package wwcb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	sampleTXT        = "sample.txt"
	removableDirPath = "/media/removable"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CopyFilesViaDock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Copy file to flash drive connecting via a Dock",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"newTestItem"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.ui.ChromeUIService", "tast.cros.ui.ScreenRecorderService"},
		Fixture:      "wwcbPasitDock",
		Data:         []string{"Capabilities.json", sampleTXT},
		Timeout:      utils.TestingTimeout,
		Params: []testing.Param{
			{
				Name:      "fast",
				ExtraAttr: []string{"pasit_fast"},
			}},
	})
}

// CopyFilesViaDock runs a test to copy a file to a removable storage device connected via dock
// and then compares the files with what's on the DUT to verify that the copy completed successfully.
func CopyFilesViaDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dut := s.DUT()

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	// Dump the UI tree and screenshot on any failure
	utils.AttachErrorHandlersForUITreeDump(cleanupCtx, s, cl.Conn)

	// Start screen recording, defer the stop and saving of said recording.
	screenRecorder := ui.NewScreenRecorderServiceClient(cl.Conn)
	utils.StartRecording(ctx, s, screenRecorder)
	defer utils.StopAndSaveScreenRecording(cleanupCtx, s, screenRecorder)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	tf := s.FixtValue().(*topology.TestFixture)

	// Get the first available dock.
	docks := tf.Helper.DevicesByType(topology.DeviceTypeDockingStation)
	if len(docks) != 1 {
		s.Fatalf("Failed to find dock, expected 1 docks got: %d", len(docks))
	}
	dockID := docks[0]

	// Make sure to switch off the Type-A fixture
	// Enable any auxiliary USB devices that pass through the dock, these devices are categorized as "HID"
	USBTypeAIDArray := tf.Helper.DevicesByTypeViaId(topology.DeviceTypeHID, dockID)
	for _, ID := range USBTypeAIDArray {
		if err := tf.Helper.DeactivateDeviceByID(ctx, ID); err != nil {
			s.Fatal("Failed to disconnect USB Type-A device: ", err)
		}
	}

	before, err := utils.GetUSBDevice(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get original USB devices: ", err)
	}

	for _, ID := range USBTypeAIDArray {
		if err := tf.Helper.ActivateDeviceByID(ctx, ID); err != nil {
			s.Fatal("Failed to connect USB Type-A device: ", err)
		}
	}

	// Expect number of USB devices is not less than number of input parameters.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		after, err := utils.GetUSBDevice(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get USB devices after the dock connected to a list of USB devices: ", err)
		}

		if len(after)-len(before) < len(USBTypeAIDArray) {
			s.Fatalf("Unexpected change of number of USB devices after connecting to dock; expect: %d, actual: %d (from %d to %d)", len(USBTypeAIDArray), (len(after) - len(before)), len(before), len(after))
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		s.Fatal("Failed to check number of USB devices: ", err)
	}

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}

		if err := utils.VerifyUSBTypeADeviceSpeed(ctx, dut, s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the usb devices speed: ", err)
		}
	}

	// Push file to remote.
	remoteTXTPath, err := pushFileToTmpDir(ctx, s, dut, sampleTXT)
	if err != nil {
		s.Fatal("Failed to push file to DUT's tmp directory: ", err)
	}
	defer dut.Conn().CommandContext(ctx, "rm", remoteTXTPath).Output()

	// Retrieve USB storage drive directory location.
	fs := dutfs.NewClient(cl.Conn)
	removableFiles, err := fs.ReadDir(ctx, removableDirPath)
	if err != nil {
		s.Fatal("Failed to list files in removable dir after connecting storage device: ", err)
	}

	var removableDirs []os.FileInfo
	for _, fi := range removableFiles {
		if fi.IsDir() {
			removableDirs = append(removableDirs, fi)
		}
	}

	if len(removableDirs) == 0 {
		s.Fatal("Failed to detect any entry in removable directory")
	}

	for _, path := range removableDirs {
		USBTXTPath := filepath.Join(removableDirPath, path.Name(), sampleTXT)
		testing.ContextLogf(ctx, "Copy file to %s", USBTXTPath)

		// Copy file to USB.
		copyCmd := fmt.Sprintf("cp '%s' '%s'", remoteTXTPath, USBTXTPath)
		if err := dut.Conn().CommandContext(ctx, "sh", "-c", copyCmd).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to copy file to USB: ", err)
		}
		defer dut.Conn().CommandContext(ctx, "rm", USBTXTPath).Output()

		// Compare texts line by line in two file.
		diffCmd := fmt.Sprintf("diff '%s' '%s'", remoteTXTPath, USBTXTPath)
		if err := dut.Conn().CommandContext(ctx, "sh", "-c", diffCmd).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to compare the DUT file with USB file: ", err)
		}
	}
}

// pushFileToTmpDir copies the data file to the DUT tmp path, returning its path on the DUT.
func pushFileToTmpDir(ctx context.Context, s *testing.State, dut *dut.DUT, fileName string) (string, error) {
	const tmpDir = "/tmp"
	remotePath := filepath.Join(tmpDir, fileName)
	testing.ContextLog(ctx, "Copy the file to remote data path: ", remotePath)
	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), map[string]string{
		s.DataPath(fileName): remotePath,
	}, linuxssh.DereferenceSymlinks); err != nil {
		return "", errors.Wrapf(err, "failed to send data to remote data path %v", remotePath)
	}
	return remotePath, nil
}
