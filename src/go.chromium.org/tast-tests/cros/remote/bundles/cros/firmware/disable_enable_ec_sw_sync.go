// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var (
	ecPathVar = testing.RegisterVarString(
		"firmware.ECPath",
		"$HOME/chromiumos/src/platform/ec/ec.bin",
		"It takes EC path as input for e.g. /path/to/ec.bin")
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DisableEnableECSWSync,
		Desc: "Flash EC using flashrom and enable disable EC SW sync",
		Contacts: []string{
			"chromeos-faft@google.com",
			"intel.chrome.automation.team@intel.com",
			"ambalavanan.m.m@intel.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		SoftwareDeps: []string{"flashrom"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		Attr:         []string{"group:intel-flashing"},
		Fixture:      fixture.NormalMode,
		Timeout:      10 * time.Minute,
	})
}

// DisableEnableECSWSync flashes EC using flashrom and enable disable EC SW sync.
func DisableEnableECSWSync(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	h := s.FixtValue().(*fixture.Value).Helper
	pathToEC := ecPathVar.Value()
	d := s.DUT()

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	oldGBBFlags, err := fwCommon.GetGBBFlags(ctx, d)
	if err != nil {
		s.Fatal("Failed to get initial GBB flags: ", err)
	}

	backupState, err := firmware.BackupECFirmware(ctx, h)
	if err != nil {
		s.Fatal("Failed to backup current EC firmware: ", err)
	}

	ecPathLocal := filepath.Join("/usr/local", filepath.Base(pathToEC))
	s.Log("ecPathLocal: ", ecPathLocal)

	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Error("Failed to ensure DUT is booted during cleanup: ", err)
		}
		_, _ = h.DUT.Conn().CommandContext(ctx, "rm", "-f", ecPathLocal).Output(ssh.DumpLogOnError)
		clearSync := fwpb.GBBFlagsState{Clear: []fwpb.GBBFlag{fwpb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}
		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, d, &clearSync); err != nil {
			s.Error("Failed to clear DISABLE_EC_SOFTWARE_SYNC before EC restore: ", err)
		}
		if err := backupState.Close(ctx, h); err != nil {
			s.Error("Failed to restore EC backup: ", err)
		}
		if _, err := fwCommon.ClearAndSetGBBFlags(ctx, d, oldGBBFlags); err != nil {
			s.Error("Failed to restore initial GBB flags: ", err)
		}
	}(cleanupCtx)

	ecKeyPath := map[string]string{pathToEC: ecPathLocal}
	if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), ecKeyPath, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send data to remote data path %v: %v", ecPathLocal, err)
	}

	ro1, rw1, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).RORWVersion(ctx)
	if err != nil {
		s.Fatal("Failed to read ectool version: ", err)
	}
	if ro1 != rw1 {
		s.Fatalf("Failed to verify EC version, versions should be the same, got: RO=%s, RW=%s, want: RO == RW", ro1, rw1)
	}

	s.Log("Setting GBB flag DISABLE_EC_SOFTWARE_SYNC")
	set := fwpb.GBBFlagsState{Set: []fwpb.GBBFlag{fwpb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}
	if _, err := fwCommon.ClearAndSetGBBFlags(ctx, s.DUT(), &set); err != nil {
		s.Fatal("Failed to set GBB flag: ", err)
	}

	s.Log("Rebooting DUT")
	if err := h.DUT.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	ro2, rw2, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).RORWVersion(ctx)
	if err != nil {
		s.Fatal("Failed to read ectool version: ", err)
	}
	if ro1 != ro2 || rw1 != rw2 {
		s.Fatalf("Failed to verify EC versions, versions should be the same, got %s=%s (RO), %s=%s (RW)", ro1, ro2, rw1, rw2)
	}

	if _, _, err := backupState.Flash(ctx, h, ecPathLocal); err != nil {
		s.Fatal("Failed to flash EC: ", err)
	}

	s.Log("Cold Reboot DUT")
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to perform mode aware reboot: ", err)
	}

	ro3, rw3, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).RORWVersion(ctx)
	if err != nil {
		s.Fatal("Failed to read ectool version: ", err)
	}
	if ro3 == ro2 || rw3 == rw2 {
		s.Fatalf("Failed to verify EC version, EC should change as per flashed EC binary got (%s, %s) RO (%s, %s) RW", ro2, ro3, rw2, rw3)
	}

	s.Log("Clearing GBB flag DISABLE_EC_SOFTWARE_SYNC")
	clear := fwpb.GBBFlagsState{Clear: []fwpb.GBBFlag{fwpb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC}}
	if _, err := fwCommon.ClearAndSetGBBFlags(ctx, s.DUT(), &clear); err != nil {
		s.Fatal("Failed to clear GBB flag: ", err)
	}

	s.Log("Rebooting DUT")
	if err := h.DUT.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	ro4, rw4, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).RORWVersion(ctx)
	if err != nil {
		s.Fatal("Failed to read ectool version: ", err)
	}
	if ro4 == rw4 && ro4 != ro3 {
		s.Fatalf("Failed to verify EC version, RW and RO regions of EC should not be the same version got %s=%s (RO, RW), RO version of EC should be version of EC flashed got %s=%s (RO, RO)", ro4, rw4, ro3, ro4)
	}
}
