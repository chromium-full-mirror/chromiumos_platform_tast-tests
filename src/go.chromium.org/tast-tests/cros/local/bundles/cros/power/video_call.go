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
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
)

// Add 5 minutes buffer time for Timeout
const timeoutBuffer = 5 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoCall,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when mutitasking typing and video call",
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Params: []testing.Param{{
			Name:    "3m_ash",
			Fixture: "powerAsh",
			Val:     power.TimeParams{Interval: 5 * time.Second, Total: 3 * time.Minute},
			Timeout: 3*time.Minute + timeoutBuffer + power.RecorderTimeout,
		}, {
			Name:      "25m_ash",
			Fixture:   "powerAsh",
			Val:       power.TimeParams{Interval: 20 * time.Second, Total: 25 * time.Minute},
			Timeout:   25*time.Minute + timeoutBuffer + power.RecorderTimeout,
			ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild", "group:power", "power_regression"},
		}, {
			Name:    "2hr_ash",
			Fixture: "powerAsh",
			Val:     power.TimeParams{Interval: 20 * time.Second, Total: 2 * time.Hour},
			Timeout: 2*time.Hour + timeoutBuffer + power.RecorderTimeout,
		}, {
			Name:              "3m_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.TimeParams{Interval: 5 * time.Second, Total: 3 * time.Minute},
			Timeout:           3*time.Minute + timeoutBuffer + power.RecorderTimeout,
		}, {
			Name:              "25m_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.TimeParams{Interval: 20 * time.Second, Total: 25 * time.Minute},
			Timeout:           25*time.Minute + timeoutBuffer + power.RecorderTimeout,
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		}, {
			Name:              "2hr_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               power.TimeParams{Interval: 20 * time.Second, Total: 2 * time.Hour},
			Timeout:           2*time.Hour + timeoutBuffer + power.RecorderTimeout,
		}},
	})
}

func VideoCall(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	interval := s.Param().(power.TimeParams).Interval
	total := s.Param().(power.TimeParams).Total

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	// TODO(b/280888518): Add preset
	const (
		urlVideo            = "https://storage.googleapis.com/chromiumos-test-assets-public/power_VideoCall/power_VideoCall.webrtc.html?preset=high"
		urlDoc              = "http://crospower.page.link/power_VideoCall_doc"
		permBubbleName      = "storage.googleapis.com wants to"
		cameraDataNameRegex = "id : camera_.* fps: .*"
		titleDoc            = "power_VideoCall Doc"
	)

	// Open a VideoWindow and snap to the left
	videoConn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to setup a new tab for video: ", err)
	}
	defer cleanup(cleanupCtx)
	defer videoConn.Close()
	defer videoConn.CloseTarget(cleanupCtx)

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get browser test API connection: ", err)
	}

	videoWin, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, videoWin.ID, ash.WindowStatePrimarySnapped); err != nil {
		s.Fatal("Failed to snap video window to the left: ", err)
	}
	docConn, err := br.NewConn(ctx, urlDoc, browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to setup a new window for doc: ", err)
	}
	defer docConn.Close()
	defer docConn.CloseTarget(cleanupCtx)

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

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	// Register test specific metrics.
	r.RegisterMetrics(
		power.NewVideoFpsMetrics(videoConn),
		power.NewWebRTCMetrics(videoConn),
		power.NewHistogramMetrics(bTconn, []string{"EventLatency.KeyPressed.TotalLatency"}),
	)

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if err := videoConn.Navigate(ctx, urlVideo); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}

	// Check whether we need camera permission by find one of these
	// * Allow button for camera permission
	// * cameraData node which indicate that the camera is already recorded
	bubble := nodewith.NameStartingWith(permBubbleName).First()
	allow := nodewith.Name("Allow").Role(role.Button).Ancestor(bubble)
	cameraData := nodewith.NameRegex(regexp.MustCompile(cameraDataNameRegex)).First()

	foundNode, err := ui.FindAnyExists(ctx, allow, cameraData)
	if err != nil {
		s.Fatal("Failed to find the permission bubble: ", err)
	}

	// Click the allow button if found, and wait until cameraData node exists
	pc := pointer.NewMouse(tconn)
	if foundNode == allow {
		if err := pc.Click(allow)(ctx); err != nil {
			s.Fatal("Failed to click permission bubble: ", err)
		}
		if err := ui.WaitUntilExists(cameraData)(ctx); err != nil {
			s.Fatal("Failed to find the fps data note: ", err)
		}
	}

	// GoBigSleepLint: Wait 5 seconds for WebRTC bandwidth to stabilize
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// Select text input field
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
