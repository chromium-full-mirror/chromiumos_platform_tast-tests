// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bytes"
	"context"
	"math"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VMBattery,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that VM Battery exists",
		Contacts:     []string{"arcvm-platform-power@google.com"},
		BugComponent: "b:1477363",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "android_vm"},
		Fixture:      "arcBooted",
		Timeout:      10 * time.Minute,
	})
}

func parseProperty(data []byte) map[string]string {
	m := make(map[string]string)
	for _, l := range bytes.Split(data, []byte("\n")) {
		f := bytes.SplitN(l, []byte(":"), 2)
		if len(f) == 1 {
			m[string(bytes.TrimSpace(f[0]))] = ""
		} else if len(f) == 2 {
			m[string(bytes.TrimSpace(f[0]))] = string(bytes.TrimSpace(f[1]))
		}
	}
	return m
}

func VMBattery(ctx context.Context, s *testing.State) {
	// Get host battery and AC information
	output, err := testexec.CommandContext(ctx, "power_supply_info").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get the power supply information: ", err)
	}
	// Parse the AC and Battery info.
	acInfo := make(map[string]string)
	batInfo := make(map[string]string)
	// `power_supply_info` command shows the properties of each power supply
	// devices, which are formatted as below;
	//
	// Device: DEVICE_NAME1
	//   Property 1: value1
	//   Property 2: value2
	//   ...
	// Device: DEVICE_NAME2
	//   ...
	for _, deviceProperties := range bytes.Split(output, []byte("Device:")) {
		if len(deviceProperties) == 0 {
			continue
		}
		property := bytes.SplitN(deviceProperties, []byte("\n"), 2)
		// The first line must be the DEVICE_NAME
		devname := string(bytes.TrimSpace(property[0]))
		if devname == "Line Power" {
			acInfo = parseProperty(property[1])
		} else if devname == "Battery" {
			batInfo = parseProperty(property[1])
		}
	}

	a := s.FixtValue().(*arc.PreData).ARC
	output, err = a.Command(ctx, "dumpsys", "battery").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get the number of guest processors: ", err)
	}
	// `dumpsys battery` shows a mixed power supply properties as below;
	//
	// Current Battery Service state:
	//   AC powered: true
	//   USB powered: false
	//   ...
	vmInfo := parseProperty(output)

	// Check AC adapter status is the same
	if (acInfo["online"] == "yes" && vmInfo["AC powered"] == "false") ||
		(acInfo["online"] == "no" && vmInfo["AC powered"] == "true") {
		s.Errorf("The AC power status does not match: %s vs %s", acInfo["online"], vmInfo["AC powered"])
	}

	// Chrome Boxes don't have the battery. If so, skip the rest of the tests.
	if _, isThere := batInfo["path"]; !isThere {
		return
	}

	// Check charged percentage is the same.
	// Note: there are "percentage" and "display percentage". The latter one is
	// passed to ARCVM.
	batLevel, serr := strconv.ParseFloat(batInfo["display percentage"], 10)
	if serr != nil {
		s.Fatalf("Failed to convert percentage %s: %q", batInfo["display percentage"], serr)
	}
	vmLevel, serr2 := strconv.Atoi(vmInfo["level"])
	if serr2 != nil {
		s.Fatalf("Failed to convert vm percentage %s: %q", vmInfo["level"], serr2)
	}
	if int(math.Round(batLevel)) != vmLevel {
		s.Fatalf("The charged percentage does not match: %d vs %d", int(math.Round(batLevel)), vmLevel)
	}
}
