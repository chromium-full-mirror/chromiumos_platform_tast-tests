// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/pointer"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/power"
	"chromiumos/tast/local/power/setup"
)

// Add 5 minutes buffer time for Timeout
const timeoutBuffer = 5 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoCall,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when mutitasking typing and video call",
		BugComponent: "b:167191", // ChromeOS > Platform > System > Power
		Contacts:     []string{"chromeos-platform-power@google.com"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "3m_ash",
			Fixture: "powerAsh",
			Val:     3,
			Timeout: 3*time.Minute + timeoutBuffer,
		}, {
			Name:    "25m_ash",
			Fixture: "powerAsh",
			Val:     25,
			Timeout: 25*time.Minute + timeoutBuffer,
		}, {
			Name:    "2hr_ash",
			Fixture: "powerAsh",
			Val:     120,
			Timeout: 2*time.Hour + timeoutBuffer,
		}, {
			Name:              "3m_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               3,
			Timeout:           3*time.Minute + timeoutBuffer,
		}, {
			Name:              "25m_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               25,
			Timeout:           25*time.Minute + timeoutBuffer,
		}, {
			Name:              "2hr_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               120,
			Timeout:           2*time.Hour + timeoutBuffer,
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

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	uiauto := uiauto.New(tconn)

	// TODO(b/280888518): Add preset
	const (
		urlVideo = "https://storage.googleapis.com/chromiumos-test-assets-public/power_VideoCall/power_VideoCall.webrtc.html?preset=high"
		urlDoc   = "http://crospower.page.link/power_VideoCall_doc"
		titleDoc = "power_VideoCall Doc"
	)

	// Open a VideoWindow and snap to the left
	videoConn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to setup a new tab for video: ", err)
	}
	defer cleanup(cleanupCtx)
	defer videoConn.Close()
	defer videoConn.CloseTarget(cleanupCtx)

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

	power.RegisterPowerMetrics(power.NewVideoFpsMetrics(videoConn))

	// TODO(b/280888518): Use shorter interval in shorter test run.
	r, err := power.NewRecorder(ctx, 20*time.Second, s.OutDir(), s.TestName())
	if err != nil {
		s.Fatal("Cannot create a new Recorder to collect power metrics: ", err)
	}
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if err := videoConn.Navigate(ctx, urlVideo); err != nil {
		s.Fatal("Failed to navigate: ", err)
	}

	bubble := nodewith.ClassName("PermissionPromptBubbleView").First()
	allow := nodewith.Name("Allow").Role(role.Button).Ancestor(bubble)
	if err := uiauto.WaitUntilExists(allow)(ctx); err != nil {
		s.Fatal("Failed to find the permission bubble: ", err)
	}

	pc := pointer.NewMouse(tconn)
	if err := pc.Click(allow)(ctx); err != nil {
		s.Fatal("Failed to click permission bubble: ", err)
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
	numLoop := s.Param().(int)
	var typingStrs = []string{
		"1234567890 ", "1234567891 ", "1234567892 ", "1234567893 ", "1234567894 \n",
		"1234567895 ", "1234567896 ", "1234567897 ", "1234567898 ", "1234567899 \n",
	}
	const secPerChunk = 6

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
