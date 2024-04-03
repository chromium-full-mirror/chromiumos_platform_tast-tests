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
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast/core/ctxutil"
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

	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	ui := uiauto.New(tconn)

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

	var videoSource = youtube.VideoSrc{
		URL:     "https://www.youtube.com/watch?v=aqz-KE-bpKQ",
		Title:   "Big Buck Bunny 60fps 4K - Official Blender Foundation Short Film",
		Quality: "2160p60",
	}
	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create clamshell action handler: ", err)
	}
	defer uiHandler.Close(ctx)

	videoApp := youtube.NewYtWeb(cr.Browser(), tconn, kb, false, ui, uiHandler)
	if err := videoApp.OpenAndPlayVideo(videoSource)(ctx); err != nil {
		s.Fatalf("Failed to open %s: %v", videoSource.URL, err)
	}
	defer videoApp.Close(cleanupCtx)

	if err := videoApp.IsPlaying()(ctx); err != nil {
		s.Fatal("Failed to play YouTube video: ", err)
	}

	if err := videoApp.EnterFullScreen(ctx); err != nil {
		s.Fatal("Failed to enter fullscreen: ", err)
	}

	fullScreenText := nodewith.Name(`Entered full screen`).Role("alert")
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
	if err := videoApp.Pause()(ctx); err != nil {
		s.Fatal("Failed to pause video: ", err)
	}

	videoStatus = "video paused"
	if err := psrCompatibleStatus(ctx, psrDeepSleepStatus, videoStatus, false); err != nil {
		s.Fatal("Failed to verify psrCompatible data: ", err)
	}

	if err := videoApp.Play()(ctx); err != nil {
		s.Fatal("Failed to play the video: ", err)
	}

	videoStatus = "video playing"
	if err := psrCompatibleStatus(ctx, psrSleepStatus, videoStatus, false); err != nil {
		s.Fatal("Failed to check psrCompatible data: ", err)
	}

	if err := videoApp.ExitFullScreen(ctx); err != nil {
		s.Fatal("Failed to exit full screen: ", err)
	}

	fullScreenExitText := nodewith.Name(`Exited full screen`).Role("alert")
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(fullScreenExitText)(ctx); err != nil {
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
