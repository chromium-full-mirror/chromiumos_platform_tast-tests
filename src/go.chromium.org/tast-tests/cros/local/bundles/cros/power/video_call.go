// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast-tests/cros/local/upstart"
)

// Add 5 minutes buffer time for Timeout
const timeoutBuffer = 5 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoCall,
		Desc:         "Collect power metrics when mutitasking typing and video call",
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		Contacts:     []string{"chromeos-power-team@google.com"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Params: []testing.Param{{
			Name:    "3m_ash",
			Fixture: "powerAsh",
			Val:     power.TimeParams{Interval: 5 * time.Second, Total: 3 * time.Minute},
			Timeout: 3*time.Minute + timeoutBuffer + power.RecorderTimeout,
			ExtraAttr: []string{
				"group:power",
				"power_daily",
				"power_weekly",
				"group:release-health",
				"release-health_power",
			},
		}, {
			Name:    "25m_ash",
			Fixture: "powerAsh",
			Val:     power.TimeParams{Interval: 20 * time.Second, Total: 25 * time.Minute},
			Timeout: 25*time.Minute + timeoutBuffer + power.RecorderTimeout + setup.BatteryPreparationTimeout,
			ExtraAttr: []string{
				"group:power",
				"power_regression",
				"group:release-health",
				"release-health_power",
			},
		}, {
			Name:              "25m_ash_arc",
			Fixture:           "powerAshARC",
			Val:               power.TimeParams{Interval: 20 * time.Second, Total: 25 * time.Minute},
			Timeout:           25*time.Minute + timeoutBuffer + power.RecorderTimeout + setup.BatteryPreparationTimeout,
			ExtraSoftwareDeps: []string{"arc"},
		}, {
			Name:    "2hr_ash",
			Fixture: "powerAsh",
			Val:     power.TimeParams{Interval: 20 * time.Second, Total: 2 * time.Hour},
			Timeout: 2*time.Hour + timeoutBuffer + power.RecorderTimeout + setup.BatteryPreparationTimeout,
		}},
	})
}

func VideoCall(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	interval := s.Param().(power.TimeParams).Interval
	total := s.Param().(power.TimeParams).Total

	// Restart camera service to recover from potential bad state left by previous tests.
	if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	ui := uiauto.New(tconn)

	// TODO(b/280888518): Add preset
	const (
		urlDoc              = "https://storage.googleapis.com/chromiumos-test-assets-public/power_VideoCall/power_VideoCall_doc.html"
		permBubbleName      = "storage.googleapis.com wants to"
		cameraDataNameRegex = "id : camera_.* fps: .*"
		titleDoc            = "power_VideoCall Doc"
		jsVideoArray        = "Array.from(document.getElementsByTagName('video'))"
		jsAllPlaying        = ".every(v => v.currentTime >= 0.001)"
		jsPrintTime         = ".map(v => v.id + ': ' + v.currentTime).join(', ')"
		jsAreVideosPlaying  = jsVideoArray + jsAllPlaying
		jsPrintVideosTime   = jsVideoArray + jsPrintTime
	)

	// Open a VideoWindow and snap to the left
	videoConn, err := cr.NewConn(ctx, "about:blank")
	if err != nil {
		s.Fatal("Failed to setup a new tab for video: ", err)
	}
	defer videoConn.Close()
	defer videoConn.CloseTarget(cleanupCtx)

	bTconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get browser test API connection: ", err)
	}

	videoWin, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch())
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, videoWin.ID, ash.WindowStatePrimarySnapped); err != nil {
		s.Fatal("Failed to snap video window to the left: ", err)
	}
	docConn, err := cr.NewConn(ctx, urlDoc, browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to setup a new window for doc: ", err)
	}
	defer docConn.Close()
	defer docConn.CloseTarget(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(ctx, s.OutDir(), s.HasError, tconn, "ui_dump")

	docWin, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, titleDoc)
	if err != nil {
		s.Fatal("Failed to open a new window for doc: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, docWin.ID, ash.WindowStateSecondarySnapped); err != nil {
		s.Fatal("Failed to snap doc window to the right: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	if err := setup.Battery(ctx, total, discharge); err != nil {
		s.Fatal("Setup battery failed: ", err)
	}

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)
	// Register test specific metrics.
	r.RegisterMetrics(
		pm.NewVideoFpsMetrics(videoConn),
		pm.NewWebRTCMetrics(videoConn),
		pm.NewHistogramMetrics(bTconn, []string{"EventLatency.KeyPressed.TotalLatency"}),
	)

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if err := videoConn.Navigate(ctx, getVideoURL(ctx)); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}

	// Allow camera permission if the prompt exists.
	if err := prompts.ClearPotentialPrompts(tconn, 30*time.Second, prompts.AllowCameraPermPrompt)(ctx); err != nil {
		s.Fatal("Failed to clear camera permission prompt dialog: ", err)
	}

	cameraData := nodewith.NameRegex(regexp.MustCompile(cameraDataNameRegex)).First()
	if err := ui.WaitUntilExists(cameraData)(ctx); err != nil {
		s.Fatal("Failed to find the fps data note: ", err)
	}

	// Wait until all videos start playing
	if err := videoConn.WaitForExprWithTimeout(ctx, jsAreVideosPlaying, 10*time.Second); err != nil {
		videosTimeStr := ""
		videoConn.Eval(ctx, jsPrintVideosTime, &videosTimeStr)
		s.Fatalf("Failed to ensure that all videos are played: %v, videos currentTime are %s", err, videosTimeStr)
	}

	// GoBigSleepLint: Wait 5 seconds for WebRTC bandwidth to stabilize
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// Select text input field
	pc := pointer.NewMouse(tconn)
	if err := pc.Click(nodewith.Name("Edit here").Role(role.TextField))(ctx); err != nil {
		s.Fatal("Failed to select input field on docs page: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Start of main test body.

	// Typing 1 minute per loops, 6 seconds per string.
	var typingStrs = []string{
		"1234567890 ", "1234567891 ", "1234567892 ", "1234567893 ", "1234567894 \n",
		"1234567895 ", "1234567896 ", "1234567897 ", "1234567898 ", "1234567899 \n",
	}
	const secPerChunk = 6
	numLoop := int(total.Minutes())

	startTime := time.Now()
	for loop := 0; loop < numLoop; loop++ {
		for chunk, str := range typingStrs {
			kb.Type(ctx, str)
			loopDuration := time.Duration(loop) * time.Minute
			chunkDuration := time.Duration((chunk+1)*secPerChunk) * time.Second
			endTime := startTime.Add(loopDuration).Add(chunkDuration)
			// GoBigSleepLint: kb.Type is too fast, sleep to match pace of 1 min per loop.
			if err := testing.Sleep(ctx, time.Until(endTime)); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
		}

	}
	// End of main test body.

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}

// getVideoURL Return camera preset appropriate to hw spec.
func getVideoURL(ctx context.Context) string {
	const basicURL = "https://storage.googleapis.com/chromiumos-test-assets-public/power_VideoCall/power_VideoCall.webrtc.html"

	// The following CPUs are based on the results of the limitation_cpu metric queried from Crosbolt.
	// If a limitation occurs, the low preset will be used.
	// Example query:
	// https://healthmon.chromeos.goog/time_series/tast.power.VideoCall.25m_ash/webrtc_limitation.limitation_cpu?skuFilter=sku%3Dblacktip_IntelR_CeleronR_CPU_N3350_1_10GHz_4GB
	regexp := regexp.MustCompile(`Intel[ ]Pentium[ ]Silver[ ]N[0-5]{4}|` + // Intel Pentium Sliver N5000
		`Intel[ ][Pentium|Celeron][ ][0-9]{4,5}[UY]|` + // Intel Pentium 6405U, Intel Celeron 5205U, Intel Celeron 3965Y
		`Intel[ ]Celeron[ ]N[3-4][0-9]{3}|` + // Intel Celeron N3350, N4500
		`Intel[ ]Core[ ]m3-[0-9]{4}Y|` + // Intel Core m3-8100Y
		`Intel[ ]Core[ ]i[357]-[67]Y[0-9]{2}|` + // Intel Core i7-7Y75
		`Intel[ ]Core[ ]i[357]-(8[0-9]{3}U|9[0-9]{3}U|10[0-9]{3}U)` + // Intel Core i7-8650U, i7-10610U
		`Intel[ ]Core[ ]i[5]-[0-9]{4}G[0-9]|` + // Intel Core i5-1135G7
		`AMD[ ]Athlon[ ]Gold[ ]3[0-9]{3}C|` + // AMD Athlon Gold 3150C
		`AMD[ ]Ryzen[ ]3[ ]3[0-9]{3}C|` + // AMD Ryzen 3 3250C
		`AMD[ ][3-9][0-9]{3}Ce|` + // AMD 3015Ce
		`AMD[ ]A[4-9]-[0-9]{4}C|` + // AMD A4-9120C
		`mediatek[ ]mt81[7-8][0-9]|` + // mediaTek mt8173, mediaTek mt8183, mediaTek mt8186
		`qcom[ ]sc[0-9]{4}`) // qcom sc7180

	cpuName := util.GetCPUName(ctx)

	if regexp.MatchString(cpuName) {
		return basicURL + "?preset=low"
	}
	return basicURL + "?preset=high"
}
