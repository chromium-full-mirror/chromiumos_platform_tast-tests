// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package docscuj contains helper util and test code for DocsCUJ.
package docscuj

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParam is the test parameters for DocsCUJ.
type TestParam struct {
	BounceKeysEnabled bool
}

// Run opens up a new Google Doc, and types paragraphs in multiple
// languages, speeds, and styles, to test the Google Docs performance.
func Run(ctx context.Context, cr *chrome.Chrome, testParam TestParam, outDir, systemTraceConfigPath, testName string, cujRecorderOptions cujrecorder.RecorderOptions) (pv *perf.Values, retErr error) {
	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return nil, errors.Wrap(err, "failed to capture device snapshot")
	}

	// Set up an about:blank page, so that we can use the given
	// tab conn to navigate to Google Docs within the recorder.
	conn, err := cr.NewConn(ctx, chrome.BlankURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup Chrome")
	}
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API connection")
	}

	ac := uiauto.New(tconn)

	if testParam.BounceKeysEnabled {
		cleanupBounceKeys, err := a11y.EnsureBounceKeysEnabled(ctx, tconn, true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to enable Bounce keys setting")
		}
		defer func() {
			if err := cleanupBounceKeys(closeCtx); err != nil {
				testing.ContextLog(closeCtx, "Failed to restore Bounce keys setting during cleanup: ", err)
			}
		}()
	}

	recorder, err := cujrecorder.NewRecorder(ctx, tconn, cr, nil, cujRecorderOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a recorder")
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(); err != nil {
		return nil, errors.Wrap(err, "failed to add common metrics to the recorder")
	}

	// Add an empty screenshot recorder.
	if err := recorder.AddScreenshotRecorder(ctx, 0, 0); err != nil {
		testing.ContextLog(ctx, "Failed to add screenshot recorder: ", err)
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	testing.ContextLogf(ctx, "Is in tablet-mode: %t", inTabletMode)
	if err != nil {
		return nil, errors.Wrap(err, "failed to detect if device is in tablet-mode or not")
	}

	var pc pointer.Context
	if inTabletMode {
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create a touch controller")
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer pc.Close(ctx)

	mw, err := input.Mouse(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a mouse")
	}
	defer mw.Close(ctx)

	kw, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a keyboard")
	}
	defer kw.Close(ctx)

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the primary display info")
	}

	tabChecker, err := cuj.NewTabCrashChecker(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create TabCrashChecker")
	}

	// Install the Google Docs offline extension to ensure that the
	// test continues to run despite any network difficulties.
	if err := cuj.EnsureDocsOfflineEnabled(ctx, cr); err != nil {
		return nil, errors.Wrap(err, "failed to enable Docs offline support")
	}

	// paragraphs will be all the paragraphs we will type during the test.
	paragraphs := getParagraphs(pc, cr, tconn, conn, ac, kw)

	// waitUntilStable waits until the root node is stable. This
	// is used for controlled delays during the test, so that the
	// Google Docs gets time to settle down before continuing with
	// future actions. This function is explicitly supposed to be
	// an action that always returns a nil error.
	waitUntilStableAndLogError := func(ctx context.Context) error {
		if err := ac.WithTimeout(10*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
			testing.ContextLog(ctx, "Root hasn't stabilized yet, continuing anyway: ", err)
		}
		return nil
	}

	// Shorten the context further so that we still have time for other cleanup,
	// even the IME removal in the for loop doesn't take too much time.
	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Before typing any paragraph, install all required languages.
	for _, p := range paragraphs {
		if err := p.language.Install(tconn)(ctx); err != nil {
			return nil, errors.Wrapf(err, "failed to install %s", p.language.Name)
		}

		if p.language != ime.DefaultInputMethod {
			defer action.Combine(
				fmt.Sprintf("remove %v", p.language),
				p.language.Remove(tconn),
				p.language.WaitUntilRemoved(tconn),
			)(cleanupCtx)
		}
	}
	defer ime.DefaultInputMethod.Activate(tconn)(cleanupCtx)

	var cleanUpDoc bool
	var docsHref string
	// Shorten the context to cleanup document.
	// Some low-end devices take a long time to delete docs, so extend
	// timeout to one minute.
	cleanUpDocCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if cleanUpDoc && docsHref != "" {
			if err := googledocs.DeleteDocWithURL(tconn, cr, docsHref)(ctx); err != nil {
				faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return true }, cr, "cleanup_doc")
				testing.ContextLog(ctx, "Failed to delete doc: ", err)
			}
		}
	}(cleanUpDocCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, outDir, func() bool { return retErr != nil }, cr, "failure")

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		recorder.Annotate(ctx, "Open_new_Google_Doc")

		const docsURL = "https://docs.new"

		if err := conn.Navigate(ctx, docsURL); err != nil {
			return errors.Wrapf(err, "failed to navigate to %q", docsURL)
		}
		// Give the Google Docs window 30 seconds to settle down and
		// finish loading everything. However, don't fail if it can't
		// settle in that time frame, because the tab should be ready
		// enough to proceed.
		if err := webutil.WaitForQuiescence(ctx, conn, 30*time.Second); err != nil {
			testing.ContextLog(ctx, "Failed to wait for the tab to quiesce")
		}

		ws, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, "Google Docs")
		if err != nil {
			return errors.Wrap(err, "failed to find Docs window")
		}

		if err := conn.Eval(ctx, "window.location.href", &docsHref); err != nil {
			return errors.Wrap(err, "failed to get Docs URL")
		}
		cleanUpDoc = true

		recorder.Annotate(ctx, "Maximize_window")
		if err := ash.SetWindowStateAndWait(ctx, tconn, ws.ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to set window state to maximized")
		}

		// Wait for root web area to exist to ensure the UI elements are captured.
		if err := ac.WaitUntilExists(googledocs.DocsWebArea)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for docs root web area")
		}

		// Google Docs sometimes pops up a prompt to notice users
		// if there're new features. Dismiss prompt if it exist.
		gotItButton := nodewith.Name("Got it").Role(role.Button)
		if err := uiauto.IfSuccessThen(ac.Exists(gotItButton), ac.DoDefault(gotItButton))(ctx); err != nil {
			return errors.Wrap(err, "failed to click 'Got it' prompt")
		}

		// Close any potential security alert that pops up.
		if err := cuj.DismissCriticalSecurityAlert(ctx, tconn, conn); err != nil {
			return errors.Wrap(err, "failed to dismiss Critical Security Alert")
		}

		// Before the first UI interaction, check if the renderer crashed.
		if err := tabChecker.Check(ctx); err != nil {
			return errors.Wrap(err, "tab renderer crashed")
		}

		// Include the time in the title to make the document more
		// unique, in case we ever wanted to refer back to it.
		if err := googledocs.UpdateTitle(ctx, pc, ac, kw, fmt.Sprintf("%s %s", testName, time.Now())); err != nil {
			return errors.Wrap(err, "failed to update Google Doc title")
		}

		// Running the drag mouse cycle is an easy way to both get
		// mouse drag metrics and focus the center of the page. Without
		// this, the Google Doc body would not be focused and ready for
		// text input.
		if err := inputsimulations.RunDragMouseCycle(ctx, tconn, info); err != nil {
			return err
		}

		for pIndex, p := range paragraphs {
			// Before typing the next paragraph, ensure that the Docs
			// tab hasn't crashed.
			if err := tabChecker.Check(ctx); err != nil {
				return errors.Wrap(err, "tab renderer crashed")
			}

			paragraphDescription := strings.ReplaceAll(p.description, " ", "_")

			// Use a mixture of key/mouse/touch actions to type a
			// bolded header for every paragraph. These titles are in
			// English, regardless of the language being typed.
			recorder.Annotate(ctx, fmt.Sprintf("Start_%s_paragraph", paragraphDescription))
			if err := action.Combine(
				fmt.Sprintf("type bolded header for the %s paragraph", p.description),

				// Default to English for the headers.
				ime.DefaultInputMethod.Activate(tconn),

				// Ctrl+Alt+1 is the shortcut for activating Heading 1.
				kw.AccelAction("Ctrl+Alt+1"),

				// Use mouse/touch input for bolding the title header.
				googledocs.UpdateTextStyleAction(ctx, pc, ac, googledocs.Bold),

				// By hitting "Enter" after typing the title, the text
				// reverts to the default "Normal" style. Thus, we do
				// not have to explicitly undo the bolding.
				typeParagraphAction(ctx, kw, p.description+"[Enter]", fastTypingSpeed),
			)(ctx); err != nil {
				return err
			}
			recorder.Annotate(ctx, fmt.Sprintf("Completed_%s_paragraph", paragraphDescription))

			// Some paragraphs require some additional setup. For
			// example, when writing a paragraph in pageless mode,
			// we need to enable pageless.
			if p.setUp != nil {
				if err := p.setUp(ctx); err != nil {
					return errors.Wrapf(err, "failed to set up %s paragraph", p.description)
				}
			}

			// Activate the language for the corresponding paragraph.
			// All languages have been installed by this point in the test.
			if err := p.language.Activate(tconn)(ctx); err != nil {
				return errors.Wrapf(err, "failed to activate %s", p.language.Name)
			}

			// See go/trace-in-cuj-tests about rules for tracing.
			if p.tracingCfg != "" {
				traceName := fmt.Sprintf("paragraph%d.data", pIndex)
				if err := recorder.StartTracingWithName(ctx, outDir, traceName, systemTraceConfigPath); err != nil {
					return errors.Wrap(err, "failed to start tracing")
				}
			}

			// If |customTypeAction| is not supplied, type the paragraph
			// at |mediumTypingSpeed|.
			if p.customTypeAction != nil {
				if err := p.customTypeAction(ctx); err != nil {
					return errors.Wrap(err, "failed to type with custom action")
				}
			} else if err := typeParagraphAction(ctx, kw, p.body+"[Enter]", mediumTypingSpeed)(ctx); err != nil {
				return errors.Wrap(err, "failed to type paragraph")
			}

			if p.tracingCfg != "" {
				if err := recorder.StopTracing(ctx); err != nil {
					return errors.Wrap(err, "failed to stop tracing")
				}
			}

			// Take a screenshot after each paragraph has been written.
			recorder.CustomScreenshot(ctx)

			if p.cleanUp != nil {
				if err := p.cleanUp(ctx); err != nil {
					return errors.Wrap(err, "failed to clean up paragraph setup")
				}
			}

			waitUntilStableAndLogError(ctx)

			// Interact with the Ash UI to get Ash smoothness metrics.
			if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
				return errors.Wrap(err, "failed to do Ash workflows")
			}

			// Drag the mouse to highlight some text on the page.
			if err := inputsimulations.RunDragMouseCycle(ctx, tconn, info); err != nil {
				return errors.Wrap(err, "failed to run drag mouse cycle")
			}

			// Scroll the document down and up to get mouse wheel metrics.
			for _, scrollDown := range []bool{true, false} {
				if err := inputsimulations.RepeatMouseScroll(ctx, mw, scrollDown, 50*time.Millisecond, 20); err != nil {
					return errors.Wrap(err, "failed to scroll")
				}
			}
			waitUntilStableAndLogError(ctx)

			// To give each paragraph type a fair shot, delete what was
			// just written.
			if err := action.Combine(
				"delete text with Ctrl+A and Backspace",
				kw.AccelAction("Ctrl+A"),
				waitUntilStableAndLogError,
				kw.AccelAction("Backspace"),
				waitUntilStableAndLogError,
			)(ctx); err != nil {
				return err
			}
		}

		// Wait until the document has completed saving before
		// navigating away, to prevent the "Changes you made may not
		// be saved" popup. Add a fixed time to wait to ensure a
		// minimum wait time.
		if err := action.Combine(
			"wait a fixed amount of time, and then wait for 'Saving...' to be gone",
			action.Sleep(5*time.Second),
			ac.WaitUntilGone(nodewith.NameStartingWith("Saving")),
		)(ctx); err != nil {
			return err
		}

		// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
		if err := conn.Navigate(ctx, "chrome://version"); err != nil {
			return errors.Wrap(err, "failed to navigate to chrome://version")
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "failed to conduct the recorder task")
	}

	if err := recorder.Record(ctx, pv); err != nil {
		return nil, errors.Wrap(err, "failed to report")
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		return nil, errors.Wrap(err, "failed to save histogram raw data")
	}
	if err := pv.Save(outDir); err != nil {
		return nil, errors.Wrap(err, "failed to store values")
	}
	return pv, nil
}
