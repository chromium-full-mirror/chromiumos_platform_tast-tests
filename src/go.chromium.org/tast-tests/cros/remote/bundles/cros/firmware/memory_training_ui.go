// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryTrainingUI,
		Desc:         "Verify user is notified during memory training",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level5"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		SoftwareDeps: []string{"flashrom"},
		Fixture:      fixture.NormalMode,
		Timeout:      30 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.MainboardHasEarlyLibgfxinit()),
	})
}

func MemoryTrainingUI(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	dut := s.DUT()

	r := h.Reporter
	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	s.Log("Clearing MRC cache")
	var flashromConfig flashrom.Config
	flashromInstance, ctx, shutdown, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(flashrom.ProgrammerHost, "").
		SetDut(dut).
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()
	if err != nil {
		s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
	}

	if _, err := flashromInstance.Erase(ctx, []string{"RW_MRC_CACHE"}); err != nil {
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

	checkMatches := func(ctx context.Context, expected string) error {
		events, err := r.EventlogList(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to read event log")
		}
		found := false
		for _, event := range events {
			s.Log("Found event: ", event)
			if strings.Contains(event.Message, "Early Sign of Life") && event.Message != expected {
				return errors.Errorf("unexpected log message: %q", event.Message)
			}
			if expected != "" && event.Message == expected {
				found = true
			}
		}

		if !found && expected != "" {
			return errors.Errorf("expected log message not found: %q", expected)
		}
		return nil
	}

	if err := checkMatches(ctx, "Early Sign of Life | MRC Early SOL Screen Shown"); err != nil {
		s.Error("Firmware log: ", err)
	}
	if err := h.Reporter.ClearEventlog(ctx); err != nil {
		s.Fatal("Failed to clear event log: ", err)
	}

	s.Log("Reboot without clearing MRC cache")
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := checkMatches(ctx, ""); err != nil {
		s.Error("Firmware log: ", err)
	}
}
