// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"math"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	memorymetrics "go.chromium.org/tast-tests/cros/local/memory/metrics"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	videocallURL        = "https://storage.googleapis.com/chromiumos-test-assets-public/power_VideoCall/power_VideoCall.html"
	docsURL             = "http://crospower.page.link/power_VideoCall_doc"
	measurementDuration = 60 * time.Second
	notes               = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. "
)

type powerVideocallParams struct {
	browserType  browser.Type
	collectTrace bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerVideocall,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Runs a video conference and text input window side-by-side with either ash-chrome and lacros-chrome",
		Contacts:     []string{"lacros-team@google.com", "hidehiko@chromium.org"},
		BugComponent: "b:1456869",
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.baserpc.FileSystem"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name:              "lacros",
			Val:               powerVideocallParams{browserType: browser.TypeLacros, collectTrace: false},
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_nightly"},
		}, {
			Name:      "ash",
			Val:       powerVideocallParams{browserType: browser.TypeAsh, collectTrace: false},
			Fixture:   "chromeLoggedIn",
			ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
		}, {
			Name:              "lacros_trace",
			Val:               powerVideocallParams{browserType: browser.TypeLacros, collectTrace: true},
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraData:         []string{tracing.TBMTracedProbesConfigFile},
		}, {
			Name:      "ash_trace",
			Val:       powerVideocallParams{browserType: browser.TypeAsh, collectTrace: true},
			Fixture:   "chromeLoggedIn",
			ExtraData: []string{tracing.TBMTracedProbesConfigFile},
		}},
	})
}

func PowerVideocall(ctx context.Context, s *testing.State) {
	params := s.Param().(powerVideocallParams)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get all windows: ", err)
	}
	for _, w := range ws {
		w.CloseWindow(ctx, tconn)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		ws, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get all windows from ash"))
		}
		if len(ws) > 0 {
			return errors.New("waiting to close all windows")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 2 * time.Minute}); err != nil {
		s.Fatal("Failed to poll to close any excess windows: ", err)
	}

	memBase, err := memorymetrics.NewBaseMemoryStats(ctx, nil)
	if err != nil {
		s.Fatal("Failed to get base zram stats: ", err)
	}

	videoConn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, params.browserType, videocallURL)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer cleanup(ctx)
	defer func() {
		videoConn.CloseTarget(ctx)
		videoConn.Close()
	}()

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get browser test API connection: ", err)
	}

	videoWin, err := ash.FindWindow(ctx, tconn, func(window *ash.Window) bool {
		if params.browserType == browser.TypeAsh {
			return window.WindowType == ash.WindowTypeBrowser
		}
		return window.WindowType == ash.WindowTypeLacros
	})
	if err != nil {
		s.Fatal("Failed to find video window: ", err)
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, videoWin.ID, ash.WindowStateSecondarySnapped); err != nil {
		s.Error("Failed to snap first blank window to the right: ", err)
	}

	pc := pointer.NewMouse(tconn)

	bubble := nodewith.ClassName("PermissionPromptBubbleBaseView").First()
	allow := nodewith.Name("Allow").Role(role.Button).Ancestor(bubble)
	if err := pc.Click(allow)(ctx); err != nil {
		s.Fatal("Failed to click permission bubble: ", err)
	}

	docsConn, err := br.NewConn(ctx, docsURL, browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to open docs window: ", err)
	}
	defer func() {
		docsConn.CloseTarget(ctx)
		docsConn.Close()
	}()

	docsWin, err := ash.FindWindow(ctx, tconn, func(window *ash.Window) bool {
		if params.browserType == browser.TypeAsh {
			if window.WindowType == ash.WindowTypeBrowser {
				return window != videoWin
			}
		} else {
			if window.WindowType == ash.WindowTypeLacros {
				return window != videoWin
			}
		}
		return false
	})
	if err != nil {
		s.Fatal("Failed to find docs window: ", err)
	}

	if err := ash.SetWindowStateAndWait(ctx, tconn, docsWin.ID, ash.WindowStatePrimarySnapped); err != nil {
		s.Error("Failed to snap second blank window to the left: ", err)
	}

	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kw.Close(ctx)

	// Select text input field
	if err := pc.Click(nodewith.Name("Edit here").Role(role.TextField))(ctx); err != nil {
		s.Fatal("Failed to select input field on docs page: ", err)
	}

	var sess *tracing.Session
	if params.collectTrace {
		traceConfigPath := s.DataPath(tracing.TBMTracedProbesConfigFile)
		traceDataPath := filepath.Join(s.OutDir(), "trace.pb")
		sess, err = tracing.StartSession(ctx, traceConfigPath, tracing.WithTraceDataPath(traceDataPath))
		if err != nil {
			s.Fatal("Failed to start tracing: ", err)
		}
		s.Log("Collecting Perfetto trace File at: ", sess.TraceDataPath())

		defer sess.Finalize(ctx)
	}

	histograms, err := metrics.RunAndWaitAll(
		ctx,
		bTconn,
		2*measurementDuration,
		func(ctx context.Context) error {
			end := time.Now()
			for time.Now().Sub(end) < measurementDuration {
				if err := uiauto.Combine(
					"sleep and type",
					action.Sleep(5*time.Second),
					kw.TypeAction(notes),
				)(ctx); err != nil {
					return err
				}
			}
			return nil
		},
		"EventLatency.KeyPressed.TotalLatency",
	)
	if err != nil {
		if params.collectTrace {
			sess.Stop(ctx)
		}
		s.Fatal("Failed to collect metric data: ", err)
	}

	if params.collectTrace {
		if err := sess.Stop(ctx); err != nil {
			s.Fatal("Failed to stop the tracing session: ", err)
		}
	}

	pv := perf.NewValues()

	if err := memorymetrics.LogMemoryStats(ctx, memBase, nil, pv, s.OutDir(), ""); err != nil {
		s.Error("Failed to log memory stats: ", err)
	}

	for _, h := range histograms {
		mean, err := h.Mean()
		if err != nil {
			s.Error("Failed to extract mean from histogram: ", err)
		}
		pv.Set(
			perf.Metric{
				Name:      h.Name + ".mean",
				Unit:      "microseconds",
				Direction: perf.SmallerIsBetter,
			},
			mean,
		)
		totalCount := h.TotalCount()
		sampleNum95 := (totalCount * 95) / 100

		var max int64
		var stdDev float64
		var value95 float64
		var t int64
		for _, b := range h.Buckets {
			midpoint := (float64(b.Min) + float64(b.Max)) / 2.0
			stdDev = stdDev + (float64(b.Count) * (mean - midpoint) * (mean - midpoint))
			max = b.Max
			if t < sampleNum95 {
				if t+b.Count >= sampleNum95 {
					value95 = float64(b.Min) + ((float64(b.Max) - float64(b.Min)) * ((float64(sampleNum95) - float64(t)) / float64(b.Count)))
				}
			}
			t = t + b.Count
		}

		stdDev = math.Sqrt(stdDev / (float64(totalCount) - 1.0))
		pv.Set(
			perf.Metric{
				Name:      h.Name + ".std_dev",
				Unit:      "microseconds",
				Direction: perf.SmallerIsBetter,
			},
			stdDev,
		)

		pv.Set(
			perf.Metric{
				Name:      h.Name + ".percent_95",
				Unit:      "microseconds",
				Direction: perf.SmallerIsBetter,
			},
			value95,
		)

		pv.Set(
			perf.Metric{
				Name:      h.Name + ".max",
				Unit:      "microseconds",
				Direction: perf.SmallerIsBetter,
			},
			float64(max),
		)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
}
