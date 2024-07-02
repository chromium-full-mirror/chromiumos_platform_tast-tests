// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mahi provides Mahi related tests (e.g. power test).
package mahi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	longUITimeout       = 10 * time.Second
	powerMetricInterval = 10 * time.Second
	localHTMLZip        = "mahi_html.zip"
)

type testParameters struct {
	expectMahiWidget bool
	doMahiSummary    bool
	urlCount         int
	interval         time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerMetrics,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect power metrics of using mahi",
		Contacts: []string{
			"ml-service-team@google.com",
			"alanlxl@google.com",
			"chenjih@google.com",
			"thanhdng@google.com",
		},
		BugComponent: "b:1116342",
		Timeout:      20*time.Minute + power.RecorderTimeout,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Params: []testing.Param{
			{
				Name: "mahi_disabled_100_pages",
				Val: testParameters{
					expectMahiWidget: false,
					doMahiSummary:    false,
					urlCount:         100,
					interval:         time.Second * 2,
				},
				Fixture: "powerAsh",
			},
			{
				Name: "mahi_enabled_100_pages",
				Val: testParameters{
					expectMahiWidget: true,
					doMahiSummary:    false,
					urlCount:         100,
					interval:         time.Second * 2,
				},
				Fixture: "powerAshMahi",
			},
			{
				Name: "mahi_enabled_20_pages",
				Val: testParameters{
					expectMahiWidget: true,
					doMahiSummary:    false,
					urlCount:         20,
					interval:         time.Second * 10,
				},
				Fixture: "powerAshMahi",
			},
			{
				Name: "mahi_summary_20_pages",
				Val: testParameters{
					expectMahiWidget: true,
					doMahiSummary:    true,
					urlCount:         20,
					interval:         time.Second * 10,
				},
				Fixture: "powerAshMahi",
			},
		},
		Data: []string{
			localHTMLZip,
		},
	})
}

func PowerMetrics(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Open the browser
	conn, err := cr.NewConn(ctx, "")
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Force install screen-ai dlc.
	if err := dlc.Install(ctx, "screen-ai", ""); err != nil {
		s.Fatal("Install DLC failed: ", err)
	}

	// Ensure screen2x is installed.
	if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{
		Timeout:  2 * time.Minute,
		Interval: 10 * time.Second,
	}); err != nil {
		s.Fatal("Failed to wait for screen-ai dlc to be installed: ", err)
	}

	ui := uiauto.New(tconn)
	window, _ := ash.GetActiveWindow(ctx, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Unzip the local html files.
	localHTMLPath := path.Join(os.TempDir(), "mahi.power_metrics")
	if err := os.MkdirAll(localHTMLPath, 0755); err != nil {
		s.Fatal("Failed to create local html directory: ", err)
	}
	defer os.RemoveAll(localHTMLPath)

	if err := testexec.CommandContext(ctx, "unzip", "-o", s.DataPath(localHTMLZip), "-d", localHTMLPath).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to unzip ", localHTMLZip, " to local html directory: ", err)
	}

	localHTMLFiles, err := os.ReadDir(localHTMLPath)
	if err != nil {
		s.Fatal("Failed to read HTML file list from local html directory: ", err)
	}

	// Setup test HTTP server.
	localServer := httptest.NewServer(http.FileServer(http.Dir(localHTMLPath)))
	defer localServer.Close()

	r := power.NewRecorder(ctx, powerMetricInterval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	// Cool down test device.
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	contextMenu := nodewith.ClassName("SubmenuView").Role("menu")
	summarizeButton := nodewith.Name("Summarize").ClassName("LabelButton")
	compactSummaryButton := nodewith.Name("Help me read this page").ClassName("MahiCondensedMenuButton")
	summaryOutlinesSection := nodewith.ClassName("SummaryOutlinesSection")
	summaryText := nodewith.NameRegex(regexp.MustCompile(`^.{20,}$`)).ClassName("Label").Role("staticText").Ancestor(summaryOutlinesSection)
	mahiErrorStatus := nodewith.ClassName("MahiErrorStatusView")
	mahiCloseButton := nodewith.Name("Close button").ClassName("IconButton")

	cleanUIElement := func() error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			if err := uiauto.Combine("Hide context menu",
				uiauto.IfSuccessThen(ui.Exists(contextMenu), kb.AccelAction("Esc")),
				ui.WaitUntilGone(contextMenu),
			)(ctx); err != nil {
				return errors.Wrap(err, "fail to hide the context menu")
			}

			if err := uiauto.Combine("Hide mahi panel",
				uiauto.IfSuccessThen(ui.Exists(mahiCloseButton), ui.LeftClick(mahiCloseButton)),
				ui.WaitUntilGone(mahiCloseButton),
			)(ctx); err != nil {
				return errors.Wrap(err, "fail to hide the mahi panel")
			}

			return nil
		}, &testing.PollOptions{
			Timeout:  5 * time.Second,
			Interval: time.Second,
		})
	}

	params := s.Param().(testParameters)
	index := 0
	succeedCount := 0
	localHTMLCount := len(localHTMLFiles)

	testing.Poll(ctx, func(ctx context.Context) error {
		if index == params.urlCount {
			s.Log("finished")
			return nil
		}

		fileName := localHTMLFiles[index%localHTMLCount].Name()
		index++

		// Open a html file from local server
		url := localServer.URL + "/" + fileName
		if err := conn.Navigate(ctx, url); err != nil {
			s.Log("Failed to open url: ", err)
			return errors.Wrap(err, "failed to open url")
		}

		if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
			s.Logf("Failed to wait for %q to be loaded and achieve quiescence, err: %q", url, err)
			return errors.Wrap(err, "failed to wait for quiescence")
		}

		// Do a right click and wait for the context menu / mahi widget card.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := mouse.Click(tconn, window.TargetBounds.CenterPoint(), mouse.RightButton)(ctx); err != nil {
				return errors.Wrap(err, "failed to right click")
			}
			if !params.expectMahiWidget {
				return ui.WaitUntilExists(contextMenu)(ctx)
			}
			return ui.WaitUntilAnyExists(summarizeButton, compactSummaryButton)(ctx)
		}, &testing.PollOptions{
			Timeout:  5 * time.Second,
			Interval: time.Second,
		}); err != nil {
			s.Log("Failed to do a right click: ", err)
			return errors.Wrap(err, "failed to do a right click")
		}

		if !params.doMahiSummary {
			if err := cleanUIElement(); err != nil {
				s.Fatal("Failed to clean UI element after right click")
			}
			succeedCount++
			return errors.New("Don't do mahi summary, go next URL")
		}

		if err := uiauto.Combine("Do summary and check the panel exists",
			uiauto.IfSucceedThenElse(ui.Exists(summarizeButton), ui.LeftClick(summarizeButton), ui.LeftClick(compactSummaryButton)),
			ui.WaitUntilExists(mahiCloseButton),
			ui.WaitUntilAnyExists(summaryText, mahiErrorStatus),
		)(ctx); err != nil {
			s.Log("Failed to do a mahi summary: ", err)
			return errors.Wrap(err, "failed to do a mahi summary")
		}

		if err := cleanUIElement(); err != nil {
			s.Fatal("Failed to clean UI element after summary")
		}

		succeedCount++
		return errors.New("not finish yet")
	}, &testing.PollOptions{
		Timeout:  15 * time.Minute,
		Interval: params.interval,
	})

	s.Log("succeedCount = ", succeedCount, " params.urlCount = ", params.urlCount)
	if params.urlCount-succeedCount > 5 {
		s.Error("Got too many failures, failed count = ", params.urlCount-succeedCount)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
