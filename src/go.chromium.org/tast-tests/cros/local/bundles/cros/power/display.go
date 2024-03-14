// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

var validBrightnessMap = map[string]bool{
	"all_brightness": true,
	"default":        true,
	"max":            true,
}

// Interval is the time interval used in power.NewRecorder.
// Not that "Total" is the length a page displays/sleeps, not the total duration of the test.
var timeParams = power.TimeParams{Interval: 5 * time.Second, Total: 1 * time.Minute}
var fastTimeParams = power.TimeParams{Interval: 5 * time.Second, Total: 20 * time.Second}

// Commonly used pages for most subtests.
var pages = []string{"checker1", "black", "white", "red", "green", "blue"}

// Pages used for testing with all 16 nonlinear brightnesses.
var pagesForBrightnessTest = []string{"checker1", "black", "white"}

// Pages used for CABC tests. CABC or Content adaptive backlight control is the
// technique that will adjust panel backlight brightness according to the content.
// For details: go/cros-cabc-test. The link is about the autotest, but the concept
// is the same.
var pagesForCABCTest = []string{
	"white", "black",
	"white", "grey10",
	"white", "grey20",
	"white", "grey30",
	"white", "grey40",
	"white", "grey50",
	"white", "checker1",
	"white", "checker90",
	"white", "checker80",
	"white", "checker70",
	"white", "checker60",
	"white", "checker50",
}

// DisplayParams used for displaying each page in default brightness
// for DisplayTimeParams.Total length.
var defaultDisplayParams = power.DisplayParams{
	Brightness:        "default",
	DisplayTimeParams: timeParams,
	Pages:             pages,
}

// DisplayParams used for the shorter/fast version of the default test.
var fastDisplayParams = power.DisplayParams{
	Brightness:        "default",
	DisplayTimeParams: fastTimeParams,
	Pages:             pages,
}

// DisplayParams used for displaying each page in max brightness
// for DisplayTimeParams.Total length.
var maxDisplayParams = power.DisplayParams{
	Brightness:        "max",
	DisplayTimeParams: timeParams,
	Pages:             pages,
}

// DisplayParams used for displaying each page in ChromeOS's 16 steps for brightness in the UI
// for DisplayTimeParams.Total length.
var brightnessDisplayParams = power.DisplayParams{
	Brightness:        "all_brightness",
	DisplayTimeParams: fastTimeParams,
	Pages:             pagesForBrightnessTest,
}

// DisplayParams used for settings that usually utilize panel CABC algorithm
// under default brightness. Each page will be displayed in default brightness.
var defaultCABCDisplayParams = power.DisplayParams{
	Brightness:        "default",
	DisplayTimeParams: fastTimeParams,
	Pages:             pagesForCABCTest,
}

// DisplayParams used for settings that usually utilize panel CABC algorithm
// under default brightness. Each page will be displayed in max brightness.
var maxCABCDisplayParams = power.DisplayParams{
	Brightness:        "max",
	DisplayTimeParams: fastTimeParams,
	Pages:             pagesForCABCTest,
}

// Commonly used page data for most subtests.
var pageData = []string{
	"display/html/checker1.html",
	"display/html/black.html",
	"display/html/white.html",
	"display/html/red.html",
	"display/html/green.html",
	"display/html/blue.html",
}

var pageDataForBrightnessTest = []string{
	"display/html/checker1.html",
	"display/html/black.html",
	"display/html/white.html",
}

var pageDataForCABCTest = []string{
	"display/html/white.html", "display/html/black.html",
	"display/html/white.html", "display/html/grey10.html",
	"display/html/white.html", "display/html/grey20.html",
	"display/html/white.html", "display/html/grey30.html",
	"display/html/white.html", "display/html/grey40.html",
	"display/html/white.html", "display/html/grey50.html",
	"display/html/white.html", "display/html/checker1.html",
	"display/html/white.html", "display/html/checker90.html",
	"display/html/white.html", "display/html/checker80.html",
	"display/html/white.html", "display/html/checker70.html",
	"display/html/white.html", "display/html/checker60.html",
	"display/html/white.html", "display/html/checker50.html",
}

const tmpPath = "/tmp"

func init() {
	testing.AddTest(&testing.Test{
		Func:         Display,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics displaying different static pages with different screen brightness level",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "jingmuli@google.com"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:      "default_ash",
			Fixture:   "powerAsh",
			Timeout:   10*time.Minute + power.RecorderTimeout,
			Val:       defaultDisplayParams,
			ExtraData: pageData,
		}, {
			Name:      "fast_ash",
			Fixture:   "powerAsh",
			Timeout:   5*time.Minute + power.RecorderTimeout,
			Val:       fastDisplayParams,
			ExtraData: pageData,
		}, {
			Name:      "max_ash",
			Fixture:   "powerAsh",
			Timeout:   10*time.Minute + power.RecorderTimeout,
			Val:       maxDisplayParams,
			ExtraData: pageData,
		}, {
			Name:      "all_brightness_ash",
			Fixture:   "powerAsh",
			Timeout:   1*time.Hour + power.RecorderTimeout,
			Val:       brightnessDisplayParams,
			ExtraData: pageDataForBrightnessTest,
		}, {
			Name:      "default_cabc_ash",
			Fixture:   "powerAsh",
			Timeout:   10*time.Minute + power.RecorderTimeout,
			Val:       defaultCABCDisplayParams,
			ExtraData: pageDataForCABCTest,
		}, {
			Name:      "max_cabc_ash",
			Fixture:   "powerAsh",
			Timeout:   10*time.Minute + power.RecorderTimeout,
			Val:       maxCABCDisplayParams,
			ExtraData: pageDataForCABCTest,
		}, {
			Name:              "default_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Timeout:           10*time.Minute + power.RecorderTimeout,
			Val:               defaultDisplayParams,
			ExtraData:         pageData,
		}, {
			Name:              "fast_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Timeout:           5*time.Minute + power.RecorderTimeout,
			Val:               fastDisplayParams,
			ExtraData:         pageData,
		}, {
			Name:              "max_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Timeout:           10*time.Minute + power.RecorderTimeout,
			Val:               maxDisplayParams,
			ExtraData:         pageData,
		}, {
			Name:              "all_brightness_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Timeout:           1*time.Hour + power.RecorderTimeout,
			Val:               brightnessDisplayParams,
			ExtraData:         pageDataForBrightnessTest,
		}, {
			Name:              "default_cabc_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Timeout:           10*time.Minute + power.RecorderTimeout,
			Val:               defaultCABCDisplayParams,
			ExtraData:         pageDataForCABCTest,
		}, {
			Name:              "max_cabc_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Timeout:           10*time.Minute + power.RecorderTimeout,
			Val:               maxCABCDisplayParams,
			ExtraData:         pageDataForCABCTest,
		}},
	})
}

func Display(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateFullscreen); err != nil {
		s.Fatal("Failed to make the browser window fullscreen: ", err)
	}

	dp := s.Param().(power.DisplayParams)

	// Create the /tmp/html dir and copy display pages over to avoid the need of setting up local http server.
	htmlDir := filepath.Join(tmpPath, "html")
	if err := os.MkdirAll(htmlDir, 0755); err != nil {
		s.Fatalf("Failed to create dir %s: %v", htmlDir, err)
	}

	defer func() {
		s.Log("Deleting files copied to disk")
		if err := os.RemoveAll(htmlDir); err != nil {
			s.Fatalf("Failed to delete %s : %v", htmlDir, err)
		}
	}()

	for _, page := range dp.Pages {
		dataPath := s.DataPath("display/html" + "/" + page + ".html")
		pagePath := filepath.Join(htmlDir, page+".html")
		if err := fsutil.CopyFile(dataPath, pagePath); err != nil {
			s.Error("Failed to copy data file: ", err)
		}
	}

	if !validBrightnessMap[dp.Brightness] {
		s.Errorf("Not an valid brightness flag: %q", dp.Brightness)
	}

	r := power.NewRecorder(ctx, dp.DisplayTimeParams.Interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)

	// Fixture already set brightness to default level, only need to set when it's max.
	if dp.Brightness == "max" {
		if err := setup.SetBacklightBrightnessNonLinearPercent(ctx, 100); err != nil {
			s.Fatal("Failed to set backlight to nonlinear percent: ", err)
		}
	}

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	//Navigate and display pages with different brightness level.
	httpPath := filepath.Join("file://", htmlDir)
	for _, page := range dp.Pages {
		url := filepath.Join(httpPath, page+".html")
		if err := conn.Navigate(ctx, url); err != nil {
			s.Fatal("Failed to navigate: ", err)
		}

		if dp.Brightness == "all_brightness" {
			for step := 16; step >= 0; step-- {
				if err := setup.SetBacklightBrightnessNonLinearPercent(ctx, float64(step)*6.25); err != nil {
					s.Fatal("Failed to set backlight to nonlinear percent: ", err)
				}
				// GoBigSleepLint: sleep to let DUT display pages to collect power data.
				if err := testing.Sleep(ctx, dp.DisplayTimeParams.Total); err != nil {
					s.Fatal("Failed to sleep: ", err)
				}
			}
		} else {
			// GoBigSleepLint: sleep to let DUT display pages to collect power data.
			if err := testing.Sleep(ctx, dp.DisplayTimeParams.Total); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
		}
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
