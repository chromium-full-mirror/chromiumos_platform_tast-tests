// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hardware

import (
	"context"
	"math"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/hardware/iio"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SensorAccel,
		BugComponent: "b:811602",
		Desc:         "Tests that accelerometer sensors can be read and give valid data",
		Contacts: []string{
			"chromeos-sensors-eng@google.com",
			"gwendal@chromium.org",
		},
		Attr: []string{"group:mainline", "group:cq-medium"},
	})
}

// SensorAccel gets the current sensor reading of all accelerometer sensors and
// verifies that the data is within 25% of 1g.
func SensorAccel(ctx context.Context, s *testing.State) {
	const (
		accel1g  = 9.8185
		accelErr = accel1g * .25
	)

	sensors, err := iio.GetSensors(ctx)
	if err != nil {
		s.Fatal("Error reading sensors on DUT: ", err)
	}

	for _, sn := range sensors {
		if sn.Name != iio.Accel {
			continue
		}

		r, err := sn.Read()
		if err != nil {
			s.Errorf("Error reading data from %v %v: %v", sn.Location, sn.Name, err)
			continue
		}

		if len(r.Data) != 3 {
			s.Errorf("Got %v from %v %v; want 3 values", r.Data, sn.Location, sn.Name)
			continue
		}

		mag := math.Sqrt(r.Data[0]*r.Data[0] + r.Data[1]*r.Data[1] + r.Data[2]*r.Data[2])
		s.Logf("%v %v magnitude is %.3f", sn.Location, sn.Name, mag)

		if math.Abs(mag-accel1g) > accelErr {
			s.Errorf("%v %v data out of range: got %.3f; want %.3f +- %.3f",
				sn.Location, sn.Name, mag, accel1g, accelErr)
		}
	}
}
