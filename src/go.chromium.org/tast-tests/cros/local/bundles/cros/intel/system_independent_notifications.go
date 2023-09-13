// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SystemIndependentNotifications,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "System independent route Notifications",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:intel-jack"},
		Fixture:      "chromeLoggedIn",
		Timeout:      7 * time.Minute,
	})
}

// SystemIndependentNotifications requires the following H/W topology to run.
// DUT ---> 3.5mm Jack headphone.
func SystemIndependentNotifications(ctx context.Context, s *testing.State) {
	const (
		audioRate    = 48000
		audioChannel = 2
		duration     = 30
	)

	// Give 5 seconds to cleanup other resources.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	output, err := testexec.CommandContext(ctx, "aplay", "-l").Output()
	if err != nil {
		s.Fatal("Failed to execute aplay command: ", err)
	}

	if !strings.Contains(string(output), "Headset") {
		s.Fatal("Failed to find Headset in aplay command")
	}

	re := regexp.MustCompile(`card (\d+):`)
	cardInfo := re.FindAllStringSubmatch(string(output), -1)
	if cardInfo == nil {
		s.Fatal("Failed to get card info from aplay command")
	}
	cardNo := cardInfo[0][1]

	reDevice := regexp.MustCompile(`device (\d+):\sHeadset`)
	deviceInfo := reDevice.FindAllStringSubmatch(string(output), -1)
	if deviceInfo == nil {
		s.Fatal("Failed to get device info from aplay command")
	}
	deviceNo := deviceInfo[0][1]

	const expectedHeadphoneAudioNode = "HEADPHONE"
	_, _, err = audioCrasSelectedOutputDevice(ctx, expectedHeadphoneAudioNode)
	if err != nil {
		s.Fatal("Failed to verify selected auido output device: ", err)
	}

	const expectedAudioUINode = "Speaker (internal)"
	if err := setActiveNodeByUI(ctx, tconn, expectedAudioUINode); err != nil {
		s.Fatal("Failed to set audio node using UI: ", err)
	}

	// Here open yoputube and play the video.
	var videoSource = youtube.VideoSrc{
		URL:     "https://www.youtube.com/watch?v=LXb3EKWsInQ",
		Title:   "COSTA RICA IN 4K 60fps HDR (ULTRA HD)",
		Quality: "1440p60",
	}

	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create clamshell action handler: ", err)
	}
	defer uiHandler.Close(ctx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard input: ", err)
	}

	cui := uiauto.New(tconn)
	videoApp := youtube.NewYtWeb(cr.Browser(), tconn, kb, false, cui, uiHandler)
	if err := videoApp.OpenAndPlayVideo(videoSource)(ctx); err != nil {
		s.Fatalf("Failed to open %s: %v", videoSource.URL, err)
	}
	defer videoApp.Close(cleanupCtx)

	const expectedAudioNode = "INTERNAL_SPEAKER"
	deviceName, _, err := audioCrasSelectedOutputDevice(ctx, expectedAudioNode)
	if err != nil {
		s.Fatal("Failed to verify selected auido output device: ", err)
	}
	if err := verifyFirstRunningDevices(ctx, deviceName); err != nil {
		s.Fatal("Failed to verify first running device: ", err)
	}

	// Sets to headphone using cardnumber by linking to soc and converting analog to digital signals.
	if err := testexec.CommandContext(ctx, "sh", "-c", "amixer c0 cset name='IF1 01 ADC Swap Mux' L/L").Run(); err != nil {
		s.Fatal("Failed to set headphone from card number: ", err)
	}
	// Allows Headphone jack to switch on.
	if err := testexec.CommandContext(ctx, "sh", "-c", "amixer -c0 cset name='Headphone Jack Switch' on").Run(); err != nil {
		s.Fatal("Failed to switch on headphone jack: ", err)
	}
	// Switches to left side of headphone by converting digital signls to analog.
	if err := testexec.CommandContext(ctx, "sh", "-c", "amixer -c0 cset name='Stereo1 DAC MIXL DAC L1 Switch' 1").Run(); err != nil {
		s.Fatal("Failed to switch to left side of headphone: ", err)
	}
	// Switches to right side of headphone by converting digital signals to analog.
	if err := testexec.CommandContext(ctx, "sh", "-c", "amixer -c0 cset name='Stereo1 DAC MIXL DAC R1 Switch' 1").Run(); err != nil {
		s.Fatal("Failed to switch to right side of headphone: ", err)
	}

	// Generate sine raw input file that lasts 30 seconds.
	rawTempFile, err := ioutil.TempFile("", "30SEC_*.raw")
	if err != nil {
		s.Error("Failed to create raw temp file: ", err)
	}
	if err := rawTempFile.Close(); err != nil {
		s.Error("Failed to close raw temp file: ", err)
	}
	rawFile := audio.TestRawData{
		Path:          rawTempFile.Name(),
		BitsPerSample: 16,
		Channels:      audioChannel,
		Rate:          audioRate,
		Frequencies:   []int{440, 440},
		Volume:        100.0,
		Duration:      duration,
	}
	if err := audio.GenerateTestRawData(ctx, rawFile); err != nil {
		s.Fatal("Failed to generate audio test data: ", err)
	}

	wavTempFile, err := ioutil.TempFile("", "30SEC_*.wav")
	if err != nil {
		s.Error("Failed to create wav temp file: ", err)
	}

	if err := wavTempFile.Close(); err != nil {
		s.Error("Failed to close wav temp file: ", err)
	}
	if err := audio.ConvertRawToWav(ctx, rawTempFile.Name(), wavTempFile.Name(), audioRate, audioChannel); err != nil {
		s.Fatal("Failed to convert raw to wav: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	recWavFileName := "30SEC_REC.wav"
	destinationFilePath := filepath.Join(downloadsPath, recWavFileName)
	recordCommand := fmt.Sprintf("play -c %d -r %d %s & rec -r %d -c %d %s trim 0 30", audioChannel, audioRate, wavTempFile.Name(), audioRate, audioChannel, destinationFilePath)
	recCmd := testexec.CommandContext(ctx, "sh", "-c", recordCommand)
	if err := recCmd.Run(); err != nil {
		s.Fatalf("Failed to execute %q: %v", recordCommand, err)
	}

	playViaHeadphoneCommand := fmt.Sprintf("aplay -Dhw:%s,%s -f dat %s", cardNo, deviceNo, destinationFilePath)
	playCmd := testexec.CommandContext(ctx, "sh", "-c", playViaHeadphoneCommand)
	if err := playCmd.Run(); err != nil {
		s.Fatalf("Failed to play auido through 3.5mm jack headphone %q: %v", playViaHeadphoneCommand, err)
	}
	defer os.Remove(rawTempFile.Name())
	defer os.Remove(wavTempFile.Name())
	defer os.Remove(destinationFilePath)
}

// audioCrasSelectedOutputDevice returns the active output device name and type and verifies with expectedAudioOuputNode.
func audioCrasSelectedOutputDevice(ctx context.Context, expectedAudioOuputNode string) (string, string, error) {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to create Cras object")
	}
	deviceName, deviceType, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get the selected audio device")
	}

	if deviceType != expectedAudioOuputNode {
		return "", "", errors.Wrapf(err, "failed to select audio device %q", expectedAudioOuputNode)
	}
	return deviceName, deviceType, nil
}

// setActiveNodeByUI sets active audio by UI.
func setActiveNodeByUI(ctx context.Context, tconn *chrome.TestConn, expectedAudioNode string) error {
	quicksettings.Hide(ctx, tconn)
	quicksettings.Show(ctx, tconn)

	if err := quicksettings.SelectAudioOption(ctx, tconn, expectedAudioNode); err != nil {
		return errors.Wrapf(err, "failed to select %q as audio output node", expectedAudioNode)
	}
	quicksettings.Hide(ctx, tconn)
	return nil
}

// verifyFirstRunningDevices verifies first running device.
func verifyFirstRunningDevices(ctx context.Context, runningDeviceName string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
		if err != nil {
			return errors.Wrap(err, "failed to detect running output device")
		}

		if deviceName := runningDeviceName; deviceName != devName {
			return errors.Wrapf(err, "failed to route the audio through expected audio node: got %q; want %q", devName, deviceName)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to check audio running device")
	}
	return nil
}
