// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
)

type browsingTestParam struct {
	ConfigName string
	TimeParams power.TimeParams
}

const setupTimeoutBuffer = 5 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         Browsing,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when browsing",
		BugComponent: "b:167191", // ChromeOS > Platform > System > Power
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"config_name", // Used in "custom" variant. The name of config file.
		},
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: "powerAsh",
			Timeout: time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:     browsingTestParam{ConfigName: "typical", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
		}, {
			Name:              "lacros",
			Fixture:           "powerLacros",
			Timeout:           time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "typical", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:    "live_ash",
			Fixture: "powerAsh",
			Timeout: time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:     browsingTestParam{ConfigName: "live", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
		}, {
			Name:              "live_lacros",
			Fixture:           "powerLacros",
			Timeout:           time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "live", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:    "custom_ash",
			Fixture: "powerAsh",
			Timeout: time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:     browsingTestParam{ConfigName: "custom", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
		}, {
			Name:              "custom_lacros",
			Fixture:           "powerLacros",
			Timeout:           time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "custom", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// timingData describes execution timing for browsing test
type timingData struct {
	// Number of loop to test
	LoopCount int `json:"loop_count"`
	// Duration in seconds for each page
	SecsPerPage int `json:"secs_per_page"`
	// Interval between each scroll with 0 indicate no scroll
	SecsPerScroll int `json:"secs_per_scroll"`
}

// urlData web page for browsing test
type urlData struct {
	// Number of page to test
	NumPage int `json:"num_page"`
	// Version of page caching, live indicates live page.
	Version string `json:"version"`
	// Pages to browse
	Pages []string `json:"pages"`
}

// browsingConfig describes configuration for this test.
type browsingConfig struct {
	FormatVersion int        `json:"format_version"`
	Version       string     `json:"config_version"`
	TimingData    timingData `json:"timing_data"`
	URLData       urlData    `json:"url_data"`
}

func Browsing(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	const (
		urlPrefix       = "https://storage.googleapis.com/chromiumos-test-assets-public/power_LoadTest/v2_config/"
		configURLSuffix = ".json"
		redirectFile    = "redirect.html"
	)

	// Fetch config from url and parse
	configName := s.Param().(browsingTestParam).ConfigName
	interval := s.Param().(browsingTestParam).TimeParams.Interval
	totalTime := s.Param().(browsingTestParam).TimeParams.Total

	if configName == "custom" {
		if v, ok := s.Var("config_name"); ok {
			configName = v
		} else {
			s.Fatal("Use custom version without specified config name")
		}
	}

	configURL := urlPrefix + configName + configURLSuffix

	configJSON, err := utils.FetchFromURL(ctx, configURL)
	if err != nil {
		s.Fatalf("Failed to fetch configuration from %s: %v", configURL, err)
	}

	configFileName := s.OutDir() + configName + configURLSuffix
	if err := ioutil.WriteFile(configFileName, []byte(configJSON), 0644); err != nil {
		s.Fatalf("Failed to write %s json file: %v", configFileName, err)
	}

	config := &browsingConfig{}
	if err := json.Unmarshal([]byte(configJSON), config); err != nil {
		s.Fatal("Failed to unmarshal configuration: ", err)
	}

	if err := validateConfig(config, interval, totalTime); err != nil {
		s.Fatal("Wrong config: ", err)
	}

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	loopCount := config.TimingData.LoopCount
	secsPerPage := config.TimingData.SecsPerPage
	secsPerScroll := config.TimingData.SecsPerScroll

	// Generate custom perf.Values for summarize in power_log.html
	configValues := perf.NewValues()
	configValues.Set(perf.Metric{Name: "perf.BrowsingConfig_loopCount", Unit: "unit"}, float64(loopCount))
	configValues.Set(perf.Metric{Name: "perf.BrowsingConfig_secsPerPage", Unit: "s"}, float64(secsPerPage))
	configValues.Set(perf.Metric{Name: "perf.BrowsingConfig_secsPerScroll", Unit: "s"}, float64(secsPerScroll))
	configValues.Set(perf.Metric{Name: "perf.BrowsingConfig_numPage", Unit: "unit"}, float64(config.URLData.NumPage))

	// Put the value in the Name for String data.
	configValues.Set(perf.Metric{Name: "perf.BrowsingConfig_ConfigName_" + configName, Unit: "unit"}, 0)
	configValues.Set(perf.Metric{Name: "perf.BrowsingConfig_ConfigVersion_" + config.Version, Unit: "unit"}, 0)
	configValues.Set(perf.Metric{Name: "perf.BrowsingConfig_ConfigURLVersion_" + config.URLData.Version, Unit: "unit"}, 0)

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Start of main test body.
	for loop := 0; loop < loopCount; loop++ {
		for _, site := range config.URLData.Pages {
			startTime := time.Now()
			url := urlPrefix + redirectFile + "?ver=" + config.URLData.Version + "&dest=" + site
			if err := conn.Navigate(ctx, url); err != nil {
				s.Fatal("Failed to navigate: ", err)
			}

			scrollAmount := 600
			if secsPerScroll > 0 {
				for sec := secsPerScroll; sec < secsPerPage; sec += secsPerScroll {
					endTime := startTime.Add(time.Duration(sec) * time.Second)
					// GoBigSleepLint: Sleep to measure power
					if err := testing.Sleep(ctx, time.Until(endTime)); err != nil {
						s.Fatal("Failed to sleep: ", err)
					}

					js := fmt.Sprintf("window.scrollBy(0, %d)", scrollAmount)
					if err := conn.Eval(ctx, js, nil); err != nil {
						s.Fatal("Failed to scroll: ", err)
					}
					scrollAmount = -scrollAmount
				}
			}
			endTime := startTime.Add(time.Duration(secsPerPage) * time.Second)
			// GoBigSleepLint: Sleep to measure power
			if err := testing.Sleep(ctx, time.Until(endTime)); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}

		}
	}
	// End of main test body.

	if err := r.Finish(ctx, configValues); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}

func validateConfig(config *browsingConfig, interval, totalTime time.Duration) error {
	const maxSupportConfigVersion = 1
	if config.FormatVersion > maxSupportConfigVersion {
		return errors.Errorf("got config version %d, only support version upto %d", config.FormatVersion, maxSupportConfigVersion)
	}
	numPage := config.URLData.NumPage
	if numPage < 1 {
		return errors.Errorf("invalid NumPage (%d)", numPage)
	}
	if numPage != len(config.URLData.Pages) {
		return errors.Errorf("NumPage (%d) and len(Pages) (%d) mismatch", numPage, len(config.URLData.Pages))
	}
	loopCount := config.TimingData.LoopCount
	if loopCount < 1 {
		return errors.Errorf("invalid LoopCount (%d)", loopCount)
	}
	secsPerScroll := config.TimingData.SecsPerScroll
	if secsPerScroll > 0 && time.Duration(secsPerScroll)*time.Second < interval {
		return errors.Errorf("secsPerScroll (%d) is less than measurement interval (%v)", secsPerScroll, interval)
	}
	secsPerPage := config.TimingData.SecsPerPage
	if time.Duration(secsPerPage)*time.Second < interval {
		return errors.Errorf("secsPerPage (%d) is less than measurement interval (%v)", secsPerPage, interval)
	}
	if secsPerScroll > 0 && secsPerPage < secsPerScroll {
		return errors.Errorf("secsPerPage (%d) is less than secsPerScroll (%d)", secsPerPage, secsPerScroll)
	}
	inferredTotal := time.Duration(loopCount*numPage*secsPerPage) * time.Second
	if inferredTotal > totalTime {
		return errors.Errorf("total time in the config (%v) is more than total test run time (%v)", inferredTotal, totalTime)
	}
	return nil
}
