// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryTraining,
		Desc:         "Verify user is notified during memory training",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		SoftwareDeps: []string{"flashrom"},
		Fixture:      fixture.NormalMode,
		Timeout:      30 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.MainboardHasEarlyLibgfxinit()),
	})
}

func MemoryTraining(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	dut := s.DUT()

	s.Log("Clearing MRC cache")
	if err := dut.Conn().CommandContext(ctx, "flashrom", "-p", "host", "-E", "-i", "RW_MRC_CACHE").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to clear MRC cache: ", err)
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

	expectedLogRe := regexp.MustCompile(`Informing user on-display of ([^\.]*)\.`)
	startBootRe := regexp.MustCompile(`coreboot.*romstage starting`)
	checkMatches := func(ctx context.Context, expected string) error {
		output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
		if err != nil {
			return errors.Wrap(err, "failed to read firmware log")
		}
		scanner := bufio.NewScanner(strings.NewReader(output))
		found := false
		var mismatchError error
		for scanner.Scan() {
			line := scanner.Text()
			if startBootRe.MatchString(line) {
				found = false
				mismatchError = nil
			} else {
				m := expectedLogRe.FindStringSubmatch(line)
				if m != nil && string(m[1]) != expected {
					mismatchError = errors.Errorf("unexpected log message: %q", string(m[0]))
				}
				found = true
			}
		}

		if !found && expected != "" {
			return errors.Errorf("expected log message not found: %q", expected)
		}
		return mismatchError
	}

	if err := checkMatches(ctx, "memory training"); err != nil {
		s.Error("Firmware log: ", err)
	}

	s.Log("Reboot without clearing MRC cache")
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := checkMatches(ctx, ""); err != nil {
		s.Error("Firmware log: ", err)
	}
}
