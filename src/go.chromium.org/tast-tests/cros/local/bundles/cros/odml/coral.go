// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package odml

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/odml/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var odmlDaemon = &hwsec.DaemonInfo{
	Name:       "odml",
	DaemonName: "odmld",
	HasDBus:    false,
}

func init() {
	testing.AddTest(&testing.Test{
		Func: Coral,
		Desc: "Checks the coral feature is functioning correctly",
		Contacts: []string{
			"hcyang@google.com",
			"cros-odml-foundations-eng@google.com",
		},
		BugComponent: "b:1445284",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model("navi")),
		Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			Timeout: 3 * time.Minute,
			// Whether the test should do performance setup.
			Val:     false,
			Fixture: fixture.CoralEnabled,
		}, {
			Name:    "japanese",
			Timeout: 3 * time.Minute,
			// Whether the test should do performance setup.
			Val:     false,
			Fixture: fixture.CoralEnabledJapanese,
		}, {
			// Test case that records performance. This might run much longer than validating the feature itself.
			Name:    "perf",
			Timeout: 7 * time.Minute,
			// Whether the test should do performance setup.
			Val:     true,
			Fixture: fixture.CoralEnabled,
		}},
	})
}

func Coral(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	perfSetup := s.Param().(bool)
	if perfSetup {
		// Restart odmld to ensure that everything starts fresh; nothing is cached
		// or preloaded. This help us better capture the performance metrics.
		cmdRunner := hwseclocal.NewCmdRunner()
		daemonController := hwsec.NewDaemonController(cmdRunner)
		if err := daemonController.Restart(ctx, odmlDaemon); err != nil {
			s.Fatal("Failed to restart odmld: ", err)
		}

		recorder, err := cujrecorder.NewRecorder(ctx, tconn, cr, nil, cujrecorder.RecorderOptions{CooldownBeforeRun: true, CheckInterval: time.Second})
		if err != nil {
			s.Fatal("Failed to create the CUJ recorder: ", err)
		}
		defer recorder.Close(cleanupCtx)

		if err := recorder.AddCommonMetrics(); err != nil {
			s.Fatal("Failed to add common metrics to recorder: ", err)
		}

		if err := recorder.Run(ctx, func(ctx context.Context) error {
			if err := runCoralFlow(ctx, cr, tconn, 5, 5); err != nil {
				return err
			}
			// This help ensure that the memory usage is back to normal before the feature is triggered.
			if err := waitUntilModelUnloaded(ctx); err != nil {
				s.Fatal("Failed to wait until model unloaded: ", err)
			}
			return nil
		}); err != nil {
			s.Fatal("Failed to run test with CUJ recorder: ", err)
		}
		pv := perf.NewValues()
		if err := recorder.Record(ctx, pv); err != nil {
			s.Fatal("Failed to record the performance data: ", err)
		}
		if err := pv.Save(s.OutDir()); err != nil {
			s.Fatal("Failed to save the performance data: ", err)
		}
	} else {
		if err := runCoralFlow(ctx, cr, tconn, 0, 1); err != nil {
			s.Fatal("Failed to run coral workflow: ", err)
		}
	}
}

func runCoralFlow(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, tabPresses, contextMenuClicks int) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Use built-in sites so the test is more stable and won't be broken by
	// changes in third-party websites. These sites form a coral group so a coral
	// chip should show up.
	for _, url := range []string{"chrome://version", "chrome://device-log", "chrome://histograms", "chrome://settings"} {
		conn, err := cr.NewConn(ctx, url)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", url)
		}
		defer conn.Close()
	}

	// Activate and dismiss overview repeatedly until the coral chip shows.
	// Unfortunately we don't have a good way to ensure that the coral ship
	// is guaranteed to be shown on the first trigger. This is because overview
	// mode has a 1-second timeout for showing the underlying chips, and if the
	// backend service is still initializing or does not have enough cache, the
	// timeout could be reached and the chip won't show. This is considered as
	// expected behavior for the coral feature, so here we just poll until it
	// shows up.
	birchButtonClassName := "CoralChipButton"
	birchButtonAttrs := map[string]interface{}{
		"className": birchButtonClassName,
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
			return errors.Wrap(err, "failed to set overview mode")
		}
		// If the chip is going to show, it should appear within 3 seconds.
		chipRoot, findErr := a11y.FindWithTimeout(ctx, tconn, a11y.FindParams{Attributes: birchButtonAttrs}, 3*time.Second)
		if findErr != nil {
			if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
				return errors.Wrap(err, "failed to dismiss overview mode")
			}
			return findErr
		}
		chipRoot.Release(cleanupCtx)
		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for coral chip to show up in overview")
	}

	// The coral chip is now shown. Coral chip's title field is asynchronously
	// updated and it will show up after the backend generates the title.
	// We can not use WaitUntilDescendantExists here because there is no simple
	// condition to use as FindParams. The coral chip has a title field and a
	// subtitle field which have same attributes, so the best way is to poll until
	// the chip contains 2 children, meaning that the title has been shown.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		root, err := a11y.Root(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to find root")
		}
		defer root.Release(cleanupCtx)
		chipRoot, err := root.Descendant(ctx, a11y.FindParams{Attributes: birchButtonAttrs})
		if err != nil {
			return errors.Wrap(err, "failed to find birch chip button")
		}
		defer chipRoot.Release(cleanupCtx)
		boxAttrs := map[string]interface{}{
			"className": "BoxLayoutView",
		}
		box, err := chipRoot.Descendant(ctx, a11y.FindParams{Attributes: boxAttrs})
		if err != nil {
			return errors.Wrap(err, "failed to find birch chip box")
		}
		defer box.Release(cleanupCtx)
		children, err := box.Children(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to find children of birch chip box")
		}
		defer children.Release(ctx)

		// 1 child: Only the subtitle field. Title is still generating.
		// 2 children: The title is generated successfully.
		// Other number of children: Something is wrong, or the UI layout has changed.
		if len(children) == 2 {
			return nil
		} else if len(children) == 1 {
			return errors.New("Chip title is still generating")
		}
		return errors.Wrapf(err, "unexpected size of children in the birch chip: %v", len(children))
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: 500 * time.Millisecond}); err != nil {
		return errors.Wrap(err, "failed to wait for title to be generated in coral chip")
	}

	ui := uiauto.New(tconn)
	birchButton := nodewith.ClassName(birchButtonClassName)
	pc := pointer.NewMouse(tconn)
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create keyboard")
	}

	// Simulate some tab events for keyboard perf recording.
	if err := uiauto.Repeat(tabPresses, uiauto.Combine("Press tab and sleep for 1 second", keyboard.AccelAction("tab"), uiauto.Sleep(time.Second)))(ctx); err != nil {
		return errors.Wrap(err, "failed to press tab in overview")
	}

	if err := uiauto.Repeat(contextMenuClicks, uiauto.Combine("Click context menu",
		pc.MenuClick(birchButton),
		// After right clicking on the chip button, the context menu should show.
		ui.WithTimeout(5*time.Second).WaitUntilExists(nodewith.ClassName("MenuContainer")),
		uiauto.Sleep(time.Second)))(ctx); err != nil {
		return errors.Wrap(err, "failed to trigger context menu")
	}

	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		return errors.Wrap(err, "failed to dismiss overview mode")
	}

	return nil
}

func waitUntilModelUnloaded(ctx context.Context) error {
	// The model will automatically be unloaded after existing overview session.
	// Just wait a few seconds here.
	// GoBigSleepLint: sleep to let odmld unload the model due to inactivity.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}
