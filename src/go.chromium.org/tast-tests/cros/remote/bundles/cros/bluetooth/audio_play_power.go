// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/bluetoothutil"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioPlayPower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure Bluetooth audio play power consumption",
		Contacts: []string{
			"chromeos-bt-team@google.com",
			"jiangzp@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth", "bluetooth_btpeers_1"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.bluetooth.BluetoothService", "tast.cros.power.MetricsService", "tast.cros.ui.AudioService"},
		HardwareDeps: hwdep.D(hwdep.Battery()),
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "audioUIEnabledWith1BTPeerPowerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name:              "floss_enabled",
				Fixture:           "audioUIEnabledWith1BTPeerPowerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
	})
}

// AudioPlayPower tests power consumption when audio is played through Bluetooth.
func AudioPlayPower(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	btpeer := fv.BTPeers[0]

	const audioPlayIntervalSec = 300 // Audio play time for the power measurement

	// Set bluetoothd config execution flags for audio.
	if err := btpeer.ChameleondClient().BluetoothAudioDevice().ResetStack(ctx, cbt.DeviceTypeBluetoothAudio.String()); err != nil {
		s.Fatal("Fail to reset stack: ", err)
	}

	s.Log("Prepare peer")
	// Emulate the bluetooth audio device with btpeer.
	s.Logf("Configuring a btpeer as %q device", cbt.DeviceTypeBluetoothAudio.String())
	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, btpeer, &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeBluetoothAudio,
	})
	if err != nil {
		s.Fatal("Failed to call NewEmulatedBTPeerDevice: ", err)
	}
	s.Logf("Device %s is ready to pair", device.String())

	if err := bluetoothutil.ConfigureAudioDevice(ctx, device, cbt.AudioProfileA2DP, "", ""); err != nil {
		s.Fatal("Failed to config audio device: ", err)
	}

	if err := device.RPCAudio().SetDiscoverable(ctx, true); err != nil {
		s.Fatal("Set discoverable failed")
	}

	// Attempt pairing device with DUT.
	s.Logf("Paring device %s", device.String())
	if err := bluetoothutil.DiscoverAndPairDevice(ctx, fv.BluetoothService, device.LocalBluetoothAddress(), device.PinCode(), bluetoothutil.DiscoverAndPairTimeout); err != nil {
		s.Fatalf("Failed to discover and pair device %s: %v", device.String(), err)
	}
	s.Logf("Successfully paired device %s", device.String())

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		id, err := device.RPCAudio().GetBluezSourceA2DPDevice(ctx, cbt.AudioProfileA2DP)
		if err != nil {
			return errors.New("failed to get a2dp BlueZ source")
		}
		s.Log("A2dp BlueZ source device number ", id)
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 4 * time.Second}); err != nil {
		s.Fatal("Failed to get a2dp BlueZ source: ", err)
	}

	audioService := fv.AudioService

	const expectedAudioOuputNode = "BLUETOOTH"

	deviceName, err := audioService.AudioCrasSelectedOutputDevice(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get output audio device info: ", err)
	}
	s.Log("Selected output device type: ", deviceName.DeviceType)
	if deviceName.DeviceType != expectedAudioOuputNode {
		s.Log("Change to output node: ", expectedAudioOuputNode)
		expectedAudioNode := ui.AudioServiceRequest{Expr: expectedAudioOuputNode}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := audioService.SetActiveNodeByType(ctx, &expectedAudioNode); err != nil {
				return errors.New("fail to select output audio node")
			}
			currentName, err := audioService.AudioCrasSelectedOutputDevice(ctx, &empty.Empty{})
			if err != nil {
				return errors.New("fail to get output audio device info")
			}
			if currentName.DeviceType != expectedAudioOuputNode {
				return errors.New("failed to select audio device")
			}
			return nil
		}, &testing.PollOptions{Timeout: 20 * time.Second, Interval: 4 * time.Second}); err != nil {
			s.Fatal("Failed to select audio device ", expectedAudioOuputNode, err)
		}
	}

	interval := time.Duration(audioPlayIntervalSec) * time.Second

	// Generate sine raw input file that lasts audioPlayIntervalSec.
	const rawFileName = "AudioFile.raw"
	res, err := audioService.DownloadsPath(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal(s, "Failed to return download path: ", err)
	}
	downloadsPath := res.DownloadsPath
	rawFilePath := filepath.Join(downloadsPath, rawFileName)

	rawDataFields := &ui.AudioServiceRequest{FilePath: rawFilePath, DurationInSecs: audioPlayIntervalSec}
	if _, err := audioService.GenerateTestRawData(ctx, rawDataFields); err != nil {
		s.Fatal("Failed to generate test raw data file: ", err)
	}

	const wavFileName = "AudioFile.wav"
	wavFile := filepath.Join(downloadsPath, wavFileName)
	convertRawFileFields := &ui.AudioServiceRequest{FilePath: rawFilePath, FileName: wavFile}
	if _, err := audioService.ConvertRawToWav(ctx, convertRawFileFields); err != nil {
		s.Fatal("Failed to convert raw to wav: ", err)
	}

	fv.StartPowerRecording(ctx)

	s.Log("Keep idle for ", interval)
	// GoBigSleepLint: sleep for measuring power consumption
	testing.Sleep(ctx, interval)

	pResultsIdle, err := fv.StopPowerRecording(ctx, s.TestName())
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pIdle, err := fv.GetPowerMetrics(ctx, pResultsIdle, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	s.Log("Measured idle power [W]: ", pIdle)

	fv.StartPowerRecording(ctx)

	dirAndFileName := &ui.AudioServiceRequest{DirectoryName: "Downloads", FileName: wavFileName}
	if _, err := audioService.OpenDirectoryAndFile(ctx, dirAndFileName); err != nil {
		fv.StopPowerRecording(ctx, s.TestName())
		s.Fatal("Failed to open local audio file: ", err)
	}

	runningDeviceName := &ui.AudioServiceRequest{Expr: deviceName.DeviceName}
	if _, err := audioService.VerifyFirstRunningDevice(ctx, runningDeviceName); err != nil {
		fv.StopPowerRecording(ctx, s.TestName())
		s.Fatalf("Failed to route audio through %q: %v", expectedAudioOuputNode, err)
	}

	s.Log("Keep recording power while music is running for ", interval)
	// GoBigSleepLint: sleep for measuring power consumption
	testing.Sleep(ctx, interval)

	pResults, err := fv.StopPowerRecording(ctx, s.TestName())
	if err != nil {
		s.Fatal("Failed to measure power consumption: ", err)
	}
	pOn, err := fv.GetPowerMetrics(ctx, pResults, "system")
	if err != nil {
		s.Fatal("Failed to read power: ", err)
	}
	s.Log("Measured system power with BT audio [W]: ", pOn)
	// TODO: (b/301167351) Collect data to define baselines for test fail/pass.
	s.Log("Bluetooth audio play power [W]: ", pOn-pIdle)
}
