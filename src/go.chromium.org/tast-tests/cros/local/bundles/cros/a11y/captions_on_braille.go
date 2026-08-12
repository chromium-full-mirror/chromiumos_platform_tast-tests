// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CaptionsOnBraille,
		Desc: "Collect power metrics of using captions on braille",
		Contacts: []string{
			"chromeos-a11y-eng@google.com",
			"xiyuan@google.com",
		},
		BugComponent: "b:1272895",
		SoftwareDeps: []string{"chrome", "ondevice_speech"},
		Fixture:      a11y.SodaDLCInstalled,
		Attr:         []string{"group:mainline", "informational"},
		Data: []string{
			"live_caption_power.html",
			"voice_en_long.ogg",
		},
	})
}

func CaptionsOnBraille(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	const (
		pageTitle = "Audio Playback For Live Caption Power Test"
	)

	cr, err := chrome.New(ctx,
		chrome.EnableFeatures("CaptionsOnBrailleDisplay"),
	)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	ui := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	// Enable ChromeVox and open the test page.
	cvData, err := chromevox.SetUpWithURLWithoutFocusWaiter(ctx, cr,
		tts.GoogleTTSEnUsVoice(), tts.GoogleTTSEngine(),
		server.URL+"/live_caption_power.html")
	if err != nil {
		s.Fatal("Failed to set up ChromeVox: ", err)
	}
	defer func() {
		cvData.TTSData.Conn.CloseTarget(cleanupCtx)

		if err := cvData.TearDown(); err != nil {
			s.Fatal("Failed to tear down ChromeVox test: ", err)
		}
	}()

	toggleViaKeyboard := func() error {
		return uiauto.Combine(
			"enable Captions on Braille via keyboard",
			kb.AccelAction("Search+O"),
			kb.AccelAction("c"),
		)(ctx)
	}

	// Toggle the first time should auto enable live captions and move ChromeVox
	// focus to the live caption bubble when it shows up.
	s.Log("Turn on Captions on Braille via keyboard command")
	if err := toggleViaKeyboard(); err != nil {
		s.Fatal("Failed to toggle Captions on Braille: ", err)
	}

	// Wait until dlc libsoda and libsoda-model-en-us are installed.
	if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for libsoda dlc to be installed: ", err)
	}

	pageRootWebArea := nodewith.Role(role.RootWebArea).Name(pageTitle)
	audioPlayButton := nodewith.Name("play").Role(role.Button).Ancestor(pageRootWebArea)
	audioPauseButton := nodewith.Name("pause").Role(role.Button).Ancestor(pageRootWebArea)
	liveCaptionBubble := nodewith.ClassName("CaptionBubbleLabel")

	// Play the audio.
	if err := ui.DoDefaultUntil(audioPlayButton, ui.WithTimeout(3*time.Second).WaitUntilExists(audioPauseButton))(ctx); err != nil {
		s.Fatal("Failed to play the audio: ", err)
	}

	// Verify that live caption bubble appears.
	if err := ui.WaitUntilExists(liveCaptionBubble)(ctx); err != nil {
		s.Fatal("Failed to wait for live caption bubble to show: ", err)
	}

	// Verify that ChromeVoxRange jumps to live caption bubble when it shows up.
	s.Log("Wait for ChromeVox focus jump to live caption bubble")
	if err := cvData.CVConn.WaitForFocusedNode(ctx, tconn, liveCaptionBubble); err != nil {
		s.Fatal("Focus is not on live caption bubble: ", err)
	}

	// Toggle and focus should be restored to the play/pause button.
	s.Log("Toggle and wait for focus restored to play/pause button")
	if err := toggleViaKeyboard(); err != nil {
		s.Fatal("Failed to toggle Captions on Braille: ", err)
	}
	if err := cvData.CVConn.WaitForFocusedNode(ctx, tconn, audioPauseButton); err != nil {
		s.Fatal("Focus is not on play button: ", err)
	}

	// Toggle and focus should move to the live caption bubble.
	s.Log("Toggle and wait for focus to go to live caption bubble")
	if err := toggleViaKeyboard(); err != nil {
		s.Fatal("Failed to toggle Captions on Braille: ", err)
	}
	if err := cvData.CVConn.WaitForFocusedNode(ctx, tconn, liveCaptionBubble); err != nil {
		s.Fatal("Focus is not on live caption bubble: ", err)
	}

	// Press `Tab` key and focus should be restored to the play/pause button.
	s.Log("Press Tab key and wait for focus to go to the play/pause button")
	if err := kb.Accel(ctx, "Tab"); err != nil {
		s.Fatal("Failed to press Tab: ", err)
	}
	if err := cvData.CVConn.WaitForFocusedNode(ctx, tconn, audioPauseButton); err != nil {
		s.Fatal("Focus is not on play button: ", err)
	}

	// Toggle and focus should move to the live caption bubble.
	s.Log("Toggle and wait for focus to go to live caption bubble")
	if err := toggleViaKeyboard(); err != nil {
		s.Fatal("Failed to toggle Captions on Braille: ", err)
	}
	if err := cvData.CVConn.WaitForFocusedNode(ctx, tconn, liveCaptionBubble); err != nil {
		s.Fatal("Focus is not on live caption bubble: ", err)
	}
}
