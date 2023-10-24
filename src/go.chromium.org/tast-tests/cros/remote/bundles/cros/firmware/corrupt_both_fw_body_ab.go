// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CorruptBothFWBodyAB,
		Desc: "Corrupt both copies of AP firmware, verify broken screen with reason 0x1b, restore backup via servo",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		Timeout:      15 * time.Minute,
		SoftwareDeps: []string{"crossystem", "flashrom"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:              "normal_mode",
				Fixture:           fixture.NormalMode,
				Val:               "normal",
				ExtraAttr:         []string{"firmware_bios"},
				ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
			},
			{
				Name:      "dev_mode",
				Fixture:   fixture.DevModeGBB,
				Val:       "developer",
				ExtraAttr: []string{"firmware_unstable"},
			},
		},
	})
}

func CorruptBothFWBodyAB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	shouldRestoreFirmware := false
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()

	s.Log("Backup AP firmware")
	out, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed creating remote temp dir: ", err)
	}
	remoteTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		err := h.DUT.Conn().CommandContext(cleanupContext, "rm", "-rf", remoteTempDir).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed deleting remote temp dir: ", err)
		}
	}()
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "read", fmt.Sprintf("%s/bios_backup.bin", remoteTempDir)).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed taking bios backup: ", err)
	}
	out, err = h.ServoProxy.OutputCommand(ctx, false, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwservoXXXXXX")
	if err != nil {
		s.Fatal("Failed to create servo temp dir")
	}
	servoTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		if err := h.ServoProxy.RunCommand(cleanupContext, false, "rm", "-rf", servoTempDir); err != nil {
			s.Fatal("Failed deleting servo temp dir: ", err)
		}
	}()
	localTempDir, err := os.MkdirTemp("", "fwlocal*")
	if err != nil {
		s.Fatal("Failed to create local temp dir")
	}

	s.Log("Downloading backup to ", localTempDir)
	if err := linuxssh.GetFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/bios_backup.bin", remoteTempDir), fmt.Sprintf("%s/bios_backup.bin", localTempDir), linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to download: ", err)
	}
	s.Log("Copying file to servohost ", servoTempDir)
	if err := h.ServoProxy.PutFiles(ctx, false, map[string]string{
		fmt.Sprintf("%s/bios_backup.bin", localTempDir): fmt.Sprintf("%s/bios_backup.bin", servoTempDir),
	}); err != nil {
		s.Fatal("Failed to copy files to servo host: ", err)
	}

	var cutoffEvent reporters.Event
	oldEvents, err := h.Reporter.EventlogList(ctx)
	if err != nil {
		s.Fatal("Finding last event: ", err)
	}
	if len(oldEvents) > 0 {
		cutoffEvent = oldEvents[len(oldEvents)-1]
	}

	s.Log("Corrupting FW bodies")
	restoreFirmware := func(ctx context.Context) {
		s.Log("Restoring AP firmware via servo")

		if err := h.ServoProxy.RunCommand(ctx, true, "futility", "update", "--servo", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()),
			"--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/bios_backup.bin", servoTempDir)); err != nil {
			s.Fatal("Failed restoring firmware via servo: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Error("Failed to connect to DUT: ", err)
		}
		shouldRestoreFirmware = false
	}
	defer func() {
		if shouldRestoreFirmware {
			restoreFirmware(cleanupContext)
		}
	}()
	out, err = h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", "-p", fmt.Sprintf("%s/bios_backup.bin", remoteTempDir), string(bios.FWBodyAImageSection), string(bios.FWBodyBImageSection)).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed getting section sizes: ", err)
	}
	fmapRe := regexp.MustCompile(`(?m)^(\S+) \d+ (\d+)`)
	matches := fmapRe.FindAllSubmatch(out, -1)
	if matches == nil {
		s.Fatal("Output doesn't match regex: ", string(out))
	}
	for _, m := range matches {
		out, err = h.DUT.Conn().CommandContext(ctx, "dd", fmt.Sprintf("of=%s/%s.bin", remoteTempDir, string(m[1])), "if=/dev/random", fmt.Sprintf("bs=%s", string(m[2])), "count=1").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed creating corrupt file: ", err)
		}
	}
	err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/corrupt.bin", remoteTempDir), fmt.Sprintf("%s/bios_backup.bin", remoteTempDir),
		fmt.Sprintf("%s:%s/%s.bin", string(bios.FWBodyAImageSection), remoteTempDir, string(bios.FWBodyAImageSection)),
		fmt.Sprintf("%s:%s/%s.bin", string(bios.FWBodyBImageSection), remoteTempDir, string(bios.FWBodyBImageSection)),
	).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed load_fmap: ", err)
	}
	shouldRestoreFirmware = true
	err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/corrupt.bin", remoteTempDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed flashing corrupt fw: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.SkipWaitConnect); err != nil {
		s.Fatal("Failed to reboot after corrupting: ", err)
	}
	waitContext, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	s.Logf("Waiting %s(DelayRebootToPing) for DUT not to boot", h.Config.DelayRebootToPing)
	if err := h.WaitConnect(waitContext); err == nil {
		s.Error("DUT is unexpectedly up, corruption failed")
	}
	restoreFirmware(cleanupContext)

	s.Log("Checking eventlog for evidence of broken screen")

	// Sometimes events are missing if you check too quickly after boot.
	var events []reporters.Event
	if err := testing.Poll(ctx, func(context.Context) error {
		var err error
		events, err = h.Reporter.EventlogListAfter(ctx, cutoffEvent)
		if err != nil {
			return testing.PollBreak(err)
		}
		if len(events) == 0 {
			return errors.New("no new events found")
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute, Interval: 5 * time.Second,
	}); err != nil {
		s.Fatal("Gathering events: ", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, "RW firmware unable to verify firmware body") {
			found = true
			break
		}
	}
	if !found {
		s.Error("Did not find expected recovery reason in event log: ", events)
	}
}
