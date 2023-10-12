// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	// Pre-requisite: Connect Type-A USB 3.0 pendrive and Type-A Headset to the DUT.
	testing.AddTest(&testing.Test{
		Func:         PlayMovieUsbTypeaPendriveHeadset,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies Play movie in USB type-A pen drive with USB type-A HS",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:intel-usb-set1"},
		Data:         []string{"bear-320x240.h264.mp4"},
		Vars:         []string{"intel.usbDetectionName"},
		Fixture:      "chromeLoggedIn",
		Timeout:      7 * time.Minute,
	})
}

func PlayMovieUsbTypeaPendriveHeadset(ctx context.Context, s *testing.State) {
	// Give 5 seconds to cleanup other resources.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	// Verify USB pendrive speed.
	usbDevicesList, err := usbutils.ListDevicesInfo(ctx, nil)
	if err != nil {
		s.Fatal("Failed to get USB devices list: ", err)
	}
	usbDeviceClassName := "Mass Storage"
	usbSpeed := "5000M"
	got := usbutils.NumberOfUSBDevicesConnected(usbDevicesList, usbDeviceClassName, usbSpeed)
	if want := 1; got != want {
		s.Fatalf("Unexpected number of USB devices connected: got %d, want %d", got, want)
	}

	usbDeviceName := s.RequiredVar("intel.usbDetectionName")
	const (
		mediaRemovable = "/media/removable/"
		videoFileName  = "bear-320x240.h264.mp4"
	)
	destinationFilePath := path.Join(mediaRemovable, usbDeviceName, videoFileName)

	if copyErr := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cp -rf %s %s", s.DataPath(videoFileName), destinationFilePath)).Run(); copyErr != nil {
		s.Fatalf("Failed to copy file to %s path: %v", destinationFilePath, copyErr)
	}
	defer os.Remove(destinationFilePath)

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}
	defer files.Close(cleanupCtx)

	if err := files.OpenDir(usbDeviceName, filesapp.FilesTitlePrefix+usbDeviceName)(ctx); err != nil {
		s.Fatal("Failed to open USB directory: ", err)
	}

	if err := files.OpenFile(videoFileName)(ctx); err != nil {
		s.Fatalf("Failed to open the audio file %q: %v", videoFileName, err)
	}
	cui := uiauto.New(tconn)
	togglePlayPause := nodewith.Name("Toggle play pause").Role(role.ToggleButton)
	if err := cui.LeftClick(togglePlayPause)(ctx); err != nil {
		s.Fatal("Failed to find and click togglePlayPause button: ", err)
	}

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object: ", err)
	}

	// Get current audio output device info.
	deviceName, deviceType, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get the selected audio device: ", err)
	}

	const expectedAudioOutputNode = "USB"
	if deviceType != expectedAudioOutputNode {
		if err := cras.SetActiveNodeByType(ctx, expectedAudioOutputNode); err != nil {
			s.Fatalf("Failed to select active device %s: %v", expectedAudioOutputNode, err)
		}
		deviceName, deviceType, err = cras.SelectedOutputDevice(ctx)
		if err != nil {
			s.Fatal("Failed to get the selected audio device: ", err)
		}
		if deviceType != expectedAudioOutputNode {
			s.Fatalf("Failed to set the audio node type: got %q; want %q", deviceType, expectedAudioOutputNode)
		}
	}

	out, err := testexec.CommandContext(ctx, "cras_test_client").Output()
	if err != nil {
		s.Fatal("Failed to exceute cras_test_client command: ", err)
	}
	re := regexp.MustCompile(`yes.*USB.*2\*`)
	if !re.MatchString(string(out)) {
		s.Fatal("Failed to select USB as output audio node")
	}

	if err := typecutils.VerifyAudioRoute(ctx, deviceName); err != nil {
		s.Fatalf("Failed to verify audio routing through %q: %v", expectedAudioOutputNode, err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard eventwriter: ", err)
	}

	var increaseSliderValue, decreasedSliderValue int

	sliderValue, err := quicksettings.SliderValue(ctx, tconn, quicksettings.SliderTypeVolume)
	if err != nil {
		s.Fatal("Failed to get slider value: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		decreasedSliderValue, err = quicksettings.DecreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeVolume)
		if err != nil {
			return errors.Wrap(err, "failed to DecreaseSlider")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to decrease slider: ", err)
	}
	if sliderValue <= decreasedSliderValue {
		s.Fatalf("Failed to decrease volume slider: got %d want lesser than %d", decreasedSliderValue, sliderValue)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		increaseSliderValue, err = quicksettings.IncreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeVolume)
		if err != nil {
			return errors.Wrap(err, "failed to IncreaseSlider")

		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to increase slider: ", err)
	}
	if decreasedSliderValue >= increaseSliderValue {
		s.Fatalf("Failed to increase volume slider: got %d want greater than %d", increaseSliderValue, decreasedSliderValue)
	}

}
