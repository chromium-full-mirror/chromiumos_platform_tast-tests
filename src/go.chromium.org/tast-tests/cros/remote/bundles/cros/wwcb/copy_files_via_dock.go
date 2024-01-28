// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook
package wwcb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	sampleTXT = "sample.txt"
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
		Vars:         []string{"DockingID", "USBTypeAIDArray", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService"},
		Data:         []string{sampleTXT},
		Params: []testing.Param{
			{
				Name:      "smoke",
				ExtraAttr: []string{"pasit_fast"},
			}},
	})
}

func CopyFilesViaDock(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	var USBTypeAIDArray []string
	for _, ID := range strings.Split(s.RequiredVar("USBTypeAIDArray"), ",") {
		USBTypeAIDArray = append(USBTypeAIDArray, ID)
	}

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

	// Open IP power to supply docking power.
	if err := utils.OpenIppower(ctx, []int{1}); err != nil {
		s.Fatal("Failed to open IP power: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, []int{1})

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to connect docking station: ", err)
	}

	before, err := utils.GetUSBDevice(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get original USB devices: ", err)
	}

	for _, ID := range USBTypeAIDArray {
		if err := utils.ControlFixture(ctx, ID, "on"); err != nil {
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

	// Push file to remote.
	remoteTXTPath, err := pushFileToTmpDir(ctx, s, dut, sampleTXT)
	if err != nil {
		s.Fatal("Failed to push file to DUT's tmp directory: ", err)
	}
	defer dut.Conn().CommandContext(ctx, "rm", remoteTXTPath).Output()

	// Retrieve USB path.
	cmd := fmt.Sprint("lsblk -l -o mountpoint | grep removable")
	output, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to retrieve USB path: ", err)
	}

	for _, path := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		USBTXTPath := filepath.Join(path, sampleTXT)
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
