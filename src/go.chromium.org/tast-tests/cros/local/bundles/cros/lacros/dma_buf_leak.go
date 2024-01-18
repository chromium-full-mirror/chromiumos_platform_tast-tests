// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/mem"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DmaBufLeak,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A regression test for crbug.com/1442990 that checks for potential dma buffer leak",
		Contacts:     []string{"lacros-team@google.com", "diwux@chromium.org", "petermcneeley@chromium.org"},
		BugComponent: "b:1456869",
		SoftwareDeps: []string{"chrome", "lacros"},
		Fixture:      "lacros",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"lacros_stable"},
			ExtraAttr:         []string{"group:mainline"},
			// Kaisa is a Puff and is currently used in Lacros Green Release
			// Voxel is a Volteer and is currently used in Browser CQ and OS CQ chrome uprev
			ExtraHardwareDeps: hwdep.D(hwdep.Model("kaisa", "voxel")),
		}, {
			Name:              "unstable",
			ExtraSoftwareDeps: []string{"lacros_unstable"},
			ExtraAttr:         []string{"group:mainline", "informational"},
		}},
	})
}

func setWindowSizeMaximized(ctx context.Context, tconn *chrome.TestConn) error {
	if err := tconn.Call(ctx, nil, `async () => {
		const win = await tast.promisify(chrome.windows.getLastFocused)();
		await tast.promisify(chrome.windows.update)(win.id, {state:"maximized"});
	}`); err != nil {
		return errors.Wrap(err, "setting window size failed")
	}

	return nil
}

// DmaBufLeak is a regression test that watches out for regressions such as crbug.com/1442990.
// What it does is to simply open five full screen YouTube tabs, and switch between them for
// 100 times. Without memory leak, this amount of tab switching will only consume a few
// hundreds of megabytes of free RAM. With memory leak such as the one tracked in
// crbug.com/1442990, such tab switching can easily consume several gigabytes of free RAM.
// The way for reading RAM consumption in this test is to read the MemFree field inside
// /proc/meminfo.
func DmaBufLeak(ctx context.Context, s *testing.State) {
	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch lacros-chrome: ", err)
	}
	defer l.Close(ctx)
	if err := navigateToAnEmptyTabAndSwitchBetweenThemLacros(ctx, tconn, l); err != nil {
		s.Fatal("DmfBuf test failed: ", err)
	}
}

func navigateToAnEmptyTabAndSwitchBetweenThemLacros(ctx context.Context, tconn *chrome.TestConn, l *lacros.Lacros) error {
	conn, err := l.NewConnForTarget(ctx, chrome.MatchAllPages())
	if err != nil {
		return errors.Wrap(err, "failed to find an about:blank tab")
	}
	if err := conn.Navigate(ctx, "chrome://newtab"); err != nil {
		return errors.Wrap(err, "failed to navigate to YouTube")
	}
	ltconn, err := l.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get lacros-chrome TestConn")
	}
	if err := setWindowSizeMaximized(ctx, ltconn); err != nil {
		return errors.Wrap(err, "failed to maximize Lacros window")
	}
	if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
		return w.State == ash.WindowStateMaximized
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for maximized window")
	}
	if err := conn.Close(); err != nil {
		return errors.Wrap(err, "failed to close youtube tab connection")
	}
	for i := 0; i < 4; i++ {
		conn, err := l.NewConn(ctx, "chrome://newtab")
		if err != nil {
			return errors.Wrap(err, "failed to open youtube tab")
		}
		if err := conn.Close(); err != nil {
			return errors.Wrap(err, "failed to close youtube tab connection")
		}
	}
	// GoBigSleepLint: sleep to let memory settle.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep before the test begins")
	}
	vmstat, err := mem.VirtualMemory()
	if err != nil {
		return errors.Wrap(err, "failed to get memory stats")
	}
	const mb = 1024 * 1024
	freeBytesBefore := vmstat.Free
	testing.ContextLogf(ctx, "MemFree before: %v MB", freeBytesBefore/mb)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	// Fixed iterations of 8 with fixed time of 7 seconds means this test will have a max of 56 seconds.
	for k := 0; k < 8; k++ {
		// To initiate a press we first need to press and release once.
		if err := kb.Accel(ctx, "Ctrl+Tab"); err != nil {
			return errors.Wrap(err, "failed to send Ctrl+Tab")
		}

		// This press will rapidly iterate through all tabs in the window.
		if err := kb.AccelPress(ctx, "Ctrl+Tab"); err != nil {
			return errors.Wrap(err, "failed to send Ctrl+Tab")
		}
		// GoBigSleepLint: This will be the press(hold) time.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep after the test completed")
		}
		if err := kb.AccelRelease(ctx, "Ctrl+Tab"); err != nil {
			return errors.Wrap(err, "failed to send Ctrl+Tab")
		}
		// GoBigSleepLint: sleep to let memory settle.
		if err := testing.Sleep(ctx, 2*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep after the test completed")
		}
		vmstat, err = mem.VirtualMemory()
		if err != nil {
			return errors.Wrap(err, "failed to get memory stats")
		}
		freeBytesAfter := vmstat.Free
		testing.ContextLogf(ctx, "MemFree after: %v MB iter %v", freeBytesAfter/mb, k)
		if freeBytesAfter >= freeBytesBefore {
			testing.ContextLog(ctx, "MemFree delta is negative. This indicates there's no leak")
			continue
		}
		deltaPercentage := 100 * (freeBytesBefore - freeBytesAfter) / freeBytesBefore
		testing.ContextLogf(ctx, "MemFree delta percentage: %v %%", deltaPercentage)
		// In normal, non-leaky environment, this delta is single digit or even negative,
		// as pointed out by the above negativity check. In the case of a dma buf leak,
		// or other kernel/driver leak, on a 8GB RAM device, this delta can easily get
		// bloated to 80 or more.
		if deltaPercentage > 20 {
			return errors.New("Potential memory leak detected during tab switching")
		}
	}
	return nil
}
