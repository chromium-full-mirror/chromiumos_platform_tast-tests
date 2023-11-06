// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	videoURL = testing.RegisterVarString(
		"power.VideoSuspend.videoURL",
		"http://commondatastorage.googleapis.com/chromeos-test-public/big_buck_bunny/big_buck_bunny_trailer_400p.mp4",
		"Video URL to play")
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoSuspend,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Suspend while video is playing",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
		},
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Vars:         []string{"power.VideoSuspend.videoURL"},
		Timeout:      5 * time.Minute,
		Data: []string{
			"video.html",
		},
		Fixture: "chromeLoggedIn",
	})
}

func checkVideoIsPlaying(ctx context.Context, conn *chrome.Conn) error {
	getCurrentTime := func(ctx context.Context) (float64, error) {
		var time float64
		err := conn.Eval(ctx, "player.currentTime", &time)
		return time, err
	}

	oldTime, err := getCurrentTime(ctx)
	if err != nil {
		return err
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		newTime, err := getCurrentTime(ctx)
		if err != nil {
			return err
		}
		if newTime > oldTime {
			return nil
		}
		return errors.New("Video is not playing")
	}, &testing.PollOptions{})
}

func suspendForSeconds(ctx context.Context, seconds int32) error {
	var (
		SuspendFailurePattern   = "Suspend failures: 0"
		FirmwareLogErrorPattern = "Firmware log errors: 0"
	)

	testing.ContextLog(ctx, "Suspending")
	out, err := testexec.CommandContext(
		ctx,
		"suspend_stress_test",
		fmt.Sprintf("--suspend_min=%d", seconds),
		fmt.Sprintf("--suspend_max=%d", seconds),
		"-c", "1").Output()
	if err != nil {
		return errors.Errorf("failed to execute suspend_stress_test command: %s", err)
	}
	testing.ContextLog(ctx, "Returned from suspend")

	errorCodes := []string{SuspendFailurePattern, FirmwareLogErrorPattern}
	for _, errMsg := range errorCodes {
		if !strings.Contains(string(out), errMsg) {
			testing.ContextLogf(ctx, "suspend_stress_test output: %s", string(out))
			return errors.Errorf("got failures, want %q", errMsg)
		}
	}
	return nil
}

// VideoSuspend suspends while video is playing then makes sure video is still
// playing after we resume.
func VideoSuspend(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	conn, err := cr.NewConn(ctx, server.URL+"/video.html")
	if err != nil {
		s.Fatal("Failed to open new tab: ", err)
	}
	if err := conn.Call(ctx, nil, "play", videoURL.Value()); err != nil {
		s.Fatal("Failed to start video: ", err)
	}
	if err := checkVideoIsPlaying(ctx, conn); err != nil {
		s.Fatal("Video not playing even prior to suspend: ", err)
	}

	// Suspending for less than 3 seconds seem to fairly reliably result in a
	// suspend failure. Presumably this is due to a race between entering S0ix
	// and the timer that's supposed to wake us up. We give it 3x this sure.
	if err := suspendForSeconds(ctx, 9); err != nil {
		s.Fatal("Failed to suspend: ", err)
	}

	testing.ContextLog(ctx, "Reconnecting to Chrome")
	err = cr.Reconnect(ctx)
	if err != nil {
		s.Fatal("Failed to reconnect to Chrome: ", err)
	}

	testing.ContextLog(ctx, "Reconnecting to our browser tab")
	conn, err = cr.NewConnForTarget(ctx, chrome.MatchAllPages())
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	if err != nil {
		s.Fatal("Failed to reconnect to browser tab")
	}

	testing.ContextLog(ctx, "Checking if video is still playing")
	if err := checkVideoIsPlaying(ctx, conn); err != nil {
		s.Fatal("Video not playing after suspend: ", err)
	}

	if err := shill.WaitForOnlineAfterResume(ctx); err != nil {
		s.Fatal("Network failed to comeback after resuming: ", err)
	}
}
