// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ServoEcho,
		Desc:         "Demonstrates running a test using Servo",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "informational"},
		VarDeps:      []string{"servo"},
	})
}

// ServoEcho demonstrates how you'd use Servo in a Tast test using the echo method.
// To run it successfully with pass status, servo must be specified and servod must
// be run on the servo host.
// Example of running servod on a labstation:
// % start servod PORT=9996
// servod (9996) start/running, process 19567
// Example of running this test without port forwarding:
// % tast run -var=servo=<servo_host>:<servo_port> <dut> example.ServoEcho
// Example of running this test with port forwarding:
// % tast run -var=servo=localhost:9996:ssh:2227 localhost:2226 example.ServoEcho
func ServoEcho(ctx context.Context, s *testing.State) {
	dut := s.DUT()

	// If you want the test to be skipped when servo is not available, run it with
	// maybemissingvars.
	// Example:
	// tast run -maybemissingvars=servo <dut> example.ServoEcho
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctx)

	const msg = "hello from servo"
	s.Logf("Sending echo request for %q", msg)
	actualMessage, err := pxy.Servo().Echo(ctx, msg)
	if err != nil {
		s.Fatal("Got error: ", err)
	}
	s.Logf("Got response %q", actualMessage)
	const expectedMessage = "ECH0ING: " + msg
	if actualMessage != expectedMessage {
		s.Fatalf("Got message %q; expected %q", actualMessage, expectedMessage)
	}
}
