// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type cCDCapabilitiesGscFullConsoleParam struct {
	capState                     servo.CCDCapState
	commandsSucceedWhenCcdOpen   bool
	commandsSucceedWhenCcdLocked bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: CCDCapabilitiesGscFullConsole,
		Desc: "Test to verify GscFullConsole locks out restricted console commands",
		Attr: []string{"group:firmware", "group:hwsec", "firmware_cr50"},
		Contacts: []string{
			"cros-hwsec@google.com",
			"mvertescher@google.com",
			"chromeos-faft@google.com",
		},
		BugComponent: "b:1188654",
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.GSCUART()),
		SoftwareDeps: []string{"gsc", "reboot"},
		Timeout:      2 * time.Minute,
		Vars:         []string{"servo"},
		Params: []testing.Param{{
			Name: "cap_default",
			Val: cCDCapabilitiesGscFullConsoleParam{
				capState:                     servo.CapDefault,
				commandsSucceedWhenCcdOpen:   true,
				commandsSucceedWhenCcdLocked: false,
			},
		}, {
			Name: "cap_always",
			Val: cCDCapabilitiesGscFullConsoleParam{
				capState:                     servo.CapAlways,
				commandsSucceedWhenCcdOpen:   true,
				commandsSucceedWhenCcdLocked: true,
			},
		}, {
			Name:              "cap_unless_locked",
			ExtraHardwareDeps: hwdep.D(hwdep.HasGSCCr50()),
			Val: cCDCapabilitiesGscFullConsoleParam{
				capState:                     servo.CapUnlessLocked,
				commandsSucceedWhenCcdOpen:   true,
				commandsSucceedWhenCcdLocked: false,
			},
		}, {
			Name: "cap_if_opened",
			Val: cCDCapabilitiesGscFullConsoleParam{
				capState:                     servo.CapIfOpened,
				commandsSucceedWhenCcdOpen:   true,
				commandsSucceedWhenCcdLocked: false,
			},
		}},
	})
}

func CCDCapabilitiesGscFullConsole(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	userParams := s.Param().(cCDCapabilitiesGscFullConsoleParam)

	if err := h.OpenCCD(ctx, true, false); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}

	ccdSettings := map[servo.CCDCap]servo.CCDCapState{"GscFullConsole": userParams.capState}
	if err := h.Servo.SetCCDCapability(ctx, ccdSettings); err != nil {
		s.Fatal("Failed to set `GscFullConsole` capability state: ", err)
	}

	runGscFullConsoleCommands(ctx, s, userParams.commandsSucceedWhenCcdOpen)

	if err := h.Servo.LockCCD(ctx); err != nil {
		s.Fatal("Failed to lock ccd: ", err)
	}

	// Open CCD when finished
	defer func() {
		if err := h.OpenCCD(ctx, true, false); err != nil {
			s.Fatal("Failed to open CCD: ", err)
		}
	}()

	runGscFullConsoleCommands(ctx, s, userParams.commandsSucceedWhenCcdLocked)
}

func runGscFullConsoleCommands(ctx context.Context, s *testing.State, expectSuccess bool) {
	h := s.FixtValue().(*fixture.Value).Helper

	for _, tc := range []struct {
		command      string
		successRegex string
		failureRegex string
	}{
		{
			command:      "idle s",
			successRegex: "idle action: sleep",
			failureRegex: "Console is locked|Access Denied",
		},
		{
			command:      "recbtnforce enable",
			successRegex: "RecBtn",
			failureRegex: "Access Denied",
		},
		{
			command:      "rddkeepalive true",
			successRegex: "Forcing",
			failureRegex: "Parameter 1 invalid|Access Denied",
		},
	} {
		regex := tc.successRegex
		if !expectSuccess {
			regex = tc.failureRegex
		}

		if err := h.Servo.CheckGSCCommandOutput(ctx, tc.command, []string{regex}); err != nil {
			s.Fatal("Failed to match GSC command output, expected command `"+tc.command+"` to succeed = "+strconv.FormatBool(expectSuccess)+": ", err)
		}
	}
}
