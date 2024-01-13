// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/tracing"
)

type browsingTestParam struct {
	ConfigName   string
	TimeParams   power.TimeParams
	CollectTrace bool
	MultiTab     bool
}

const setupTimeoutBuffer = 5 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         Browsing,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when browsing",
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"config_name", // Used in "custom" variant. The name of config file.
		},
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: "powerAsh",
			Timeout: time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:     browsingTestParam{ConfigName: "browsing", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
		}, {
			Name:              "lacros",
			Fixture:           "powerLacros",
			Timeout:           time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "browsing", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:      "20min_ash",
			Fixture:   "powerAsh",
			Timeout:   20*time.Minute + setupTimeoutBuffer + power.RecorderTimeout,
			Val:       browsingTestParam{ConfigName: "browsing_20min", TimeParams: power.TimeParams{Interval: 5 * time.Second, Total: 20 * time.Minute}},
			ExtraAttr: []string{"group:power", "power_daily"},
		}, {
			Name:              "20min_lacros",
			Fixture:           "powerLacros",
			Timeout:           20*time.Minute + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "browsing_20min", TimeParams: power.TimeParams{Interval: 5 * time.Second, Total: 20 * time.Minute}},
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraAttr:         []string{"group:power", "power_daily"},
		}, {
			Name:    "heavy_ash",
			Fixture: "powerAsh",
			Timeout: time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:     browsingTestParam{ConfigName: "heavy", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
		}, {
			Name:              "heavy_lacros",
			Fixture:           "powerLacros",
			Timeout:           time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "heavy", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}},
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:      "heavy_20min_ash",
			Fixture:   "powerAsh",
			Timeout:   20*time.Minute + setupTimeoutBuffer + power.RecorderTimeout,
			Val:       browsingTestParam{ConfigName: "heavy_20min", TimeParams: power.TimeParams{Interval: 5 * time.Second, Total: 20 * time.Minute}},
			ExtraAttr: []string{"group:power", "power_regression"},
		}, {
			Name:              "heavy_20min_lacros",
			Fixture:           "powerLacros",
			Timeout:           20*time.Minute + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "heavy_20min", TimeParams: power.TimeParams{Interval: 5 * time.Second, Total: 20 * time.Minute}},
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
			Name:    "multitab_ash",
			Fixture: "powerAsh",
			Timeout: time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:     browsingTestParam{ConfigName: "browsing", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}, MultiTab: true},
		}, {
			Name:              "multitab_lacros",
			Fixture:           "powerLacros",
			Timeout:           time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "browsing", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}, MultiTab: true},
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:      "tracing_ash",
			Fixture:   "powerAsh",
			Timeout:   time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:       browsingTestParam{ConfigName: "custom", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}, CollectTrace: true},
			ExtraData: []string{tracing.TBMTracedProbesConfigFile},
		}, {
			Name:              "tracing_lacros",
			Fixture:           "powerLacros",
			Timeout:           time.Hour + setupTimeoutBuffer + power.RecorderTimeout,
			Val:               browsingTestParam{ConfigName: "custom", TimeParams: power.TimeParams{Interval: 20 * time.Second, Total: time.Hour}, CollectTrace: true},
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraData:         []string{tracing.TBMTracedProbesConfigFile},
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

type tabData struct {
	Conn     *chrome.Conn
	TabIndex int
	WinIndex int
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
	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
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
	collectTrace := s.Param().(browsingTestParam).CollectTrace
	multiTab := s.Param().(browsingTestParam).MultiTab

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

	// Get ChromeApp name for window switching
	chromeApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find the Chrome app: ", err)
	}
	chromeName := chromeApp.Name

	// Default tabData for single tab case
	tab1 := tabData{Conn: conn, TabIndex: 0, WinIndex: 0}
	tabDataList := []tabData{tab1}

	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create action handler: ", err)
	}
	defer uiHandler.Close(cleanupCtx)

	// Create Window A with tab 1 & 2 in foreground
	// Window B with tab 3 & 4 & 5 in background
	if multiTab {
		// tab1 was already created above
		conn2, err := uiHandler.NewChromeTab(ctx, br, "about:blank", false)
		if err != nil {
			s.Fatal("Failed to open new Chrome tab: ", err)
		}
		tab2 := tabData{Conn: conn2, TabIndex: 1, WinIndex: 0}

		conn3, err := uiHandler.NewChromeTab(ctx, br, "about:blank", true)
		if err != nil {
			s.Fatal("Failed to open new Chrome tab: ", err)
		}
		tab3 := tabData{Conn: conn3, TabIndex: 0, WinIndex: 1}

		conn4, err := uiHandler.NewChromeTab(ctx, br, "about:blank", false)
		if err != nil {
			s.Fatal("Failed to open new Chrome tab: ", err)
		}
		tab4 := tabData{Conn: conn4, TabIndex: 1, WinIndex: 1}

		conn5, err := uiHandler.NewChromeTab(ctx, br, "about:blank", false)
		if err != nil {
			s.Fatal("Failed to open new Chrome tab: ", err)
		}
		tab5 := tabData{Conn: conn5, TabIndex: 2, WinIndex: 1}

		tabDataList = []tabData{tab1, tab2, tab3, tab4, tab5}
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

	if collectTrace {
		var session *tracing.Session
		traceConfigPath := s.DataPath(tracing.TBMTracedProbesConfigFile)
		traceDataPath := filepath.Join(s.OutDir(), "trace.perfetto-trace")
		session, err = tracing.StartSession(ctx, traceConfigPath, tracing.WithTraceDataPath(traceDataPath))
		if err != nil {
			s.Fatal("Failed to start tracing: ", err)
		}
		s.Log("Collecting Perfetto trace File at: ", session.TraceDataPath())

		defer session.Finalize(cleanupCtx)
		defer session.Stop()
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Start of main test body.
	tabDataIndex := 0
	for loop := 0; loop < loopCount; loop++ {

		loopName := fmt.Sprintf("loop%02d", loop)
		loopCheckpoint := r.StartCheckpoint(loopName)

		for _, site := range config.URLData.Pages {
			tabData := tabDataList[tabDataIndex]
			tabDataIndex = (tabDataIndex + 1) % len(tabDataList)

			startTime := time.Now()

			if multiTab {
				if tabData.TabIndex == 0 {
					if err := uiHandler.SwitchToAppWindowByIndex(chromeName, tabData.WinIndex)(ctx); err != nil {
						s.Fatal("Failed to switch Chrome window: ", err)
					}
				}
				if err := uiHandler.SwitchToChromeTabByIndex(tabData.TabIndex)(ctx); err != nil {
					s.Fatal("Failed to switch Chrome tab: ", err)
				}
			}

			url := urlPrefix + redirectFile + "?ver=" + config.URLData.Version + "&dest=" + site
			if err := tabData.Conn.Navigate(ctx, url); err != nil {
				s.Fatal("Failed to navigate: ", err)
			}

			siteCheckpoint := r.StartCheckpoint(site)

			scrollAmount := 600
			if secsPerScroll > 0 {
				for sec := secsPerScroll; sec < secsPerPage; sec += secsPerScroll {
					endTime := startTime.Add(time.Duration(sec) * time.Second)
					// GoBigSleepLint: Sleep to measure power
					if err := testing.Sleep(ctx, time.Until(endTime)); err != nil {
						s.Fatal("Failed to sleep: ", err)
					}

					js := fmt.Sprintf("window.scrollBy(0, %d)", scrollAmount)
					if err := tabData.Conn.Eval(ctx, js, nil); err != nil {
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

			r.EndCheckpoint(siteCheckpoint)

		}

		r.EndCheckpoint(loopCheckpoint)

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
