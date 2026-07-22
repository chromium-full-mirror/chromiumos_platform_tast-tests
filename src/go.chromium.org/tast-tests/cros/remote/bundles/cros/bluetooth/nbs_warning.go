// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	qs "go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type btNBSWarningTestCase struct {
	enableWBS bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         NbsWarning,
		Desc:         "Verifies when an NBS device is connected, a warning is shown in the QS",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "jrwu@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:bluetooth"},
		HardwareDeps: hwdep.D(hwdep.SkipOnFormFactor(hwdep.Chromebox)),
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.bluetooth.BluetoothUIService",
			"tast.cros.ui.AudioService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
		},
		Timeout:         5 * time.Minute,
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
		Params: []testing.Param{
			{
				Name:      "floss_disabled_wbs_disabled",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &btNBSWarningTestCase{
					enableWBS: false,
				},
			},
			{
				Name:      "floss_disabled_wbs_enabled",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val: &btNBSWarningTestCase{
					enableWBS: true,
				},
			},
			{
				Name:      "floss_enabled_wbs_disabled",
				Fixture:   "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &btNBSWarningTestCase{
					enableWBS: false,
				},
			},
			{
				Name:      "floss_enabled_wbs_enabled",
				Fixture:   "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr: []string{"bluetooth_floss_flaky"},
				Val: &btNBSWarningTestCase{
					enableWBS: true,
				},
			},
		},
	})
}

func selectInternalMic(ctx context.Context, qsSvc qs.QuickSettingsServiceClient) error {
	_, err := qsSvc.SelectNthAudioOption(
		ctx, &qs.SelectNthAudioOptionRequest{
			AudioNodeName: "Microphone (internal)",
			Nth:           0,
		})

	return err
}

func selectBTMic(ctx context.Context, qsSvc qs.QuickSettingsServiceClient) error {
	_, err := qsSvc.SelectNthAudioOption(
		ctx, &qs.SelectNthAudioOptionRequest{
			AudioNodeName: "RASPI_AUDIO",
			Nth:           1,
		})

	return err
}

// NbsWarning verifies when a NBS device is connected, a warning is shown in the QS.
func NbsWarning(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	tc := s.Param().(*btNBSWarningTestCase)

	adSvc := fv.AudioService
	btUISvc := fv.BluetoothUIService
	qsSvc := fv.QuickSettingsService

	if _, err := adSvc.SetWBSEnabled(
		ctx, &ui.AudioServiceRequest{
			WBSEnabled: tc.enableWBS,
		}); err != nil {
		s.Fatal("Failed to change WBS support: ", err)
	}

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

	if _, err := btUISvc.PairDeviceWithQuickSettings(ctx, &bts.PairDeviceWithQuickSettingsRequest{
		AdvertisedName: emulatedDevice.AdvertisedName(),
	}); err != nil {
		s.Fatal("Failed to pair device: ", err)
	}

	// wait until device is selectable
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		if e := selectBTMic(ctx, qsSvc); e != nil {
			return errors.New("unable to set active node to BT mic")
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  30 * time.Second,
		Interval: 5 * time.Second,
	}); err != nil {
		s.Fatal("Failed to select device: ", err)
	}

	expectWarning := !tc.enableWBS

	// verify if warning is shown as expected
	if err = testing.Poll(ctx, func(ctx context.Context) error {
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
		Interval: 5 * time.Second,
	}); err != nil {
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
