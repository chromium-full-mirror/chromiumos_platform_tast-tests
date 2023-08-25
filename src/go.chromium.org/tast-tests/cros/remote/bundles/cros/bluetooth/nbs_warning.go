// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	qs "go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/durationpb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NbsWarning,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies when an NBS device is connected, a warning is shown in the QS",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "jrwu@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:bluetooth",
			"bluetooth_btpeers_1",
		},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.ui.AudioService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
		},
		Timeout: 3 * time.Minute,
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

func pairDevice(ctx context.Context, dev *bluetooth.EmulatedBTPeerDevice, btSvc bts.BluetoothServiceClient) error {
	if _, err := btSvc.DiscoverDevice(ctx, &bts.DiscoverDeviceRequest{
		DeviceAddress:    dev.LocalBluetoothAddress(),
		DiscoveryTimeout: durationpb.New(45 * time.Second),
	}); err != nil {
		return errors.Wrap(err, "failed to discover device")
	}

	err := testing.Poll(ctx, func(ctx context.Context) error {
		_, pairError := btSvc.PairDevice(ctx, &bts.PairDeviceRequest{
			DeviceAddress: dev.LocalBluetoothAddress(),
			Pin:           dev.PinCode(),
		})
		return pairError
	}, &testing.PollOptions{
		Timeout:  1 * time.Minute,
		Interval: 5 * time.Second,
	})

	return err
}

func reconnectAndSelectBluetoothMic(ctx context.Context, dev *bluetooth.EmulatedBTPeerDevice, btSvc bts.BluetoothServiceClient, qsSvc qs.QuickSettingsServiceClient) error {
	err := testing.Poll(ctx, func(ctx context.Context) error {
		reconnectPairedDevice(ctx, dev, btSvc)

		_, selectError := qsSvc.SelectNthAudioOption(
			ctx, &qs.SelectNthAudioOptionRequest{
				AudioNodeName: dev.AdvertisedName(),
				Nth:           1,
			})

		return selectError
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: 1 * time.Second,
	})

	return err
}

func selectInternalMic(ctx context.Context, qsSvc qs.QuickSettingsServiceClient) error {
	_, err := qsSvc.SelectNthAudioOption(
		ctx, &qs.SelectNthAudioOptionRequest{
			AudioNodeName: "Microphone (internal)",
			Nth:           0,
		})

	return err
}

func disconnectPairedDevice(ctx context.Context, dev *bluetooth.EmulatedBTPeerDevice, btSvc bts.BluetoothServiceClient) error {
	if _, err := btSvc.DisconnectDevice(ctx, &bts.DisconnectDeviceRequest{
		DeviceAddress: dev.LocalBluetoothAddress(),
	}); err != nil {
		return errors.Wrap(err, "failed to disconnect device")
	}

	err := testing.Poll(ctx, func(ctx context.Context) error {
		connected, checkErr := isDeviceConnected(ctx, dev, btSvc)
		if checkErr != nil {
			return errors.Wrap(checkErr, "failed to verify device connection after disconnecting")
		}
		if connected {
			return errors.New("Device is still connected after disconnection: ")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: 1 * time.Second,
	})

	return err
}

func connectPairedDevice(ctx context.Context, dev *bluetooth.EmulatedBTPeerDevice, btSvc bts.BluetoothServiceClient) error {
	if _, err := btSvc.ConnectDevice(ctx, &bts.ConnectDeviceRequest{
		DeviceAddress: dev.LocalBluetoothAddress(),
	}); err != nil {
		return errors.Wrap(err, "failed to connect device")
	}

	err := testing.Poll(ctx, func(ctx context.Context) error {
		connected, checkErr := isDeviceConnected(ctx, dev, btSvc)
		if checkErr != nil {
			return errors.Wrap(checkErr, "failed to verify device connection after connecting")
		}
		if !connected {
			return errors.New("Device is not connected after connection")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: 1 * time.Second,
	})

	return err
}

func reconnectPairedDevice(ctx context.Context, dev *bluetooth.EmulatedBTPeerDevice, btSvc bts.BluetoothServiceClient) error {
	if err := disconnectPairedDevice(ctx, dev, btSvc); err != nil {
		return errors.Wrap(err, "failed to disconnect in reconnecting")
	}

	if err := connectPairedDevice(ctx, dev, btSvc); err != nil {
		return errors.Wrap(err, "failed to connect in reconnecting")
	}

	return nil
}

// NbsWarning verifies when a NBS device is connected, a warning is shown in the QS.
func NbsWarning(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	adSvc := fv.AudioService
	btSvc := fv.BluetoothService
	qsSvc := fv.QuickSettingsService

	emulatedDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0],
		&bluetooth.EmulatedBTPeerDeviceConfig{DeviceType: cbt.DeviceTypeBluetoothAudio})
	if err != nil {
		s.Fatal("Failed to emulate the device type: ", err)
	}

	if _, err := emulatedDevice.RPCAudio().StartOfono(ctx); err != nil {
		s.Fatal("Failed to start Ofono: ", err)
	}

	if _, err := emulatedDevice.RPCAudio().StartPulseaudio(ctx, "hfp_wbs"); err != nil {
		s.Fatal("Failed to start Pulseaudio: ", err)
	}

	if err := pairDevice(ctx, emulatedDevice, btSvc); err != nil {
		s.Fatal("Failed to pair device: ", err)
	}

	WBSTests := []bool{true, false}
	for _, enableWBS := range WBSTests {
		if _, err := adSvc.SetWBSEnabled(
			ctx, &ui.AudioServiceRequest{
				WBSEnabled: enableWBS,
			}); err != nil {
			s.Fatal("Failed to change WBS support: ", err)
		}

		// reconnect to reflect the capability change
		if err := reconnectAndSelectBluetoothMic(ctx, emulatedDevice, btSvc, qsSvc); err != nil {
			s.Fatal("Failed to reconnect and select BT mic: ", err)
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
	}
}
