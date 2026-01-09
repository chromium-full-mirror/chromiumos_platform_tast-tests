// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package biod

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/fingerprint"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CheckHWID,
		Desc: "Checks that fingerprint board in cros-config agrees with mcu and sensor",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"josienordrum@google.com",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		// This test is fingerprint-agnostic so it can run in general mainline suites.
		Attr: []string{"group:mainline", "group:fingerprint-cq", "group:cq-medium", "group:fingerprint-release"},
		// Note that hwdep for fingerprint relies on cros-config.
		// We omit any dependencies on hwdep fingerprint so that we can detect
		// issues where cros-config fails to mention fingerprint support.
	})
}

// CheckHWID checks that cros-config fingerprint board matches the sensor and mcu found by probing in
// https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/platform/factory/py/probe/functions/generic_fingerprint.py, which is used to construct the HWID.
// This test should also match Paris Recovery's cros_fingerprint_exec.go "Collect fingerprint" action.
func CheckHWID(ctx context.Context, s *testing.State) {
	board, err := crosconfig.Get(ctx, "/fingerprint", "board")
	if err != nil || board == "" {
		// In the error case, assume device does not have fingerprint.
		board = "None"
	}

	// command to grab the fingerprint_mcu from dut.
	mcuCmdOut, _ := testexec.CommandContext(ctx, "ectool", "--name=cros_fp", "chipinfo").Output()
	mcu := ""
	if len(mcuCmdOut) == 0 {
		mcu = "None"
	} else {
		chipInfoMap := fingerprint.ParseColonDelimitedOutput(string(mcuCmdOut))
		mcu = chipInfoMap["name"]
		// Handle mcu running zephyr which may have more specific name.
		if strings.HasPrefix(mcu, "stm32f412") {
			mcu = "stm32f412"
		} else if strings.HasPrefix(mcu, "stm32h7") {
			mcu = "stm32h7x3"
		} else if strings.HasPrefix(mcu, "npcx9mfp") {
			mcu = "NPCX99FP"
		}
	}

	// grab the fingerprint_sensor info from dut.
	const fpcVendorID = `20435046`
	sensor := ""
	sensorCmdOut, _ := testexec.CommandContext(ctx, "ectool", "--name=cros_fp", "fpinfo").Output()

	if len(sensorCmdOut) == 0 {
		sensor = "None"
	} else {
		fpInfoMap, err := fingerprint.ParseFpInfo(string(sensorCmdOut))
		if err != nil {
			s.Fatalf("Failed to parse Fingerprint sensor map: %s fails with error %v", fpInfoMap.FingerprintSensor, err)
		}
		sensorVendor := fpInfoMap.FingerprintSensor["vendor"]
		sensor = fpInfoMap.FingerprintSensor["model"]
		if sensorVendor == fpcVendorID {
			intsensorMasked, err := strconv.ParseInt(sensor, 16, 64)
			if err != nil {
				s.Fatalf("Probed FPC vendor, but model is not hex string: %s: ", sensor)
			} else {
				intsensorMasked &= ^0xf // Mask off the last four bits
				sensor = fmt.Sprintf("%x", intsensorMasked)
			}
		}
	}

	if !checkFingerprintInfo(board, mcu, sensor) {
		s.Fatalf("Unexpected fingerprint hw/sw combo. Board: %s, Mcu:%s, Sensor: %s", board, mcu, sensor)
	}

}

// checkFingerprintInfo confirms that fingerprint mcu and sensor from probing the fpmcu match the fingerprint board found in cros_config.
func checkFingerprintInfo(board, mcu, sensor string) bool {
	// From go/cros-fingerprint-docs#mcu-sensor-table.
	validCombinations := map[string]map[string]string{
		"bloonchipper": {"stm32f412": "210"},
		"dartmonkey":   {"stm32h7x3": "1400"},
		"nami_fp":      {"stm32h7x3": "1400"},
		"nocturne_fp":  {"stm32h7x3": "1400"},
		"helipilot":    {"NPCX99FP": "210"},
		"buccaneer":    {"NPCX99FP": "4f4f"},
		"None":         {"None": "None"},
	}

	if mcus, ok := validCombinations[board]; ok {
		if sensors, ok := mcus[mcu]; ok {
			return sensors == sensor
		}
	}

	return false

}
