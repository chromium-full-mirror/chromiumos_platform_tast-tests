// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyPsrSleepStateWithEdp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify PSR sleep states with eDP panel",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		Attr:         []string{"group:intel-nda"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInPsrEnableDisable",
		Timeout:      8 * time.Minute,
	})
}

func VerifyPsrSleepStateWithEdp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	videoStatus := "No video is playing"
	if err := psrCompatibleStatus(ctx, "", videoStatus, true); err != nil {
		s.Fatal("Failed to check psrCompatible data: ", err)
	}

	cmd := testexec.CommandContext(ctx, "dmesg")
	crcCmdOut, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to get crc value as empty the values got are %s: %s", err, crcCmdOut)
	}
	if strings.Contains(string(crcCmdOut), "CRC") {
		s.Fatal("Failed to verify CRC in dmesg")
	}

	youtubeURL := "https://www.youtube.com/watch?v=aqz-KE-bpKQ"
	ytbConn, err := cr.NewConn(ctx, youtubeURL)
	if err != nil {
		s.Fatal("Failed to open url in chrome browser: ", err)
	}
	defer ytbConn.Close()

	if err := webutil.WaitForYoutubeVideo(ctx, ytbConn, 0); err != nil {
		s.Fatal("Failed to wait for video element: ", err)
	}

	if err := verifyVideoPlay(ctx, ytbConn); err != nil {
		s.Fatal("Failed to play YouTube video: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to wait for video element: ", err)
	}

	if err := kb.Accel(ctx, "f"); err != nil {
		s.Fatal("Failed to press f(fullscreen) key: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	ui := uiauto.New(tconn)
	fullScreenText := nodewith.Name(`Exit full screen (f)`).Role(role.Button)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(fullScreenText)(ctx); err != nil {
		s.Fatal("Failed to check the existence of Exit full screen to valiadte the full screen mode: ", err)
	}

	const (
		psrDeepSleepStatus     = `Source PSR status.*DEEP_SLEEP.*`
		psrSleepStatus         = `Source PSR status:.SLEEP.*`
		psrDeepIdleSleepStatus = `Source PSR status(.*DEEP_SLEEP.*|.*IDLE.)`
	)

	videoStatus = "Playing in fullscreen"
	if err := psrCompatibleStatus(ctx, psrSleepStatus, videoStatus, false); err != nil {
		s.Fatal("Failed to check psrCompatible data: ", err)
	}

	if err := kb.Accel(ctx, "k"); err != nil {
		s.Fatal("Failed to press k(pause) key: ", err)
	}

	if err := verifyVideoPlay(ctx, ytbConn); err == nil {
		s.Fatal("Failed to pause YouTube video: ", err)
	}

	videoStatus = "video paused"
	if err := psrCompatibleStatus(ctx, psrDeepSleepStatus, videoStatus, false); err != nil {
		s.Fatal("Failed to verify psrCompatible data: ", err)
	}

	if err := kb.Accel(ctx, "k"); err != nil {
		s.Fatal("Failed to press k(play) key: ", err)
	}

	if err := verifyVideoPlay(ctx, ytbConn); err != nil {
		s.Fatal("Failed to play YouTube video: ", err)
	}

	videoStatus = "video playing"
	if err := psrCompatibleStatus(ctx, psrSleepStatus, videoStatus, false); err != nil {
		s.Fatal("Failed to check psrCompatible data: ", err)
	}

	if err := kb.Accel(ctx, "f"); err != nil {
		s.Fatal("Failed to press f key: ", err)
	}

	if err := ui.WithTimeout(10 * time.Second).WaitUntilGone(fullScreenText)(ctx); err != nil {
		s.Fatal("Failed to check the existence of Exit full screen to valiadte the full screen mode: ", err)
	}

	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		s.Fatal("Failed to signout: ", err)
	}

	videoStatus = "Dut in Signout state and video closed"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := psrCompatibleStatus(ctx, psrDeepIdleSleepStatus, videoStatus, false); err != nil {
			return errors.Wrap(err, "failed to check Psr deep idle sleep status after signout")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		s.Fatal("Failed to verify psrCompatible data: ", err)
	}
}

// psrCompatibleStatus function verifies PSR compatibilty when No video is playing, video playing, Dut in Signout state and video closed.
func psrCompatibleStatus(ctx context.Context, psrString, videoStatus string, sourceStatus bool) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		psrCompatibleFile := "/sys/kernel/debug/dri/0/i915_edp_psr_status"
		psrOutput, err := ioutil.ReadFile(psrCompatibleFile)
		if err != nil {
			return errors.Wrap(err, "failed to read Psr compatible file")
		}

		if sourceStatus {
			reStrings := []string{`Sink support.*yes.*`, `PSR mode.*enabled`, `Source PSR status.*DEEP_SLEEP.*`}

			for _, reString := range reStrings {
				re := regexp.MustCompile(reString)
				if !re.MatchString(string(psrOutput)) {
					return errors.Errorf("failed to find %s status data in psrOutput", reStrings)
				}
			}
		} else {
			re := regexp.MustCompile(psrString)
			if !re.MatchString(string(psrOutput)) {
				return errors.Errorf("failed to check %s after %s", psrString, videoStatus)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 250 * time.Millisecond}); err != nil {
		return errors.Wrap(err, "failed to check psrCompatible data")
	}
	return nil
}

// verifyVideoPlay functions verifies video play status.
func verifyVideoPlay(ctx context.Context, ytConn *chrome.Conn) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		const playingState = 1 // Playing state of the YouTube player.
		var playerState int
		getPlayerState := `document.getElementById('movie_player').getPlayerState()`
		if err := ytConn.Eval(ctx, getPlayerState, &playerState); err != nil {
			return errors.Wrap(err, "failed to get YouTube player state")
		}
		if playerState != playingState {
			return errors.New("YouTube video is not playing")
		}
		testing.ContextLog(ctx, "YouTube video is playing")
		return nil
	}, &testing.PollOptions{Timeout: 45 * time.Second, Interval: 2 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to verify video play status")
	}
	return nil
}
