// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	parseDeviceIDLineRE     = regexp.MustCompile(`(.*)=(.*)`)
	deviceIDFields          = [8]string{"brand", "device", "product", "manufacturer", "model", "sn", "imei", "meid"}
	deviceIDStorageCommands = [2]string{"delete", "commit"}
	optionalDeviceIDFields  = []string{"imei", "meid"}
	noDeviceIDFields        = map[string]string{}
	// unsetVal is the string gsctool prints to signal the value is unset and not just an empty string
	unsetVal = "__unset__"
	// unsetSize is the unininitialzed size value (max u8)
	unsetSize = "255"
	// Before fields are set the RMA value should be NA
	unsetRMA = "NA"
	// Device IDs were not set using RMA. They should report RMA=N
	setRMA             = "N"
	deviceIDStrMax     = strings.Repeat("A", 32)
	deviceIDStrTooLong = deviceIDStrMax + "B"
)

const (
	deviceIDStrMedium = "Device ID Val"
)

type testSetDeviceIDCmd struct {
	cmdType      string
	fieldVal     string
	skipOptional bool
	ok           bool
}

type configTestDeviceIDs struct {
	bus     ti50.TpmBus
	setCmds []testSetDeviceIDCmd
}

var infoHeaderErased = map[string]string{
	"version":           "1",
	"valid":             "N",
	"finalized":         "N",
	"storage_type":      "255",
	"info_storage":      "N",
	"scratch_storage":   "N",
	"field_count":       "255",
	"total_fields_size": "65535",
	"status":            "1",
}

var infoHeaderSet = map[string]string{
	"version":           "1",
	"valid":             "Y",
	"finalized":         "Y",
	"storage_type":      "2",
	"info_storage":      "Y",
	"scratch_storage":   "N",
	"field_count":       "8",
	"total_fields_size": "264",
	"status":            "0",
}

var scratchHeaderCommitFailed = map[string]string{
	"version":           "1",
	"valid":             "N",
	"finalized":         "N",
	"storage_type":      "1",
	"info_storage":      "N",
	"scratch_storage":   "Y",
	"field_count":       "8",
	"total_fields_size": "264",
	"status":            "3",
}

var scratchHeaderUnset = map[string]string{
	"version":           "1",
	"valid":             "N",
	"finalized":         "N",
	"storage_type":      "1",
	"info_storage":      "N",
	"scratch_storage":   "Y",
	"field_count":       "255",
	"total_fields_size": "65535",
	"status":            "7",
}

var scratchHeaderSet = map[string]string{
	"version":           "1",
	"valid":             "Y",
	"finalized":         "N",
	"storage_type":      "1",
	"info_storage":      "N",
	"scratch_storage":   "Y",
	"field_count":       "8",
	"total_fields_size": "264",
	"status":            "0",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCDeviceIDs,
		Desc:    "Verify GSC handles getting and setting the Device IDs",
		Timeout: 7 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"mruthven@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_shield",
			"gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
		Params: []testing.Param{{
			// Verify GSC doesn't save anything in the info space
			// when there's been no commit
			Name: "no_commit",
			Val: configTestDeviceIDs{
				bus: ti50.TpmBusI2c,
				setCmds: []testSetDeviceIDCmd{
					{cmdType: "noCmd"}, // Verify IDs start out erased
					{cmdType: "setAll", fieldVal: deviceIDStrMedium, ok: true},
					{cmdType: "factoryDisable", ok: false},
					// Verify all set commands are blocked after factory disable
					{cmdType: "setAll", fieldVal: deviceIDStrMax, ok: false},
					{cmdType: "commit", ok: false},
					{cmdType: "delete", ok: false},
				},
			},
		}, {
			// Verify GSC accepts empty strs for ID values.
			Name: "empty_str_i2c",
			Val: configTestDeviceIDs{
				bus: ti50.TpmBusI2c,
				setCmds: []testSetDeviceIDCmd{
					// Set all IDs to empty strings. Skip IMEI and MIED. Verify GSC can still commit the IDs
					{cmdType: "setAll", skipOptional: true, fieldVal: "", ok: true},
					{cmdType: "commit", ok: true},
					{cmdType: "factoryDisable", ok: true},
				},
			},
		}, {
			Name: "empty_str_spi",
			Val: configTestDeviceIDs{
				bus: ti50.TpmBusSpi,
				setCmds: []testSetDeviceIDCmd{
					// Set all IDs to empty strings. Skip IMEI and MIED. Verify GSC can still commit the IDs
					{cmdType: "setAll", skipOptional: true, fieldVal: "", ok: true},
					{cmdType: "commit", ok: true},
					{cmdType: "factoryDisable", ok: true},
				},
			},
		}, {
			Name: "update_ids",
			Val: configTestDeviceIDs{
				bus: ti50.TpmBusI2c,
				setCmds: []testSetDeviceIDCmd{
					{cmdType: "noCmd"}, // Verify IDs start out erased
					// Verify commit fails when no IDs have been set.
					{cmdType: "commit", ok: false},
					{cmdType: "setAll", fieldVal: deviceIDStrMax, ok: true},
					// Verify IDs get deleted with the delete command
					{cmdType: "delete", ok: true},
					// Commit fails, because no IDs are set
					{cmdType: "commit", ok: false},
					{cmdType: "setAll", fieldVal: deviceIDStrMedium, ok: true},
					// Verify IDs can be updated before commit
					{cmdType: "setAll", skipOptional: true, fieldVal: deviceIDStrMax, ok: true},
					// Verify scratch IDs survive GSC reset
					{cmdType: "reset"},
					// Verify scratch IDs survive ccd open
					{cmdType: "ccd open"},
					// Verify GSC rejects strings that are too long.
					{cmdType: "setAll", fieldVal: deviceIDStrTooLong, ok: false},
					{cmdType: "commit", ok: true},
					// Verify IDs can't be updated after commit
					{cmdType: "setAll", fieldVal: "", ok: false},
					{cmdType: "factoryDisable", ok: true},
				},
			},
		}},
	})
}

// GSCDeviceIDs verifies getting and setting the device IDs.
func GSCDeviceIDs(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	config := s.Param().(configTestDeviceIDs)
	bus := config.bus
	emptyFields := make(map[string]string)
	setFields := make(map[string]string)
	for _, name := range deviceIDFields {
		emptyFields[name] = unsetVal
		emptyFields[name+"_size"] = unsetSize
		emptyFields[name+"_rma"] = unsetRMA
		setFields[name] = unsetVal
		setFields[name+"_size"] = unsetSize
		setFields[name+"_rma"] = unsetRMA
	}

	tpm := b.ResetAndTpmStartupForBus(ctx, i, bus, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	expectedInfoFields := emptyFields
	expectedInfoHeader := infoHeaderErased
	expectedScratchFields := emptyFields
	expectedScratchHeader := scratchHeaderUnset
	writeLocked := false

	for j, setCmd := range config.setCmds {
		desc := fmt.Sprintf("%d %s", j, setCmd.cmdType)
		if setCmd.ok {
			desc += "-ok"
		}
		switch setCmd.cmdType {
		case "noCmd":
			s.Log("No set cmd")
		case "setAll":
			for _, name := range deviceIDFields {
				if setCmd.skipOptional && slices.Contains(optionalDeviceIDFields, name) {
					s.Log("Skip optional field ", name)
					continue
				}
				arg := fmt.Sprintf("%s:%s", name, setCmd.fieldVal)
				setOut, err := setDeviceIDs(ctx, b, config.bus, arg)
				s.Logf("Result: %s", setOut)
				if setCmd.ok != (err == nil) {
					s.Fatalf("%s: failed to run %s %s", desc, arg, err)
				}
				if err == nil {
					s.Logf("set: %q", arg)
					setFields[name] = setCmd.fieldVal
					setFields[name+"_size"] = fmt.Sprintf("%d", len(setCmd.fieldVal))
					setFields[name+"_rma"] = setRMA
				} else {
					s.Logf("%s: failed to set: %q", desc, arg)
				}
			}
			if setCmd.ok {
				expectedScratchHeader = scratchHeaderSet
				expectedScratchFields = setFields
			}
		case "ccd open":
			if err := i.WipeTpmWithCCDOpen(ctx); err != nil {
				s.Fatalf("%s: failed ccd open %s", desc, err)
			}
			th.MustSucceed(i.WaitUntilBooted(ctx), "failed to wait for gsc")
			tpm = b.ResetAndTpmStartupForBus(ctx, i, bus, ti50.FfClamshell)
			th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
		case "commit":
			_, err := setDeviceIDs(ctx, b, config.bus, "commit")
			if setCmd.ok != (err == nil) {
				s.Fatalf("%s: unexpected result got %s", desc, err)
			}
			if setCmd.ok {
				expectedScratchHeader = infoHeaderSet
				expectedInfoHeader = infoHeaderSet
				expectedInfoFields = setFields
			} else if !writeLocked {
				expectedScratchHeader = scratchHeaderCommitFailed
			}
		case "delete":
			_, err := setDeviceIDs(ctx, b, config.bus, "delete_scratch")
			if setCmd.ok != (err == nil) {
				s.Fatalf("%s: unexpected result got %s", desc, err)
			}
			if setCmd.ok {
				expectedScratchHeader = scratchHeaderUnset
				expectedScratchFields = emptyFields
				for _, name := range deviceIDFields {
					setFields[name] = unsetVal
					setFields[name+"_size"] = unsetSize
					setFields[name+"_rma"] = unsetRMA
				}
			}
		case "factoryDisable":
			// Factory disable is not blocked by setting the Device IDs
			err := tpm.TpmvFactoryModeDisable()
			th.MustSucceed(err, desc+" failed")

			if setCmd.ok {
				expectedInfoHeader = infoHeaderSet
				expectedScratchHeader = infoHeaderSet
			} else {
				expectedScratchHeader = infoHeaderErased
				expectedInfoHeader = infoHeaderErased
			}
			expectedScratchFields = expectedInfoFields
			writeLocked = true
		case "reset":
			// Reset the GSC. Verify scratch IDs don't change
			tpm = b.ResetAndTpmStartupForBus(ctx, i, bus, ti50.FfClamshell)
			th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
		default:
			s.Fatalf("Unsupported cmd: %+v", setCmd.cmdType)
		}

		out, err := getDeviceIDs(ctx, b, bus, "get_scratch")
		th.MustSucceed(err, desc+": failed go get scratch")
		s.Logf("%s result: %s", desc, out)
		validateOutput(s, desc+" scratch", out, expectedScratchHeader, expectedScratchFields)

		out, err = getDeviceIDs(ctx, b, bus, "get_info")
		th.MustSucceed(err, desc+": failed to get info")
		s.Logf("%s result: %s", desc, out)
		validateOutput(s, desc+" info", out, expectedInfoHeader, expectedInfoFields)
	}
}

func validateOutput(s *testing.State, desc string, out, expectedHeader, expectedFields map[string]string) {
	for k, v := range expectedHeader {
		outV, ok := out[k]
		if !ok {
			s.Errorf("%s: %s not found in out", desc, k)
		} else if v != outV {
			s.Errorf("%s %s: expected %s got %s", desc, k, v, outV)
		}
	}
	for _, name := range deviceIDFields {
		expectedVal, expectedOk := expectedFields[name]
		// gsctool output adds "FIELD_" to the field ID names.
		actVal, actOk := out["field_"+name]
		act := fmt.Sprintf("ok(%t)", actOk)
		if actOk {
			act += fmt.Sprintf(" val(%s)", actVal)
		}
		expected := fmt.Sprintf("ok(%t)", expectedOk)
		if expectedOk {
			expected += fmt.Sprintf(" val(%s)", expectedVal)
		}
		if actOk != expectedOk || actVal != expectedVal {
			s.Errorf("%s %s: expected %s got %s", desc, name, expected, act)
		}
	}
}

// parseOutput parses the get device IDs output
func parseOutput(out string) (map[string]string, error) {
	result := make(map[string]string)
	matches := parseDeviceIDLineRE.FindAllStringSubmatch(out, -1)
	if matches == nil {
		return nil, errors.New("failed to parse ccd output")
	}
	for j := 0; j < len(matches); j++ {
		// Standardize the field names between the machine and regular
		// output.
		key := strings.ToLower(matches[j][1])
		key = strings.TrimSpace(key)
		result[key] = strings.TrimSpace(matches[j][2])
	}
	return result, nil
}

// getDeviceIDs runs the get device id command and parses the output
func getDeviceIDs(ctx context.Context, b utils.DevboardHelper, bus ti50.TpmBus, idType string) (map[string]string, error) {
	out, err := b.GSCToolCommandViaTPM(ctx, bus, "", "-M", "--device_ids", idType)
	if err != nil {
		return nil, err
	}
	testing.ContextLog(ctx, "output:", string(out))
	return parseOutput(string(out))
}

// setDeviceIDs runs the set device id command
func setDeviceIDs(ctx context.Context, b utils.DevboardHelper, bus ti50.TpmBus, setCmd string) (string, error) {
	out, err := b.GSCToolCommandViaTPM(ctx, bus, "", "--device_ids", setCmd)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
