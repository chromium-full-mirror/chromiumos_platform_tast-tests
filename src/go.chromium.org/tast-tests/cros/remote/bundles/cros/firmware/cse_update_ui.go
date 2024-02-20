// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
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
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		SoftwareDeps: []string{"flashrom"},
		Fixture:      fixture.NormalMode,
		Timeout:      30 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.MainboardHasEarlyLibgfxinit()),
	})
}

func CSEUpdateUI(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	dut := s.DUT()

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}
	backup, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{Programmer: pb.Programmer_BIOSProgrammer})
	if err != nil {
		s.Fatal("Failed to backup BIOS")
	}
	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to EnsureDUTBooted: ", err)
		}
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Failed to require BiosServiceClient: ", err)
		}
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, backup); err != nil {
			s.Fatal("Failed to restore BIOS: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "rm", "-f", backup.Path).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete BIOS backup: ", err)
		}
	}(ctx)

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

	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	s.Log("Upgrade BIOS to backup version")
	if err := dut.Conn().CommandContext(ctx, "futility", "update", "--mode=autoupdate", "--wp=1", "-i", backup.Path).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to upgrade bios: ", err)
	}

	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
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
