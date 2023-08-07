// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Reboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks whether Android can be successfully rebooted",
		Contacts:     []string{"arc-core@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Timeout: 10 * time.Minute,
	})
}

func Reboot(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		if err := cr.Close(ctx); err != nil {
			s.Fatal("Failed to close Chrome while (re)booting ARC: ", err)
		}
	}()

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer func() {
		if a != nil {
			a.Close(ctx)
		}
	}()

	oldPID, err := arc.InitPID()
	if err != nil {
		s.Fatal("Failed to get init PID before reboot: ", err)
	}

	s.Log("Running reboot command via ADB")
	if err := a.Command(ctx, "reboot").Run(); err != nil {
		s.Fatal("Failed to run reboot command via ADB: ", err)
	}

	s.Log("Waiting for old init process to exit")
	if err := waitProcessExit(ctx, oldPID); err != nil {
		s.Fatal("Failed to wait for old init process to exit: ", err)
	}

	a.Close(ctx)

	// Reboot Android and re-establish ADB connection.
	a, err = arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		// We can assume a == nil at this point.
		s.Fatal("Failed to restart ARC: ", err)
	}

	newPID, err := arc.InitPID()
	if err != nil {
		s.Fatal("Failed to get init PID after reboot: ", err)
	}
	if newPID == oldPID {
		s.Fatal("Failure: init PID did not change")
	}
}

// waitProcessExit waits for a process to exit.
func waitProcessExit(ctx context.Context, pid int32) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := process.NewProcess(pid); err == nil {
			return errors.Errorf("pid %d still exists", pid)
		}
		return nil
	}, &testing.PollOptions{Timeout: 60 * time.Second})
}
