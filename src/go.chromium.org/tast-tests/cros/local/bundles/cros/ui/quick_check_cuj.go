// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: QuickCheckCUJ,
		Desc: "Measures the smoothess of screen unlock and open an gmail thread",
		Contacts: []string{
			"cros-sw-perf@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome", "arc"},
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      4 * time.Minute,
		Fixture:      "loggedInToCUJUser",
	})
}

func QuickCheckCUJ(ctx context.Context, s *testing.State) {
	const (
		lockTimeout     = 30 * time.Second
		goodAuthTimeout = 30 * time.Second
		gmailTimeout    = 30 * time.Second
	)

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	conn, err := cr.NewConn(ctx, chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to setup Chrome: ", err)
	}
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	defer faillog.DumpUITreeOnError(closeCtx, s.OutDir(), s.HasError, tconn)

	password := cr.Creds().Pass

	recorder, err := cujrecorder.NewRecorder(ctx, tconn, cr, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a CUJ recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	const accel = "Search+L"
	s.Log("Locking screen via ", accel)
	if err := kb.Accel(ctx, accel); err != nil {
		s.Fatalf("Typing %v failed: %v", accel, err)
	}
	s.Log("Waiting for Chrome to report that screen is locked")
	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, lockTimeout); err != nil {
		s.Fatalf("Waiting for screen to be locked failed: %v (last status %+v)", err, st)
	}
	defer func() {
		// Ensure that screen is unlocked even if the test fails.
		if st, err := lockscreen.GetState(closeCtx, tconn); err != nil {
			s.Error("Failed to get lockscreen state: ", err)
		} else if st.Locked {
			if err := kb.Type(closeCtx, password+"\n"); err != nil {
				s.Error("Failed ot type password: ", err)
			}
		}
	}()

	var elapsed time.Duration
	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// See go/trace-in-cuj-tests about rules for tracing.
		if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
			return errors.Wrap(err, "failed to start tracing")
		}

		start := time.Now()

		s.Log("Unlocking screen by typing password")
		if err := kb.Type(ctx, password+"\n"); err != nil {
			return errors.Wrap(err, "typing correct password failed")
		}
		s.Log("Waiting for Chrome to report that screen is unlocked")
		if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return !st.Locked }, goodAuthTimeout); err != nil {
			return errors.Wrapf(err, "waiting for screen to be unlocked failed (last status %+v)", st)
		}

		if err := conn.Navigate(ctx, "https://www.gmail.com/"); err != nil {
			return errors.Wrap(err, "failed to open Gmail")
		}

		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get windows")
		}
		if wsCount := len(ws); wsCount != 1 {
			return errors.Wrapf(err, "expected 1 window; found %d", wsCount)
		}
		s.Log("Maximizing the window (if it is not already maximized)")
		if err := ash.SetWindowStateAndWait(ctx, tconn, ws[0].ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to maximize window")
		}

		s.Log("Opening the first email thread")
		firstRow := nodewith.Role(role.Row).First()
		ac := uiauto.New(tconn)
		if err := ac.LeftClick(firstRow)(ctx); err != nil {
			return errors.Wrap(err, "failed to click to open the email thread row")
		}

		if err := webutil.WaitForQuiescence(ctx, conn, gmailTimeout); err != nil {
			return errors.Wrap(err, "failed to wait for gmail to finish loading")
		}

		elapsed = time.Since(start)
		s.Log("Elapsed ms: ", elapsed.Milliseconds())

		// Close any potential security alert that pops up.
		if err := cuj.DismissCriticalSecurityAlert(ctx, tconn, conn); err != nil {
			return errors.Wrap(err, "failed to dismiss Critical Security Alert")
		}

		s.Log("Waiting to simulate a user passively reading the email thread (top scroll position)")
		// GoBigSleepLint: Sleep to simulate user action.
		if err := testing.Sleep(ctx, 10*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep (top scroll position)")
		}

		emailThread := nodewith.Role("genericContainer").ClassName("AO")
		emailThreadBounds, err := ac.Location(ctx, emailThread)
		if err != nil {
			return errors.Wrap(err, "failed to get the email thread location")
		}
		scrollThumbDragOffset := coords.NewPoint(-5, 5)
		scrollThumbDragStart := emailThreadBounds.TopRight().Add(scrollThumbDragOffset)
		scrollThumbDragMiddle := coords.NewPoint(
			emailThreadBounds.Right(),
			emailThreadBounds.Top+nonnegativeIntDivideAndRoundToNearest(emailThreadBounds.Height, 3),
		).Add(scrollThumbDragOffset)
		scrollThumbDragEnd := emailThreadBounds.BottomRight().Add(scrollThumbDragOffset)

		s.Log("Scrolling the email thread (top to middle)")
		if err := mouse.Drag(tconn, scrollThumbDragStart, scrollThumbDragMiddle, 3*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to drag the thumb of the email thread's scrollbar from top to middle")
		}

		s.Log("Waiting to simulate a user passively reading the email thread (middle scroll position)")
		// GoBigSleepLint: Sleep to simulate user action.
		if err := testing.Sleep(ctx, 25*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep (middle scroll position)")
		}

		s.Log("Scrolling the email thread (middle to bottom)")
		if err := mouse.Drag(tconn, scrollThumbDragMiddle, scrollThumbDragEnd, 2*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to drag the thumb of the email thread's scrollbar from middle to bottom")
		}

		s.Log("Waiting to simulate a user passively reading the email thread (bottom scroll position)")
		// GoBigSleepLint: Sleep to simulate user action.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep (bottom scroll position)")
		}

		if err := recorder.StopTracing(ctx); err != nil {
			return errors.Wrap(err, "failed to stop tracing")
		}

		info, err := display.GetPrimaryInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get the primary display info")
		}

		// Drag the mouse to ensure we collect additional mouse drag metrics.
		if err := inputsimulations.RunDragMouseCycle(ctx, tconn, info); err != nil {
			return err
		}

		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := conn.Navigate(ctx, "chrome://version"); err != nil {
			return errors.Wrap(err, "failed to navigate to chrome://version")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to run the test scenario: ", err)
	}

	pv := perf.NewValues()

	pv.Set(perf.Metric{
		Name:      "QuickCheckCUJ.ElapsedTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(elapsed.Milliseconds()))

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to collect the data from the recorder: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}

func nonnegativeIntDivideAndRoundToNearest(n, d int) int {
	return (n + d/2) / d
}
