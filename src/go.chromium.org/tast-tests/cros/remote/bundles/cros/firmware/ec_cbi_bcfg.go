// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type bcfgData struct {
	battManufName  string
	battDeviceName string
	bytes          []byte
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ECCbiBcfg,
		Desc: "Test that battery config is stored in CBI, and is accessible",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT

		TestBedDeps: tbdep.ServoPresentAndWorking,
		// TODO: b/317891316 When stable, change firmware_unstable to a different attr.
		Attr:    []string{"group:firmware", "firmware_unstable"},
		Fixture: fixture.NormalMode,
		Timeout: 15 * time.Minute,
		// Only run on platforms that include CL crrev/c/1234747 so that CBI can be reversibly written to.
		HardwareDeps: hwdep.D(hwdep.ECBuildConfigOptions(
			"BATTERY_CONFIG_IN_CBI",
			"PLATFORM_EC_BATTERY_CONFIG_IN_CBI",
		)),
	})
}

func ECCbiBcfg(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	s.Log("Disabling write protect")
	if err := h.SetECWriteProtect(ctx, false); err != nil {
		s.Fatal("Failed to disable write protect: ", err)
	}

	batteryConfigTag := "12"
	modifiedTag := false
	modifiedBcfgDataFile := "modifiedBcfgData.json"

	originalBcicData, err := readBatteryConfigFromCbi(ctx, h, batteryConfigTag)
	if err != nil {
		s.Fatal("Expected read to succeed: ", err)
	}
	originalBcicData = strings.ReplaceAll(originalBcicData, "\n", "")
	s.Logf("CBI data for battery config tag %q, val: %q", batteryConfigTag, originalBcicData)

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	fs := dutfs.NewClient(cl.Conn)

	tempDir, err := fs.TempDir(ctx, "", "ECBcicTAST_*")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}
	s.Logf("Created temp directory: %q", tempDir)
	modifiedBcfgDataFile = filepath.Join(tempDir, modifiedBcfgDataFile)

	defer func() {
		s.Log("Cleaning up wp status")
		if err := h.SetECWriteProtect(ctx, false); err != nil {
			s.Fatal("Failed to disable firmware write protect: ", err)
		}

		// Connect to the gRPC server on the DUT.
		cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(ctx)

		fs := dutfs.NewClient(cl.Conn)
		if fs != nil {
			tempDirExists, err := fs.Exists(ctx, tempDir)
			if err != nil {
				s.Fatal("Failed to check existence of temp directory: ", tempDir, err)
			}

			if tempDirExists {
				if err := fs.RemoveAll(ctx, tempDir); err != nil {
					s.Fatal("Failed to remove temp directory: ", tempDir, err)
				}
			}
		} else {
			s.Log("DUTFS connection not available. Skip removing the temp directory")
		}

		if modifiedTag {
			s.Logf("Restoring tag %q", batteryConfigTag)
			if err := writeBatteryConfigToCbi(ctx, h, batteryConfigTag, originalBcicData); err != nil {
				s.Fatal("Expected restoring data to succeed: ", err)
			}
		}
	}()

	originalBcfgData, err := getBcfg(ctx, h, "")
	if err != nil {
		s.Fatal("Expected BCFG read to succeed: ", err)
	}
	originalBcfgBytes := []byte(originalBcfgData)
	bcfg := &bcfgData{
		bytes: originalBcfgBytes,
	}

	battManufName, battDeviceName, err := getManufacturerAndDeviceName(ctx, h)
	if err != nil {
		s.Fatal("Battery info not found: ", err)
	}
	s.Logf("Battery manufacturer name: %q, Battery device name: %q", battManufName, battDeviceName)

	if err := updateBcfgBytes(ctx, h, bcfg, battManufName, battDeviceName); err != nil {
		s.Fatal("Data modification failed: ", err)
	}

	if err := fs.WriteFile(ctx, modifiedBcfgDataFile, bcfg.bytes, 0666); err != nil {
		s.Fatal("Expected file write to succeed: ", err)
	}

	if err = setBcfg(ctx, h, modifiedBcfgDataFile, bcfg); err != nil {
		s.Fatal("Expected BCFG write to succeed: ", err)
	} else {
		modifiedTag = true
	}

	if err := coldResetEC(ctx, h); err != nil {
		s.Fatal("Expected EC cold reset to succeed: ", err)
	}

	// Connect to the gRPC server on the DUT.
	cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	_ = dutfs.NewClient(cl.Conn)

	updatedBcfgData, err := getBcfg(ctx, h, "0")
	if err != nil {
		s.Fatal("Expected BCFG read to succeed: ", err)
	}
	updatedBcfgBytes := []byte(updatedBcfgData)

	jsonVal, err := jsonBytesToMap(ctx, originalBcfgBytes)
	if err != nil {
		s.Fatal("Expected bytes to map conversion to succeed: ", err)
	}
	originalBcfgBytes, err = jsonMapToBytes(ctx, jsonVal)
	if err != nil {
		s.Fatal("Expected map to bytes conversion to succeed: ", err)
	}

	jsonVal, err = jsonBytesToMap(ctx, updatedBcfgBytes)
	if err != nil {
		s.Fatal("Expected bytes to map conversion to succeed: ", err)
	}
	updatedBcfgBytes, err = jsonMapToBytes(ctx, jsonVal)
	if err != nil {
		s.Fatal("Expected map to bytes conversion to succeed: ", err)
	}

	if bytes.Equal(originalBcfgBytes, updatedBcfgBytes) {
		s.Fatal("Expected BCFG output not remain same: ", string(originalBcfgBytes), string(updatedBcfgBytes))
	}

	if !bytes.Equal(bcfg.bytes, updatedBcfgBytes) {
		s.Fatal("Expected BCFG output match: ", string(bcfg.bytes), string(updatedBcfgBytes))
	}
}

func getManufacturerAndDeviceName(ctx context.Context, h *firmware.Helper) (string, string, error) {
	testing.ContextLog(ctx, "Attempting to read Battery info")
	out, err := h.Servo.RunECCommandGetOutput(ctx, "battery", []string{`Manuf:\s+([^\n\r]+)[\n\r]`, `Device:\s+([^\n\r]+)[\n\r]`})
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get Battery info")
	}
	manufName, deviceName := out[0][1], out[1][1]
	return manufName, deviceName, nil
}

// updateBcfgBytes modifies the input bytes in the bcfg data based on the battery manufacturer and
// device name from the EC console.
func updateBcfgBytes(ctx context.Context, h *firmware.Helper, bcfg *bcfgData, battManufName, battDeviceName string) error {
	key0 := strings.Join([]string{battManufName, battDeviceName}, ",")
	key1 := "batt_info"
	key2 := "start_charging_max_c"
	var val float64

	bcfgMap, err := jsonBytesToMap(ctx, bcfg.bytes)
	if err != nil {
		return errors.Wrap(err, "failed to convert bytes to map")
	}

	var keys = []string{}
	for key, val0 := range bcfgMap {
		testing.ContextLogf(ctx, "Found key %q in bcfg map", key)
		keys = append(keys, key)
		keySplit := strings.Split(key, ",")
		if len(keySplit) != 2 || keySplit[0] == "" || keySplit[1] == "" {
			testing.ContextLogf(ctx, "Key %q does not have 2 parts", key)
			continue
		}

		if strings.HasPrefix(strings.ToUpper(battManufName), strings.ToUpper(keySplit[0])) &&
			strings.HasPrefix(strings.ToUpper(battDeviceName), strings.ToUpper(keySplit[1])) {
			bcfg.battManufName = keySplit[0]
			bcfg.battDeviceName = keySplit[1]
			val1, exists := val0.(map[string]interface{})[key1]
			if !exists {
				return errors.Wrapf(err, "key not found: %q in map: %q", key1, val0)
			}
			val2, exists := val1.(map[string]interface{})[key2]
			if !exists {
				return errors.Wrapf(err, "key not found: %q in map: %q", key2, val1)
			}
			val = val2.(float64)
			if val == 100 {
				val--
			} else {
				val++
			}
			val1.(map[string]interface{})[key2] = val
			if bcfg.bytes, err = jsonMapToBytes(ctx, bcfgMap); err != nil {
				return errors.Wrap(err, "failed to convert map to bytes")
			}
			return nil
		}
		testing.ContextLogf(ctx, "Skipping key because manufacturer or device name mismatches battery info: %s,%s", bcfg.battManufName, bcfg.battDeviceName)
	}
	return errors.Wrapf(err, "key not found: %s in the keys of bcfg map: %q", key0, keys)
}

func getBcfg(ctx context.Context, h *firmware.Helper, index string) (string, error) {
	testing.ContextLog(ctx, "Attempting to read Battery config")
	args := []string{"-j"}
	if index != "" {
		args = append(args, index)
	}
	out, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).BCFG(ctx, firmware.BCFGGet, args...)
	if err != nil {
		return "", errors.Wrap(err, "failed to get Battery config")
	}
	return out, nil
}

func setBcfg(ctx context.Context, h *firmware.Helper, fileName string, bcfg *bcfgData) error {
	testing.ContextLog(ctx, "Attempting to write Battery config")
	args := []string{fileName, bcfg.battManufName, bcfg.battDeviceName}
	_, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).BCFG(ctx, firmware.BCFGSet, args...)
	if err != nil {
		return errors.Wrap(err, "failed to set Battery config")
	}
	return nil
}

func coldResetEC(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Rebooting the DUT with cold reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT with cold reset")
	}

	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to reconnect")
	}

	return nil
}

func writeBatteryConfigToCbi(ctx context.Context, h *firmware.Helper, tag, data string) error {
	testing.ContextLogf(ctx, "Attempting to write data %q to tag %q", data, tag)
	writeArgs := []string{tag, data, "0"}
	out, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).CBI(ctx, firmware.CBISet, writeArgs...)
	if err != nil {
		return errors.Wrapf(err, "failed to write data %q to cbi tag %q, got output: %v", data, tag, out)
	}
	return nil
}

func readBatteryConfigFromCbi(ctx context.Context, h *firmware.Helper, tag string) (string, error) {
	testing.ContextLog(ctx, "Attempting to read data from tag ", tag)
	out, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).CBI(ctx, firmware.CBIGet, tag)
	if err != nil {
		if strings.Contains(err.Error(), "no value") {
			testing.ContextLogf(ctx, "No value for tag %s from cbi", tag)
			return "", nil
		}
		return "", errors.Wrapf(err, "failed to read tag %q from cbi", tag)
	}
	return out, nil
}

func jsonBytesToMap(ctx context.Context, bytes []byte) (map[string]interface{}, error) {
	var jsonMap map[string]interface{}
	err := json.Unmarshal(bytes, &jsonMap)
	if err != nil {
		return nil, errors.Wrapf(err, "JSON bytes to map conversion failed: %q", bytes)
	}
	return jsonMap, err
}

func jsonMapToBytes(ctx context.Context, jsonMap map[string]interface{}) ([]byte, error) {
	bytes, err := json.MarshalIndent(jsonMap, "", "\t")
	if err != nil {
		return nil, errors.Wrapf(err, "JSON map to bytes conversion failed: %q", jsonMap)
	}
	return bytes, nil
}
