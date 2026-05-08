// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mahi provides Mahi related tests (e.g. power test).
package mahi

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/ui/mahicuj/mahiutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
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
		Func: PowerMetrics,
		Desc: "Collect power metrics of using mahi",
		Contacts: []string{
			"ml-service-team@google.com",
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

	if err := mahiutil.InstallScreenAIDLC(ctx); err != nil {
		s.Fatal("Install DLC failed: ", err)
	}

	ui := uiauto.New(tconn)
	window, _ := ash.GetActiveWindow(ctx, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Unzip the data file and setup test HTTP server
	localHTMLPath, localHTMLFiles, localServer, err := mahiutil.PrepareLocalServer(ctx, s.DataPath(localHTMLZip))
	if err != nil {
		s.Fatal("Failed to prepare local html files: ", err)
	}
	defer os.RemoveAll(localHTMLPath)
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

	if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
		s.Fatal("Failed to clean UI element after summary: ", err)
	}

	params := s.Param().(testParameters)

	if params.expectMahiWidget {
		if err := mahiutil.MaybePassConsentFlow(
			ctx, conn, tconn, window, ui, kb, localServer.URL+"/"+localHTMLFiles[0].Name()); err != nil {
			s.Fatal("Failed to pass the consent flow: ", err)
		}
	}

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

		if err := mahiutil.NavigateToURL(ctx, conn, localServer.URL+"/"+fileName); err != nil {
			return errors.Wrapf(err, "failed to open local html %s", fileName)
		}

		// Do a right click and wait for the context menu / mahi widget card.
		if err := mahiutil.RightClickAndMaybeShowMahiWidget(
			ctx, tconn, window, ui, params.expectMahiWidget); err != nil {
			s.Log("Failed to do a right click: ", err)
			return errors.Wrap(err, "failed to do a right click")
		}

		if !params.doMahiSummary {
			if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
				s.Fatal("Failed to clean UI element after right click: ", err)
			}
			succeedCount++
			return errors.New("Don't do mahi summary, go next URL")
		}

		if err := mahiutil.DoSummary(ctx, ui, false /*expectMockResponse*/); err != nil {
			s.Log("Failed to do a mahi summary: ", err)
			return errors.Wrap(err, "failed to do a mahi summary")
		}

		if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
			s.Fatal("Failed to clean UI element after summary: ", err)
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
