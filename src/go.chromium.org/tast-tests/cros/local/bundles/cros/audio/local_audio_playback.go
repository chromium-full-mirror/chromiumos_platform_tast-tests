// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type localAudioParams struct {
	expectedOutputDeviceUI string
	verifyPendrive         bool
	expectedOutputDevice   string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalAudioPlayback,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Play local audio file through default app and check if the audio is routing through expected device",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "bailideng@google.com"},
		BugComponent: "b:776546",
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		Data:         []string{"audio.mp3"},
		Vars:         []string{"audio.usbDetectionName"},
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		Params: []testing.Param{{
			Name:      "internal_speaker",
			ExtraAttr: []string{"group:mainline", "informational", "group:intel-gating", "group:intel-nda"},
			Val:       localAudioParams{"Speaker (internal)", false, "INTERNAL_SPEAKER"},
		}, {
			Name:      "headphone",
			Val:       localAudioParams{"Headphone", false, "HEADPHONE"},
			ExtraAttr: []string{"group:intel-jack"},
		}, {
			Name:      "usb_speaker",
			Val:       localAudioParams{"USB", false, "USB"},
			ExtraAttr: []string{"group:intel-usb-set2"},
		}, {
			Name:      "usb_pendrive",
			Val:       localAudioParams{"Speaker (internal)", true, "INTERNAL_SPEAKER"},
			ExtraAttr: []string{"group:intel-type-c-usb"},
		}},
	})
}

// LocalAudioPlayback generates audio file and plays it through default audio player.
// Switching nodes via UI interactions is the recommended way, instead of using
// cras.SetActiveNode() method, as UI will always send the preference input/output
// devices to CRAS. Calling cras.SetActiveNode() changes the active devices for a
// moment, but they soon are reverted by UI. See (b/191602192) for details.
func LocalAudioPlayback(ctx context.Context, s *testing.State) {
	testParam := s.Param().(localAudioParams)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	timeForCleanup := 10 * time.Second
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, timeForCleanup)
	defer cancel()

	// Open the test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}
	defer crastestclient.Unmute(cleanupCtx)

	if testParam.verifyPendrive {
		// Verify USB pendrive speed.
		usbDevicesList, err := usbutils.ListDevicesInfo(ctx, nil)
		if err != nil {
			s.Fatal("Failed to get USB devices list: ", err)
		}
		const (
			mediaRemovable     = "/media/removable/"
			audioFileName      = "audio.mp3"
			usbDeviceClassName = "Mass Storage"
			usbSpeed           = "5000M"
		)
		got := usbutils.NumberOfUSBDevicesConnected(usbDevicesList, usbDeviceClassName, usbSpeed)
		if want := 1; got != want {
			s.Fatalf("Unexpected number of USB devices connected: got %d, want %d", got, want)
		}

		usbDeviceName := s.RequiredVar("audio.usbDetectionName")

		destinationFilePath := path.Join(mediaRemovable, usbDeviceName, audioFileName)

		if err := usbutils.TransferFile(ctx, s.DataPath(audioFileName), destinationFilePath, false); err != nil {
			s.Fatalf("Failed to copy file to %s path: %v", destinationFilePath, err)
		}
		defer os.Remove(destinationFilePath)

		files, err := filesapp.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch the Files App: ", err)
		}
		defer files.Close(cleanupCtx)

		if err := uiauto.Combine("Open the audio files",
			files.OpenDir(usbDeviceName, filesapp.FilesTitlePrefix+usbDeviceName),
			files.OpenFile(audioFileName),
		)(ctx); err != nil {
			s.Fatalf("Failed to open the audio file %q: %v", audioFileName, err)
		}

		if err := verifyAudioRouting(ctx, testParam.expectedOutputDevice); err != nil {
			s.Fatal("Timeout waiting for USB pendrive detection: ", err)
		}
		if err := files.Close(ctx); err != nil {
			s.Fatal("Failed to close: ", err)
		}
	}
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	wavFileName := "30SEC.wav"
	if err := generateAudioTestData(ctx, downloadsPath, wavFileName); err != nil {
		s.Fatal("Failed to generate audio file: ", err)
	}
	defer os.Remove(downloadsPath)

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}
	defer files.Close(cleanupCtx)
	if err := files.OpenDownloads()(ctx); err != nil {
		s.Fatal("Failed to open Downloads folder in files app: ", err)
	}
	// Open the audio file.
	// The audio file automatically plays once it is opened in the gallery app.
	if err := files.OpenFile(wavFileName)(ctx); err != nil {
		s.Fatalf("Failed to open the audio file %q: %v", wavFileName, err)
	}
	// Closing the audio player.
	defer func() {
		if kb.Accel(cleanupCtx, "Ctrl+W"); err != nil {
			s.Error("Failed to close Audio player: ", err)
		}
	}()

	// Select output device.
	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show Quick Settings")
	}
	defer quicksettings.Hide(cleanupCtx, tconn)

	if err := quicksettings.SelectAudioOption(ctx, tconn, testParam.expectedOutputDeviceUI); err != nil {
		s.Fatal("Failed to select audio option: ", err)
	}
	if err := verifyAudioRouting(ctx, testParam.expectedOutputDevice); err != nil {
		s.Fatal("Failed to verify audio routing: ", err)
	}
}

// verifyAudioRouting function verifies whether audio is routing or not on specified audio device.
func verifyAudioRouting(ctx context.Context, expectedAudioNode string) error {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create cras object")
	}
	deviceName, deviceType, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the selected audio device")
	}
	if deviceType != expectedAudioNode {
		if err := cras.SetActiveNodeByType(ctx, expectedAudioNode); err != nil {
			return errors.Wrapf(err, "failed to select active device %s", expectedAudioNode)
		}
		deviceName, deviceType, err = cras.SelectedOutputDevice(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get the selected audio device")
		}
		if deviceType != expectedAudioNode {
			return errors.Wrapf(err, "failed to set the audio node type: got %q; want %q", deviceType, expectedAudioNode)
		}
	}
	devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
	if err != nil {
		return errors.Wrap(err, "failed to detect running output device")
	}
	if deviceName != devName {
		return errors.Errorf("failed to route the audio through expected audio node: got %q; want %q", devName, deviceName)
	}
	return nil
}

func generateAudioTestData(ctx context.Context, downloadsPath, wavFileName string) error {
	// Generate sine raw input file that lasts 30 seconds.
	rawFileName := "30SEC.raw"
	rawFilePath := filepath.Join(downloadsPath, rawFileName)
	rawFile := audio.TestRawData{
		Path:          rawFilePath,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Frequencies:   []int{440, 440},
		Volume:        0.05,
		Duration:      30,
	}
	if err := audio.GenerateTestRawData(ctx, rawFile); err != nil {
		return errors.Wrap(err, "failed to generate audio test data")
	}

	wavFile := filepath.Join(downloadsPath, wavFileName)
	if err := audio.ConvertRawToWav(ctx, rawFile, wavFile); err != nil {
		return errors.Wrap(err, "failed to convert raw to wav")
	}
	return nil
}
