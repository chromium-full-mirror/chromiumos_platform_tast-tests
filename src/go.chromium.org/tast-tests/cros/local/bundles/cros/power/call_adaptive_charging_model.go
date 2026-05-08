// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"bytes"
	"context"
	"math"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CallAdaptiveChargingModel,
		Desc: "Check the AdaptiveCharging model",
		Contacts: []string{
			"ml-service-team@google.com",
			"amoylan@chromium.org",
			"thanhdng@chromium.org",
		},
		// Software > Fundamentals > Machine Intelligence > Adaptive Charging
		BugComponent: "b:843222",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"ml_service"},
	})
}

// CallAdaptiveChargingModel give two pokes to the adaptive charging model, one
// with "dbus-send" command, the other with dbusutil. "dbus-send" is able to
// invoke the necessary ml-service instance but its output is in stdout and
// requires parsing. While dbusutil requires the existence of ml-service
// instance but it returns structured values, making the return value
// verification easier.
func CallAdaptiveChargingModel(ctx context.Context, s *testing.State) {
	const (
		job                      = "ml-service"
		instanceParameter        = "TASK"
		instance                 = "adaptive_charging"
		dbusName                 = "org.chromium.MachineLearning.AdaptiveCharging"
		dbusPath                 = "/org/chromium/MachineLearning/AdaptiveCharging"
		dbusInterfaceMethod      = dbusName + ".RequestAdaptiveChargingDecision"
		timeout                  = 60 * time.Second
		float64EqualityThreshold = 1e-6
	)

	if err := upstart.StopJob(ctx, job, upstart.WithArg(instanceParameter, instance)); err != nil {
		s.Fatalf("Failed to stop %s: %v", job, err)
	}

	if err := upstart.WaitForJobStatus(ctx, job, upstartcommon.StopGoal, upstartcommon.WaitingState, upstart.RejectWrongGoal, 15*time.Second, upstart.WithArg(instanceParameter, instance)); err != nil {
		s.Fatalf("Failed to wait for %v to stop: %v", job, err)
	}

	// Now the ml-service adaptive_charging instance is stopped, make a dbus-send to invoke it and do a model inference.
	cmd := testexec.CommandContext(ctx, "dbus-send", "--system", "--type=method_call", "--print-reply",
		"--dest="+dbusName, dbusPath, dbusInterfaceMethod, "array:uint16:")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to trigger ml-service instance TASK=adaptive_charging via dbus-send: ", err)
	}

	testing.ContextLog(ctx, stdout.String())
	if !strings.Contains(stdout.String(), "boolean true") {
		s.Fatal("Failed because dbus-send call returns inference status false")
	}

	// Now the ml-service instance is running, dbusutil is able to connect to the dbus service.
	_, obj, err := dbusutil.Connect(ctx, dbusName, dbus.ObjectPath(dbusPath))
	if err != nil {
		s.Fatalf("Failed to connect to %s: %v", dbusName, err)
	}

	s.Log("Call " + dbusInterfaceMethod + " to trigger adaptive charging model")
	var emptySerializedExampleProto []uint8
	var status bool
	var result []float64
	if err := obj.CallWithContext(ctx, dbusInterfaceMethod, 0, emptySerializedExampleProto).Store(&status, &result); err != nil {
		s.Fatalf("Failed to call dbus method %s: %v", dbusInterfaceMethod, err)
	}

	if !status {
		s.Fatal("Adaptive charging model inference status is false")
	}

	if len(result) != 9 {
		s.Fatalf("Failed to verify the length of return array, wanted 9, got %d", len(result))
	}

	floatEqual := func(v1, v2 float64) bool {
		return math.Abs(v1-v2) <= float64EqualityThreshold
	}

	// The expectedResult is in sync with ml_service unit test
	// "TfModelGraphExecutorTest.ExecuteAdaptiveCharging20230314", see
	// src/platform2/ml/dbus_service/tf_model_graph_executor_test.cc
	expectedResult := [9]float64{0.207927, 0.187088, 0.121717, 0.090703, 0.065328, 0.045972, 0.026243, 0.018707, 0.236313}
	for i, value := range result {
		if !floatEqual(value, expectedResult[i]) {
			s.Fatalf("Failed to verify return array[%d], wanted %f, got %f", i, expectedResult[i], value)
		}
	}

}
