// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package taskswitchcuj contains helper util and test code for TaskSwitchCUJ.
package taskswitchcuj

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TaskSwitchTest holds parameters for the TaskSwitchCUJ test variants.
type TaskSwitchTest struct {
	BrowserType browser.Type
	Tablet      bool
}

// Run runs the task switch CUJ by opening up ARC and browser windows
// and switching among them using various workflows.
func Run(ctx context.Context, s *testing.State) {
	const taskSwitchingDuration = 5 * time.Minute

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		s.Fatal("Failed to capture device snapshot: ", err)
	}

	testParam := s.Param().(TaskSwitchTest)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	a := s.FixtValue().(cuj.FixtureData).ARC

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, testParam.BrowserType)
	if err != nil {
		s.Fatal("Failed to setup Chrome: ", err)
	}
	defer closeBrowser(closeCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Falied to connect to browser test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, testParam.Tablet)
	if err != nil {
		s.Fatalf("Failed to ensure the tablet mode state [%t]: %v", testParam.Tablet, err)
	}
	defer cleanup(closeCtx)

	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kw.Close(ctx)

	// Create a virtual mouse for clamshell tests.
	var mw *input.MouseEventWriter
	if !testParam.Tablet {
		mw, err = input.Mouse(ctx)
		if err != nil {
			s.Fatal("Failed to create a mouse: ", err)
		}
		defer mw.Close(ctx)
	}

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}

	ac := uiauto.New(tconn)

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, a, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to the recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	// Take a screenshot every 2 minutes up to a maximum of 5
	// screenshots, to try to capture at least 2 screenshots in each
	// of the task switching workflows.
	if err := recorder.AddScreenshotRecorder(ctx, 2*time.Minute, 5); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	topRow, err := input.KeyboardTopRowLayout(ctx, kw)
	if err != nil {
		s.Fatal("Failed to obtain the top-row layout: ", err)
	}

	setOverviewModeWithKeyboard := func(ctx context.Context) error {
		if err := kw.Accel(ctx, topRow.SelectTask); err != nil {
			return errors.Wrap(err, "failed to hit overview key")
		}
		if err := ash.WaitForOverviewState(ctx, tconn, ash.Shown, 30*time.Second); err != nil {
			return errors.Wrap(err, "failed to wait for overview state")
		}

		// Add a stabilization delay after entering overview mode to
		// mitigate Lacros crashes on lower-end devices. This delay
		// ensures that the previews are properly loaded before we
		// interact with the windows in overview mode.
		if err := ac.WithTimeout(5*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
			s.Log("Failed to wait for overview stabilization: ", err)
		}
		return nil
	}

	setOverviewMode := setOverviewModeWithKeyboard
	var tcc *input.TouchCoordConverter
	var stw *input.SingleTouchEventWriter
	var pc pointer.Context
	if testParam.Tablet {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to create a touch controller: ", err)
		}
		defer pc.Close(ctx)

		var tsew *input.TouchscreenEventWriter
		if tsew, tcc, err = touch.NewTouchscreenAndConverter(ctx, tconn); err != nil {
			s.Fatal("Failed to access the touchscreen: ", err)
		}
		defer tsew.Close(ctx)

		if stw, err = tsew.NewSingleTouchWriter(); err != nil {
			s.Fatal("Failed to create a single touch writer: ", err)
		}
		defer stw.Close()

		// Overview mode on tablet scrolls horizontally, limiting the number of windows
		// visible on the screen at a given time. Thus, after showing the overview
		// mode, horizontally scroll the window to the right, so the least recently
		// used window is visible on the screen.
		var startX, startY, endX, endY input.TouchCoord
		startX, startY, endX, endY = tsew.Width()-1, tsew.Height()/2, 0, tsew.Height()/2
		setOverviewMode = func(ctx context.Context) error {
			// Press the overview button to emulate an external keyboard,
			// due to the flakiness of swiping to overview mode on
			// lower-end devices.
			if err := setOverviewModeWithKeyboard(ctx); err != nil {
				return err
			}

			if err := stw.Swipe(ctx, startX, startY, endX, endY, 2*time.Second); err != nil {
				return errors.Wrap(err, "failed to swipe horizontally in overview mode")
			}

			if err := stw.End(); err != nil {
				return errors.Wrap(err, "failed to end swipe animation")
			}
			return nil
		}
	} else {
		pc = pointer.NewMouse(tconn)
		defer pc.Close(ctx)
	}

	defer ash.CloseAllWindows(closeCtx, tconn)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to set up ARC and Play Store: ", err)
	}
	defer func(ctx context.Context) {
		if err := d.Close(ctx); err != nil {
			s.Log("Failed closing UI Automator: ", err)
		}
		a.DumpUIHierarchyOnError(ctx, s.OutDir(), s.HasError)
	}(closeCtx)

	pwaOpened := false
	defer func(ctx context.Context) {
		// The faillog captures the errors that occur before opening PWA.
		// Errors occur after the opening PWA will be captured by the other faillog.
		if !pwaOpened {
			faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump_before_PWA_opened")
		}
	}(closeCtx)

	s.Log("Installing packages")
	packages := getPackages(ctx, tconn, d)
	if err := installPackages(ctx, tconn, a, d, packages); err != nil {
		s.Fatal("Failed to install packages: ", err)
	}

	// Launch packages before launching Chrome tabs, to mitigate
	// flakiness when opening applications. When a lot of tabs are
	// open, sometimes the launcher does not stabilize within the
	// required timeout.
	s.Log("Launching packages")
	numAppWindows, err := launchPackages(ctx, tconn, kw, ac, packages)
	if err != nil {
		s.Fatal("Failed to launch apps: ", err)
	}

	s.Log("Opening Chrome Tabs")
	numBrowserWindows, err := openChromeTabs(ctx, tconn, bTconn, br, testParam.BrowserType, testParam.Tablet)
	if err != nil {
		s.Fatal("Failed to open Chrome tabs: ", err)
	}

	s.Log("Opening PWA")
	cleanupPWA, err := openPWA(ctx, cr, tconn, br)
	if err != nil {
		s.Fatal("Failed to open PWA: ", err)
	}
	defer cleanupPWA(closeCtx)
	pwaOpened = true

	// Increase the count of app windows, to include the PWA that
	// was opened in openPWA.
	numAppWindows++

	// cleanupPWA opens the os settings and blocks the UI tree and screenshot of the error.
	// Dump UI tree and screenshot before the cleanupPWA function.
	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Get a list of metrics to collect for each test phase.
	ashMetrics, browserMetrics := cujrecorder.GetShortenedPerformanceMetrics()

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		recorder.Annotate(ctx, "Open_Aquarium")
		// Open another Chrome tab so we can save the tab connection.
		// We will use this tab connection to navigate away from the
		// page to ensure collection of
		// PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		extraURL := "https://webglsamples.org/aquarium/aquarium.html?numFish=1000"
		extraTab, err := cuj.NewTabByURL(ctx, br, true, extraURL)
		if err != nil {
			return err
		}
		numBrowserWindows++
		numWindows := numAppWindows + numBrowserWindows

		recorder.Annotate(ctx, "Initialize_task_switching_workflows")
		// Initialize task switch workflows only after launching Chrome
		// tabs and applications, because switching by Hotseat requires
		// knowing the bounds of the icons of the open windows.
		taskSwitchers := []taskSwitchWorkflow{
			initializeSwitchTaskByOverviewMode(ctx, tconn, pc, setOverviewMode),
		}
		if testParam.Tablet {
			switchTaskByHotseat, err := initializeSwitchTaskByHotseat(ctx, tconn, stw, tcc, pc, ac, numWindows, numBrowserWindows)
			if err != nil {
				return errors.Wrap(err, "failed to initialize switching task by hotseat")
			}
			taskSwitchers = append(taskSwitchers, *switchTaskByHotseat)
		} else {
			taskSwitchers = append(taskSwitchers, initializeSwitchTaskByAltTab(ctx, kw, numWindows))
		}

		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get window list")
		}
		initialWindows := ws

		for _, taskSwitcher := range taskSwitchers {
			recorder.Annotate(ctx, "Switch_windows_by_"+taskSwitcher.name)

			stopSnapshot, err := recorder.StartSnapshot(ctx, taskSwitcher.name, ashMetrics, browserMetrics)
			if err != nil {
				return errors.Wrapf(err, "failed to start snapshot for %s", taskSwitcher.name)
			}

			s.Log(taskSwitcher.description)
			cycles := 0
			for endTime := time.Now().Add(taskSwitchingDuration); time.Now().Before(endTime); {
				if taskSwitcher.recordTrace {
					if cycles == 0 {
						// See go/trace-in-cuj-tests about rules for tracing.
						if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
							return errors.Wrap(err, "failed to start tracing")
						}
					} else if cycles == numWindows {
						if err := recorder.StopTracing(ctx); err != nil {
							return errors.Wrap(err, "failed to stop tracing")
						}
					}
				}
				if err := taskSwitcher.run(ctx); err != nil {
					return errors.Wrapf(err, "failed to switch to next window using %s", taskSwitcher.name)
				}

				activeWindow, err := ash.GetActiveWindow(ctx, tconn)
				if err != nil {
					return errors.Wrap(err, "failed to find active window")
				}
				if err := ash.WaitWindowFinishAnimating(ctx, tconn, activeWindow.ID); err != nil {
					return errors.Wrap(err, "failed to wait for window to finish animating")
				}

				// GoBigSleepLint: Wait a few seconds to let the page load.
				// Wait a fixed amount of time to keep consistency in task
				// switches among different devices.
				if err := testing.Sleep(ctx, 3*time.Second); err != nil {
					return errors.Wrap(err, "failed to sleep")
				}

				if !testParam.Tablet {
					// Move mouse to center of window to ensure we are
					// scrolling on the currently active window.
					if err := mouse.Move(tconn, activeWindow.BoundsInRoot.CenterPoint(), 500*time.Millisecond)(ctx); err != nil {
						return errors.Wrap(err, "failed to move mouse to center of window")
					}
				}

				// Try to scroll down and up by pressing the down and up
				// arrow key. This gives us some input latency metrics
				// while delaying between each task switch. This also
				// helps increase memory pressure, because it forces Chrome
				// to load more of the page.
				for _, key := range []string{"Down", "Up"} {
					if err := inputsimulations.RepeatKeyPress(ctx, kw, key, 100*time.Millisecond, 3); err != nil {
						return errors.Wrapf(err, "failed to repeatedly press %q in between task switches", key)
					}

					if testParam.Tablet {
						// Since tablets are unable to scroll with the mouse,
						// scroll again with the keyboard. Avoid swiping on
						// the screen, to limit unintentionally tapping on
						// links within each window.
						if err := inputsimulations.RepeatKeyPress(ctx, kw, key, 50*time.Millisecond, 20); err != nil {
							return errors.Wrapf(err, "failed to repeatedly and rapidly press %q in between task switches", key)
						}
					} else {
						if err := inputsimulations.RepeatMouseScroll(ctx, mw, key == "Down", 50*time.Millisecond, 20); err != nil {
							return errors.Wrapf(err, "failed to repeatedly mouse scroll %s", key)
						}
					}
				}

				// GoBigSleepLint: Give a fixed delay between scrolling and dragging,
				// to limit action overlap.
				if err := testing.Sleep(ctx, time.Second); err != nil {
					return errors.Wrap(err, "failed to sleep")
				}

				// If we are on the Chromium window, drag the
				// mouse to the left and right of the screen, to ensure
				// we collect mouse drag input latency.
				if strings.Contains(activeWindow.Title, "Chromium") {
					if err := inputsimulations.RunDragMouseCycle(ctx, tconn, info); err != nil {
						return errors.Wrap(err, "failed to run drag mouse cycle")
					}
				}

				cycles++
			}
			s.Logf("Switched task by %s %d times", taskSwitcher.name, cycles)

			if err := stopSnapshot(ctx); err != nil {
				return errors.Wrapf(err, "failed to stop snapshot for %s", taskSwitcher.name)
			}

			// Ensure the right number of windows are still opened.
			if ws, err := ash.GetAllWindows(ctx, tconn); len(ws) != numWindows {
				// Print out two window list details when window counts are misaligned.
				windowMismatchMsg := cuj.LogWindowMismatch(ctx, initialWindows, ws)
				return errors.Wrapf(err, "unexpected number of windows open after launching ARC applications, got: %d, expected: %d; %s", len(ws), numWindows, windowMismatchMsg)
			}

			// Wait for any animations from the previous workflow to
			// successfully finish. Continue to the next workflow if
			// stabilization fails, because sometimes the visible
			// window does not stabilize in a timely manner.
			if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
				s.Logf("Failed to wait for the window to stabilize after running workflow %s: %v", taskSwitcher.name, err)
			}
		}

		// Find the 'WebGL Aquarium' window and shifts focus to it.
		w, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "WebGL Aquarium")
		if err != nil {
			return errors.Wrap(err, "could not find window with matching title")
		}
		if err := w.ActivateWindow(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to activate 'WebGL Aquarium' window")
		}
		// GoBigSleepLint: Wait a few seconds to let the page load.
		// Wait a fixed amount of time to keep consistency among
		// different devices.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		if err := uiauto.Retry(3, func(ctx context.Context) error {
			// Test the tab connection and reconnect if needed. This is necessary for
			// discarded tabs due to OOM issue.
			isAlive, err := extraTab.IsAlive(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to check if extraTab is still alive")
			}
			if !isAlive {
				if err := extraTab.Reconnect(ctx, br); err != nil {
					return errors.Wrap(err, "cdp connection is invalid and failed to reconnect")
				}
			}
			return nil
		})(ctx); err != nil {
			return errors.Wrap(err, "failed to check the extraTab connection and reconnect")
		}
		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := extraTab.Conn.Navigate(ctx, "chrome://version"); err != nil {
			return errors.Wrap(err, "failed to navigate to chrome://version")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := recorder.SaveHistograms(s.OutDir()); err != nil {
		s.Error("Failed to save histogram raw data: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
}
