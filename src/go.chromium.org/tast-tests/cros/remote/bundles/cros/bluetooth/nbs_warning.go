// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	qs "go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NbsWarning,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies when an NBS device is connected, a warning is shown in the QS",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "jrwu@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:bluetooth", "bluetooth_btpeers_1"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.bluetooth.BluetoothUIService",
			"tast.cros.ui.AudioService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
		},
		Timeout: 5 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name:              "floss_enabled",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

func isDeviceConnected(ctx context.Context, dev *bluetooth.EmulatedBTPeerDevice, btSvc bts.BluetoothServiceClient) (bool, error) {
	connected, err := btSvc.DeviceIsConnected(ctx, &bts.DeviceIsConnectedRequest{
		DeviceAddress: dev.LocalBluetoothAddress(),
	})

	if err != nil {
		return false, errors.Wrap(err, "failed to check if device is connected")
	}

	return connected.GetDeviceIsConnected(), nil
}

func selectInternalMic(ctx context.Context, qsSvc qs.QuickSettingsServiceClient) error {
	_, err := qsSvc.SelectNthAudioOption(
		ctx, &qs.SelectNthAudioOptionRequest{
			AudioNodeName: "Microphone (internal)",
			Nth:           0,
		})

	return err
}

// NbsWarning verifies when a NBS device is connected, a warning is shown in the QS.
func NbsWarning(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	adSvc := fv.AudioService
	btUISvc := fv.BluetoothUIService
	qsSvc := fv.QuickSettingsService

	emulatedDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0],
		&bluetooth.EmulatedBTPeerDeviceConfig{DeviceType: cbt.DeviceTypeBluetoothAudio})
	if err != nil {
		s.Fatal("Failed to emulate the device type: ", err)
	}

	if err := emulatedDevice.RPCAudio().StartPulseaudio(ctx, cbt.AudioProfileHFPWBS); err != nil {
		s.Fatal("Failed to start Pulseaudio: ", err)
	}

	if err := emulatedDevice.RPCAudio().StartOfono(ctx); err != nil {
		s.Fatal("Failed to start Ofono: ", err)
	}

	WBSTests := []bool{true, false}
	for _, enableWBS := range WBSTests {
		if _, err := adSvc.SetWBSEnabled(
			ctx, &ui.AudioServiceRequest{
				WBSEnabled: enableWBS,
			}); err != nil {
			s.Fatal("Failed to change WBS support: ", err)
		}

		if _, err := btUISvc.PairDeviceWithQuickSettings(ctx, &bts.PairDeviceWithQuickSettingsRequest{
			AdvertisedName: emulatedDevice.AdvertisedName(),
		}); err != nil {
			s.Fatal("Failed to pair device: ", err)
		}

		audioDevice, err := adSvc.AudioCrasSelectedInputDevice(ctx, &emptypb.Empty{})
		if err != nil {
			s.Fatal("Failed to get input audio device info: ", err)
		}

		expectWarning := audioDevice.DeviceType == "BLUETOOTH_NB_MIC"

		// verify if warning is shown as expected
		err = testing.Poll(ctx, func(ctx context.Context) error {
			res, checkErr := qsSvc.IsNBSWarningShown(ctx, &emptypb.Empty{})
			if checkErr != nil {
				return checkErr
			}
			if !expectWarning && res.GetIsNbsWarningShown() {
				return errors.New("the NBS warning should not be shown in Quick Settings")
			}
			if expectWarning && !res.GetIsNbsWarningShown() {
				return errors.New("the NBS warning should be shown in Quick Settings")
			}
			return nil
		}, &testing.PollOptions{
			Timeout:  30 * time.Second,
			Interval: 1 * time.Second,
		})

		if err != nil {
			s.Fatal("Unexpected: ", err)
		}

		if err := selectInternalMic(ctx, qsSvc); err != nil {
			s.Fatal("Failed to select internal mic: ", err)
		}

		res, err := qsSvc.IsNBSWarningShown(ctx, &emptypb.Empty{})
		if err != nil {
			s.Fatal("Failed to check whether the NBS warning is shown: ", err)
		}
		if res.GetIsNbsWarningShown() {
			s.Fatal("The NBS warning should not be shown when internal mic is chosen")
		}

		btUISvc.ForgetBluetoothDevice(ctx, &bts.ForgetBluetoothDeviceRequest{DeviceName: emulatedDevice.AdvertisedName()})
	}
}
