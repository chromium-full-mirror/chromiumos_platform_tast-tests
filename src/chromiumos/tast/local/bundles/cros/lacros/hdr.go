// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lacros tests lacros-chrome running on ChromeOS.
package lacros

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/audio"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/lacros"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/mtbf/youtube"
	"chromiumos/tast/local/power"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

const testVol = 1

var pixelFormatPattern = regexp.MustCompile(`(?:format=)\w+`)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HDR,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that hdr videos play on lacros and use a 30-bit buffer",
		Contacts:     []string{"lacros-team@google.com", "mrfemi@google.com"},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.Model("kohaku")),
		SoftwareDeps: []string{"chrome", "lacros"},
		Fixture:      "lacrosHDR",
		Timeout:      7 * time.Minute,
	})
}

func HDR(ctx context.Context, s *testing.State) {
	// Ensure display is on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}
	var videoSource = youtube.VideoSrc{
		URL:     "https://www.youtube.com/watch?v=N1-Jmq7BLFE",
		Title:   "Bulgaria 8K HDR 60P (FUHD)",
		Quality: "1080p60",
	}

	// Reserve time to clean up other resources.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Lower the volume.
	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create the volumeHelper: ", err)
	}
	s.Logf("Setting Output node volume to %d", testVol)
	if err := vh.SetVolume(ctx, testVol); err != nil {
		s.Errorf("Failed to set output node volume to %d: %v", testVol, err)
	}

	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch lacros-chrome: ", err)
	}

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close()

	extendedDisplay := false
	ui := uiauto.New(tconn)

	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create clamshell action handler: ", err)
	}
	defer uiHandler.Close()

	videoApp := youtube.NewYtWeb(l.Browser(), tconn, kb, extendedDisplay, ui, uiHandler)
	if err := videoApp.OpenAndPlayVideo(videoSource)(ctx); err != nil {
		s.Fatalf("Failed to open %q: %v", videoSource.URL, err)
	}
	defer videoApp.Close(cleanupCtx)

	var filePath string
	for i := 0; ; i++ {
		filePath = fmt.Sprintf("/sys/kernel/debug/dri/%d/state", i)
		_, err := os.Stat(filePath)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			s.Fatalf("Failed to stat %q: %v", filePath, err)
		}
		if i == 2 {
			s.Fatal("No dri debug file exists")
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		s.Fatal("Could not read dri debug file")
	}
	matches := pixelFormatPattern.FindStringSubmatch(string(data))
	for _, match := range matches {
		if match == "format=AR30" ||
			match == "format=AB30" ||
			match == "format=XR30" ||
			match == "format=XB30" {
			return
		}
	}
	s.Error("Did not find 30-bit buffer")
}
