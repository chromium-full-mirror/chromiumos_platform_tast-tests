// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hardware

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hardware/iio"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hardware/iioservice"
	"go.chromium.org/tast/core/testing"
)

const latencyExceedsTolerance = "Max latency exceeds latency tolerance."

func init() {
	testing.AddTest(&testing.Test{
		Func:         SensorIioservice,
		BugComponent: "b:811602",
		Desc:         "Tests that iioservice provides sensors' samples properly",
		Contacts: []string{
			"chromeos-sensors-eng@google.com",
			"gwendal@chromium.com",
			"chenghaoyang@chromium.org", // Test author
		},
		Attr:         []string{"group:sensors"},
		SoftwareDeps: []string{"iioservice"},
	})
}

// SensorIioservice reads all devices' samples from daemon iioservice.
func SensorIioservice(ctx context.Context, s *testing.State) {
	var maxFreq int
	var strOut string

	// Call libmems' functions directly here to read and verify samples
	sensors, err := iio.GetSensors(ctx)
	if err != nil {
		s.Fatal("Error reading sensors on DUT: ", err)
	}

	for _, sn := range sensors {
		maxFreq = sn.MaxFrequency

		if sn.Name == iio.Ring {
			s.Error("Kernel must be compiled with USE=iioservice")
		}

		if sn.Name == iio.Activity || sn.Name == iio.Light {
			continue
		}

		frequency := fmt.Sprintf("--frequency=%f", float64(maxFreq)/1000)

		out, err := testexec.CommandContext(ctx, "iioservice_simpleclient",
			fmt.Sprintf("--device_id=%d", sn.IioID), "--channels=timestamp",
			frequency).CombinedOutput()

		if err != nil {
			s.Error("Error reading samples on DUT: ", err)
		}

		strOut = string(out)
		if strings.Contains(strOut, iioservice.OnErrorOccurredText) {
			s.Error("OnErrorOccurred: ", sn.Name)
		} else if strings.Contains(strOut, latencyExceedsTolerance) {
			s.Error("Latency Exceeds Tolerance: ", sn.Name)
		} else if !strings.Contains(strOut, iioservice.SucceedReadingText) {
			s.Error("Not enough successful readsamples on sensor: ", sn.Name)
		} else {
			s.Logf("Test passed on device name: %v, id: %v", sn.Name, sn.IioID)
		}
	}
}
