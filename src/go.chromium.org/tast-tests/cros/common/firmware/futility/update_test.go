// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

var updatePositionalArgs = []string{testFutilityPath, "update"}

func Test_Update_NoInputFile(t *testing.T) {
	i := newTestInstance(nil, nil, nil)
	opts := NewUpdateOptions("")

	_, err := i.Update(context.Background(), opts)
	if want := "no image provided"; err != nil && err.Error() != want {
		t.Errorf("unexpected error received = got %q, want %q", err, want)
	} else if err == nil {
		t.Error("error expected, got nothing")
	}
}

func Test_Update_Opts(t *testing.T) {
	for _, p := range []struct {
		args   []string
		setter func(o *updateOptions) *updateOptions
	}{
		{[]string{}, func(o *updateOptions) *updateOptions { return o.WithECImage("") }},
		{[]string{"--ec_image", "ec.bin"}, func(o *updateOptions) *updateOptions { return o.WithECImage("ec.bin") }},

		{[]string{}, func(o *updateOptions) *updateOptions { return o.WithTryAB(false) }},
		{[]string{"--try"}, func(o *updateOptions) *updateOptions { return o.WithTryAB(true) }},

		{[]string{}, func(o *updateOptions) *updateOptions { return o.WithFast(false) }},
		{[]string{"--fast"}, func(o *updateOptions) *updateOptions { return o.WithFast(true) }},

		{[]string{}, func(o *updateOptions) *updateOptions { return o.WithQuirks(make(map[string]string)) }},
		// WithQuirks with value tested in another test due to map behavior.

		{[]string{}, func(o *updateOptions) *updateOptions { return o.WithMode(UpdateModeDefault) }},
		{[]string{"--mode", string(UpdateModeAutoUpdate)}, func(o *updateOptions) *updateOptions { return o.WithMode(UpdateModeAutoUpdate) }},
		{[]string{"--mode", string(UpdateModeRecovery)}, func(o *updateOptions) *updateOptions { return o.WithMode(UpdateModeRecovery) }},
		{[]string{"--mode", string(UpdateModeFactory)}, func(o *updateOptions) *updateOptions { return o.WithMode(UpdateModeFactory) }},

		{[]string{}, func(o *updateOptions) *updateOptions { return o.WithWriteProtection(WriteProtectionUnchanged) }},
		{[]string{"--wp=1"}, func(o *updateOptions) *updateOptions { return o.WithWriteProtection(WriteProtectionEnable) }},
		{[]string{"--wp=0"}, func(o *updateOptions) *updateOptions { return o.WithWriteProtection(WriteProtectionDisable) }},

		{[]string{}, func(o *updateOptions) *updateOptions { return o.WithHostOnly(false) }},
		{[]string{"--host_only"}, func(o *updateOptions) *updateOptions { return o.WithHostOnly(true) }},

		// WithGBBFlags tested in another test due to integer parsing.
	} {
		i := newTestInstance(nil, nil, nil)
		opts := p.setter(NewUpdateOptions("input.bin"))

		if _, err := i.Update(context.Background(), opts); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		ti := i.commandRunner.(*testCommandRunner)
		if err := ti.assertCalledWith(updatePositionalArgs, append([][]string{{"--image", "input.bin"}}, p.args), nil); err != nil {
			t.Errorf("unexpected command call: %v", err)
		}
	}
}

func Test_Update_Quirks(t *testing.T) {
	inputQuirks := map[string]string{
		"quirkA": "0",
		"quirkB": "2",
		"quirkC": "",
	}
	expectedQuirks := map[string]string{
		"quirkA": "0",
		"quirkB": "2",
		"quirkC": "1",
	}

	i := newTestInstance(nil, nil, nil)
	opts := NewUpdateOptions("input.bin").WithQuirks(inputQuirks)

	if _, err := i.Update(context.Background(), opts); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	ti := i.commandRunner.(*testCommandRunner)

	// Try to find --quirks.
	optionIndex := -1
	for i, v := range ti.args {
		if v == "--quirks" && i < (len(ti.args)-1) {
			optionIndex = i + 1
		}
	}
	if optionIndex < 0 {
		t.Fatalf("--quirks option not found in arguments: %v", ti.args)
	}
	// Parse quirks and store it for comparison.
	parsedQuirks := make(map[string]string)
	for _, v := range strings.Split(ti.args[optionIndex], ",") {
		q := strings.Split(v, "=")
		parsedQuirks[q[0]] = q[1]
	}
	if fmt.Sprint(parsedQuirks) != fmt.Sprint(expectedQuirks) {
		t.Errorf("command quirks do not match with expected quirks, expected: %q, got: %q", expectedQuirks, parsedQuirks)
	}

	// Drop {.., "--quirks", "quirks-list", ..}.
	ti.args = append(ti.args[:optionIndex-1], ti.args[optionIndex+1:]...)

	if err := ti.assertCalledWith(updatePositionalArgs, [][]string{{"--image", "input.bin"}}, nil); err != nil {
		t.Errorf("unexpected command call: %v", err)
	}
}

func Test_Update_GBBFlags(t *testing.T) {

	// Test multiple values.
	for _, v := range []int{0, 1, 0x4039, 0xFFFFFFFF} {
		i := newTestInstance(nil, nil, nil)
		opts := NewUpdateOptions("input.bin").WithGBBFlags(v)

		if _, err := i.Update(context.Background(), opts); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		ti := i.commandRunner.(*testCommandRunner)

		optionIndex := -1
		for i, v := range ti.args {
			if v == "--gbb_flags" && i < (len(ti.args)-1) {
				optionIndex = i + 1
			}
		}
		if optionIndex < 0 {
			t.Fatalf("--gbb_flags option not found in arguments: %v", ti.args)
		}
		flags, err := strconv.ParseInt(ti.args[optionIndex], 0, 64)
		if err != nil {
			t.Fatalf("failed to convert flags to integer: %v", err)
		}

		if int(flags) != v {
			t.Fatalf("flags do not match, expected %#X, got %#X", v, flags)
		}

		// Drop {.., "--gbb_flags", "<flags>", ..}.
		ti.args = append(ti.args[:optionIndex-1], ti.args[optionIndex+1:]...)

		if err := ti.assertCalledWith(updatePositionalArgs, [][]string{{"--image", "input.bin"}}, nil); err != nil {
			t.Errorf("unexpected command call: %v", err)
		}
	}
}
