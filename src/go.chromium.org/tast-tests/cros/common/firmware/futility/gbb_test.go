// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

var basicCorrectOutput = []byte("flags: 0x4039\n" +
	"hardware_id: VOXEL TEST 1234\n" +
	"digest: eb65bdb13cc60671b5be89026c4645395d13bd3b3211410981b43a5688b6aebd valid\n")

const basicCorrectOutputFlags = 0x4039
const basicCorrectOutputHWID = "VOXEL TEST 1234"
const basicCorrectOutputDigest = "eb65bdb13cc60671b5be89026c4645395d13bd3b3211410981b43a5688b6aebd"

var gbbPositionalArgs = []string{testFutilityPath, "gbb"}
var getGBBOptionArgs = [][]string{{"--get"}, {"--hwid"}, {"--flags"}, {"--digest"}}

func Test_GetGBB_BasicValues(t *testing.T) {
	opts := &GetGBBOptions{}

	i := newTestInstance(basicCorrectOutput, nil, nil)

	expected := &GBB{
		HWID:   basicCorrectOutputHWID,
		Flags:  basicCorrectOutputFlags,
		Digest: basicCorrectOutputDigest,
	}

	gbb, out, err := i.GetGBB(context.Background(), opts)
	if !reflect.DeepEqual(gbb, expected) {
		t.Errorf("Invalid GBB returned, expected %v, got %v", expected, gbb)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		t.Error("expected command output, got nothing")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	ti := i.commandRunner.(*testCommandRunner)
	optionArgs := append(getGBBOptionArgs, []string{"--flash"})
	if err = ti.assertCalledWith(gbbPositionalArgs, optionArgs, nil); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}

func Test_GetGBB_InvalidBasicValues(t *testing.T) {
	invalidFlags := []byte("flags: invalid\n" +
		"hardware_id: VOXEL TEST 1234\n" +
		"digest: eb65bdb13cc60671b5be89026c4645395d13bd3b3211410981b43a5688b6aebd valid\n")
	invalidDigest := []byte("flags: 0x1234\n" +
		"hardware_id: VODEL TEST 1234\n" +
		"digest: invalid invalid\n")
	opts := &GetGBBOptions{}

	for _, input := range [][]byte{invalidFlags, invalidDigest} {
		i := newTestInstance(input, nil, nil)

		gbb, out, err := i.GetGBB(context.Background(), opts)
		if gbb != nil {
			t.Errorf("expected nil, got GBB: %+v", gbb)
		}
		if len(strings.TrimSpace(string(out))) == 0 {
			t.Error("expected command output, got nothing")
		}
		if err == nil {
			t.Error("error expected, got nothing")
		}
		ti := i.commandRunner.(*testCommandRunner)
		optionArgs := append(getGBBOptionArgs, []string{"--flash"})
		if err = ti.assertCalledWith(gbbPositionalArgs, optionArgs, nil); err != nil {
			t.Errorf("unexpected command call: %v", err)
		}
	}
}

func Test_GetGBB_Servo(t *testing.T) {
	opts := &GetGBBOptions{}

	i := newTestInstance(basicCorrectOutput, nil, nil)
	i.params.servoPort = 1234

	expected := &GBB{
		HWID:   basicCorrectOutputHWID,
		Flags:  basicCorrectOutputFlags,
		Digest: basicCorrectOutputDigest,
	}

	gbb, out, err := i.GetGBB(context.Background(), opts)
	if !reflect.DeepEqual(gbb, expected) {
		t.Errorf("invalid GBB returned, expected %v, got %v", expected, gbb)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		t.Error("expected command output, got nothing")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	ti := i.commandRunner.(*testCommandRunner)
	optionArgs := append(getGBBOptionArgs, []string{"--servo_port", "1234"}, []string{"--flash"})
	extraArgs := [][]string{{"--servo"}}
	if err = ti.assertCalledWith(gbbPositionalArgs, optionArgs, extraArgs); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}

func Test_GetGBB_Flash(t *testing.T) {
	opts := &GetGBBOptions{}

	i := newTestInstance(basicCorrectOutput, nil, nil)
	i.params.servoPort = 0

	expected := &GBB{
		HWID:   basicCorrectOutputHWID,
		Flags:  basicCorrectOutputFlags,
		Digest: basicCorrectOutputDigest,
	}

	gbb, out, err := i.GetGBB(context.Background(), opts)
	if !reflect.DeepEqual(gbb, expected) {
		t.Errorf("invalid GBB returned, expected %v, got %v", expected, gbb)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		t.Error("expected command output, got nothing")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	ti := i.commandRunner.(*testCommandRunner)
	optionArgs := append(getGBBOptionArgs, []string{"--flash"})
	if err = ti.assertCalledWith(gbbPositionalArgs, optionArgs, nil); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}

var setGBBOptionArgs = [][]string{{"--set"}}

func Test_SetGBB_Params(t *testing.T) {
	for _, p := range []struct {
		args   []string
		setter func(o *SetGBBOptions) *SetGBBOptions
	}{
		{
			args:   []string{"--flags", "0x4039"},
			setter: func(o *SetGBBOptions) *SetGBBOptions { return o.WithFlags(0x4039) },
		},
		{
			args:   []string{"--hwid", "EXAMPLE HWID ABCD"},
			setter: func(o *SetGBBOptions) *SetGBBOptions { return o.WithHWID("EXAMPLE HWID ABCD") },
		},
		{
			args:   []string{"--rootkey", "root.key"},
			setter: func(o *SetGBBOptions) *SetGBBOptions { return o.WithRootKey("root.key") },
		},
		{
			args:   []string{"--recoverykey", "recovery.key"},
			setter: func(o *SetGBBOptions) *SetGBBOptions { return o.WithRecoveryKey("recovery.key") },
		},
	} {
		i := newTestInstance(nil, nil, nil)

		opts := p.setter(&SetGBBOptions{})

		_, err := i.SetGBB(context.Background(), opts)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		ti := i.commandRunner.(*testCommandRunner)
		optionArgs := append(setGBBOptionArgs, []string{"--flash"}, p.args)
		if err = ti.assertCalledWith(gbbPositionalArgs, optionArgs, nil); err != nil {
			t.Errorf("unexpected command call: %v", err)
		}
	}
}

func Test_SetGBB_Servo(t *testing.T) {
	opts := (&SetGBBOptions{}).WithFlags(0x1234)

	i := newTestInstance(basicCorrectOutput, nil, nil)
	i.params.servoPort = 1234

	_, err := i.SetGBB(context.Background(), opts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	ti := i.commandRunner.(*testCommandRunner)
	optionArgs := append(setGBBOptionArgs, []string{"--flags", "0x1234"}, []string{"--servo_port", "1234"}, []string{"--flash"})
	extraArgs := [][]string{{"--servo"}}
	if err = ti.assertCalledWith(gbbPositionalArgs, optionArgs, extraArgs); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}

func Test_SetGBB_Flash(t *testing.T) {
	opts := (&SetGBBOptions{}).WithHWID("A B C D")

	i := newTestInstance(basicCorrectOutput, nil, nil)
	i.params.servoPort = 0

	_, err := i.SetGBB(context.Background(), opts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	ti := i.commandRunner.(*testCommandRunner)
	optionArgs := append(setGBBOptionArgs, []string{"--hwid", "A B C D"}, []string{"--flash"})
	if err = ti.assertCalledWith(gbbPositionalArgs, optionArgs, nil); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}

func Test_CreateGBB_InvalidOutputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	out, err := i.CreateGBB(context.Background(), "", 128, 4096, 4096)
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("unexpected command output: %q", string(out))
	}
	if want := "cannot create GBB blob, output file not passed"; err != nil && err.Error() != want {
		t.Errorf("unexpected error received = got %q, want %q", err, want)
	} else if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_CreateGBB_Params(t *testing.T) {
	i := newTestInstance(nil, nil, nil)

	_, err := i.CreateGBB(context.Background(), "output.bin", 128, 4096, 4096)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	ti := i.commandRunner.(*testCommandRunner)
	positionalArgs := append(gbbPositionalArgs, "0x80,0x1000,0,0x1000", "output.bin")
	optionArgs := [][]string{{"--create"}}
	if err = ti.assertCalledWith(positionalArgs, optionArgs, nil); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}
