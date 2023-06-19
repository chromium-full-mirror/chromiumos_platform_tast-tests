// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

type servoV4Flash struct {
	firmwareType   string
	programmerInit flashrom.Programmer
	region         string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         FlashUsingServoV4,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "System should support flashing ec/coreboot using Servo v4",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"servo"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.security.BootLockboxService"},
		Attr:         []string{"group:intel-flashing"},
		Fixture:      fixture.NormalMode,
		Vars:         []string{"firmware.firmwarePath"},
		Timeout:      30 * time.Minute,
		Params: []testing.Param{{
			Name: "ec",
			Val: servoV4Flash{
				firmwareType:   "ec",
				programmerInit: flashrom.ProgrammerEc,
				region:         "EC_RW",
			},
		}, {
			Name: "coreboot",
			Val: servoV4Flash{
				firmwareType:   "coreboot",
				programmerInit: flashrom.ProgrammerHost,
				region:         "RW_SECTION_B",
			},
		}},
	})
}

// FlashUsingServoV4 flashes ec/coreboot using Servo v4.
func FlashUsingServoV4(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	testParms := s.Param().(servoV4Flash)

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	if isV4, err := h.Servo.IsServoV4(ctx); err != nil {
		s.Fatal("Failed to determine whether servo is v4: ", err)
	} else if !isV4 {
		s.Fatal("Servo must be v4")
	}

	pathToFirmware := s.RequiredVar("firmware.firmwarePath")

	if _, err := os.Stat(pathToFirmware); os.IsNotExist(err) {
		s.Fatalf("File %q not found: %v", pathToFirmware, err)
	} else if err != nil {
		s.Fatal("Failed to find: ", err)
	}

	dut := s.DUT()
	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	client := dutfs.NewClient(cl.Conn)
	tmp, err := client.TempDir(ctx, "/tmp", "temp")
	if err != nil {
		s.Fatal("Failed to create tmp directory: ", err)
	}

	binPath := filepath.Join(tmp, pathToFirmware)
	corebootPath := map[string]string{pathToFirmware: binPath}
	if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), corebootPath, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to send data to remote data path: ", err)
	}
	var flashromConfig flashrom.Config
	flash, ctx, cleanup, _, err := flashromConfig.
		ProgrammerInit(testParms.programmerInit, "").
		SetDut(h.DUT).
		Probe(ctx)
	defer cleanup()
	if err != nil {
		s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
	}

	ctx, restore, orginal, err := utils.BackupAndRestoreAPFirmwareAndWriteProtect(ctx, h.DUT, h.Servo, flash)
	if err != nil {
		s.Fatal("Firmware backup failed: ", err)
	}
	defer restore(s)

	if _, err := flash.Write(ctx, binPath, false, true, "", nil); err != nil {
		s.Fatal("Failed to flash coreboot/ec: ", err)
	}

	if _, err := flash.Verify(ctx, *orginal, testParms.region); err != nil {
		s.Fatal("Failed to verify flashing of coreboot/ec: ", err)
	}

}
