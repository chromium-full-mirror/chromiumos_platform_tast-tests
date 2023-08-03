// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CopyAndMoveFilesExternalStorage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Move and copy the files to erternal storage media and verify files are being copied/moved successfully",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:294328393", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"USBID"},
		Data:         []string{"sample.txt"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
		},
	})
}

func CopyAndMoveFilesExternalStorage(ctx context.Context, s *testing.State) {
	/*
		This test checks the copy and move file after format:
		1- Login.
		2- Plug in the external torage media to chromebook.
		3- Expect number of USB devices is not less than number of input parameters.
		4- Retrieve USB path.
		5- Push file to dut.
		6- Format external storage.
		7- Copy file to external storage and verify file are being copied successfully.
		8- Move file to external storage and verify file are being moved successfully.
	*/
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	usbID := s.RequiredVar("USBID")

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed initialize the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Start Chrome on the DUT.
	cs := ui.NewChromeServiceClient(cl.Conn)
	loginReq := &ui.NewRequest{}
	if _, err := cs.New(ctx, loginReq, grpc.WaitForReady(true)); err != nil {
		s.Fatal("Failed to initialize Chrome login: ", err)
	}
	defer cs.Close(cleanupCtx, &empty.Empty{})

	dut := s.DUT()
	fs := dutfs.NewClient(cl.Conn)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)
	// Push file to remote.
	remoteTextPath, err := utils.PushFileToDUT(ctx, s, dut, "sample.txt", "/tmp")
	if err != nil {
		s.Fatal("Failed to initialize to push file to DUT's /tmp directory: ", err)
	}
	defer dut.Conn().CommandContext(cleanupCtx, "rm", remoteTextPath).Output()

	before, err := utils.GetUSBDevice(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get original USB devices: ", err)
	}

	if err := utils.ControlFixture(ctx, usbID, "on"); err != nil {
		s.Fatal("Failed to plug the external storage media: ", err)
	}
	usbCount := 1
	// Expect number of USB devices is not less than number of input parameters.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		after, err := utils.GetUSBDevice(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get USB devices after plug: ", err)
		}

		if len(after)-len(before) < usbCount {
			s.Fatalf("Failed to unexpected change in the number of USB devices detected; expect: %d, actual: %d (from %d to %d) after plug", usbCount, (len(after) - len(before)), len(before), len(after))
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		s.Fatal("Failed to check number of USB devices after plug: ", err)
	}

	// Retrieve USB path.
	cmd := fmt.Sprint("lsblk -l -o mountpoint | grep removable")
	output, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to retrieve USB path after plug: ", err)
	}

	ctxVerify, cancelVerify := context.WithTimeout(ctx, 3*time.Second)
	defer cancelVerify()
	for _, path := range strings.Split(strings.TrimSpace(string(output)), "\n") {

		if err := utils.FormatStorageToFAT(ctx, path, dut, fs); err != nil {
			s.Fatalf("Failed to format %s: %v", path, err)
		}

		usbTextPathCopy := filepath.Join(path, "sample.txt")
		usbTextPathMove := filepath.Join(path, "sample_move.txt")
		testing.ContextLogf(ctx, "Copy file to %s", usbTextPathCopy)

		// Copy file to external storage.
		if err := dut.Conn().CommandContext(ctxVerify, "cp", remoteTextPath, usbTextPathCopy).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to copy file to external storage: ", err)
		}
		defer dut.Conn().CommandContext(cleanupCtx, "rm", usbTextPathCopy).Output()
		// Compare texts line by line in two files.
		diffCmd := fmt.Sprintf("diff '%s' '%s'", remoteTextPath, usbTextPathCopy)
		if err := dut.Conn().CommandContext(ctx, "sh", "-c", diffCmd).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to compare the DUT file with external storage file after copy: ", err)
		}

		// Move file to external storage.
		testing.ContextLogf(ctx, "Move file to %s", usbTextPathMove)
		if err := dut.Conn().CommandContext(ctxVerify, "mv", remoteTextPath, usbTextPathMove).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to move file to external storage: ", err)
		}
		defer dut.Conn().CommandContext(cleanupCtx, "rm", usbTextPathMove).Output()
		if _, err := utils.PushFileToDUT(ctx, s, dut, "sample.txt", "/tmp"); err != nil {
			s.Fatal("Failed to push file to DUT's /tmp directory: ", err)
		}
		// Compare texts line by line in two files.
		diffCmd = fmt.Sprintf("diff '%s' '%s'", remoteTextPath, usbTextPathMove)
		if err := dut.Conn().CommandContext(ctx, "sh", "-c", diffCmd).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to compare the DUT file with external storage file after move: ", err)
		}
	}
}
