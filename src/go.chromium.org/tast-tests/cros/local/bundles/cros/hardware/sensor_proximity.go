// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hardware

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hardware/iioservice"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SensorProximity,
		BugComponent: "b:811602",
		Desc:         "Tests a proximity sensor is present in the system",
		Contacts: []string{
			"chromeos-sensors-eng@google.com",
			"chenghaoyang@chromium.org", // Test author
			"gwendal@chromium.com",
		},
		Attr:         []string{"group:sensors"},
		SoftwareDeps: []string{"iioservice"},
		HardwareDeps: hwdep.D(hwdep.Model("pujjo")),
	})
}

// SensorProximity reads one event from a proximity sensor owned by iioservice.
func SensorProximity(ctx context.Context, s *testing.State) {
	out, err := testexec.CommandContext(ctx, "iioservice_event", "--device_type=12",
		"--events=1", "--indices=0").CombinedOutput()

	if err != nil {
		s.Error("Error reading events on DUT: ", err)
	}

	strOut := string(out)
	if strings.Contains(strOut, iioservice.OnErrorOccurredText) {
		s.Error("OnErrorOccurred: ", strOut)
	} else if !strings.Contains(strOut, iioservice.SucceedReadingText) {
		s.Error("Not enough successful read events on sensor: ", strOut)
	} else {
		s.Log("Test passed on proximity")
	}
}
