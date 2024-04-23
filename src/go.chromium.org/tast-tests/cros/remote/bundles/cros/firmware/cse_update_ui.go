// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CSEUpdateUI,
		Desc:         "Verify user is notified during CSE update",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"flashrom"},
		Fixture:      fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
		Timeout:      30 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.MainboardHasEarlyLibgfxinit()),
	})
}

func CSEUpdateUI(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()

	backupManager := s.FixtValue().(*fixture.Value).BackupManager
	h := s.FixtValue().(*fixture.Value).Helper
	dut := s.DUT()

	out, err := dut.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed creating remote temp dir: ", err)
	}
	dutTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		if err := dut.Conn().CommandContext(cleanupCtx, "rm", "-rf", dutTempDir).Run(ssh.DumpLogOnError); err != nil {
			s.Log("Failed to delete temp dir on DUT: ", err)
		}
	}()

	backupOnDut := filepath.Join(dutTempDir, "bios_backup.bin")

	defer func() {
		if err := h.EnsureDUTBooted(cleanupCtx); err != nil {
			s.Fatal("Failed to EnsureDUTBooted: ", err)
		}

		if err := backupManager.CopyBackupToDut(cleanupCtx, dut, fixture.FirmwareAP, backupOnDut); err != nil {
			s.Fatal("Failed to copy backup firmware image to DUT: ", err)
		}

		futilityInstance, err := futility.NewLocalBuilder(dut).Build()
		if err != nil {
			s.Fatal("Failed to setup futility instance: ", err)
		}

		updateOpts := futility.NewUpdateOptions(backupOnDut).WithHostOnly(true).WithMode(futility.UpdateModeRecovery)
		if out, err := futilityInstance.Update(cleanupCtx, updateOpts); err != nil {
			s.Fatal("Failed to restore firmware: ", err, "\nOutput:\n", string(out))
		}
	}()

	s.Log("Downgrade BIOS to uprev version")
	if err := dut.Conn().CommandContext(ctx, "chromeos-firmwareupdate", "--mode=recovery", "--wp=1").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to downgrade bios: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := backupManager.CopyBackupToDut(cleanupCtx, dut, fixture.FirmwareAP, backupOnDut); err != nil {
		s.Fatal("Failed to copy backup firmware image to DUT: ", err)
	}

	s.Log("Upgrade BIOS to backup version")
	futilityInstance, err := futility.NewLocalBuilder(dut).Build()
	if err != nil {
		s.Fatal("Failed to setup futility instance: ", err)
	}

	updateOpts := futility.NewUpdateOptions(backupOnDut).WithMode(futility.UpdateModeAutoUpdate).WithWriteProtection(futility.WriteProtectionEnable)
	if out, err := futilityInstance.Update(ctx, updateOpts); err != nil {
		s.Fatal("Failed to upgrade firmware: ", err, "\nOutput:\n", string(out))
	}

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	checkMatches := func(ctx context.Context, expected string) error {
		events, err := h.Reporter.EventlogList(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to read event log")
		}
		found := false
		for _, event := range events {
			s.Log("Found event: ", event)
			if event.Message == expected {
				found = true
				break
			}
		}

		if !found {
			return errors.Errorf("expected log message not found: %q", expected)
		}
		return nil
	}

	checkNoMatches := func(ctx context.Context) error {
		events, err := h.Reporter.EventlogList(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to read event log")
		}
		for _, event := range events {
			s.Log("Found event: ", event)
			if strings.Contains(event.Message, "Early Sign of Life") {
				return errors.Errorf("unexpected log message: %q", event.Message)
			}
		}

		return nil
	}

	if err := checkMatches(ctx, "Early Sign of Life | CSE Sync Early SOL Screen Shown"); err != nil {
		s.Error("Firmware log: ", err)
	}

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}
	s.Log("Reboot without update")
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := checkNoMatches(ctx); err != nil {
		s.Error("Firmware log: ", err)
	}
}
