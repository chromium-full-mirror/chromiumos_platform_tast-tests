// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"testing"
)

func TestInstance_ReadAP_NoOutputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)
	opts := NewReadAPOptions("")

	_, err := i.ReadAP(context.Background(), opts)
	if want := "cannot read AP firmware: output file not available"; err != nil && err.Error() != want {
		t.Errorf("unexpected error received = got %q, want %q", err, want)
	} else if err == nil {
		t.Error("error expected, got nothing")
	}
}

func TestInstance_ReadAP_Params(t *testing.T) {
	for _, p := range []struct {
		args   []string
		setter func(o *readAPOptions) *readAPOptions
	}{
		{
			args:   []string{"--region", "FW_MAIN_A"},
			setter: func(o *readAPOptions) *readAPOptions { return o.WithRegion("FW_MAIN_A") },
		},
		{
			args:   []string{"--debug"},
			setter: func(o *readAPOptions) *readAPOptions { return o.WithDebug(true) },
		},
		{
			args:   []string{"--verbose"},
			setter: func(o *readAPOptions) *readAPOptions { return o.WithVerbose(true) },
		},
	} {
		i := newTestInstance(nil, nil, nil)
		opts := p.setter(NewReadAPOptions("output.bin"))

		_, err := i.ReadAP(context.Background(), opts)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		ti := i.commandRunner.(*testCommandRunner)
		positionalArgs := []string{testFutilityPath, "read", "output.bin"}
		optionArgs := [][]string{p.args}
		if err = ti.assertCalledWith(positionalArgs, optionArgs, nil); err != nil {
			t.Errorf("unexpected command call: %v", err)
		}
	}
}

func TestInstance_ReadAP_Servo(t *testing.T) {
	i := newTestInstance(nil, nil, nil)
	i.params.servoPort = 1234
	opts := NewReadAPOptions("output.bin")

	_, err := i.ReadAP(context.Background(), opts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	ti := i.commandRunner.(*testCommandRunner)
	positionalArgs := []string{testFutilityPath, "read", "output.bin"}
	optionArgs := [][]string{{"--servo"}, {"--servo_port", "1234"}}
	if err = ti.assertCalledWith(positionalArgs, optionArgs, nil); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}
