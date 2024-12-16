// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"reflect"
	"testing"

	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
)

// flags converts a list of ints to a slice of pb.GBBFlags.
func flags(f ...int) []pb.GBBFlag {
	var fs []pb.GBBFlag
	for _, i := range f {
		fs = append(fs, pb.GBBFlag(i))
	}
	return fs
}

// state constructs a GBBFlagsState.
func state(clear, set []pb.GBBFlag) *pb.GBBFlagsState {
	return &pb.GBBFlagsState{Clear: clear, Set: set}
}

func TestCanonicalGBBFlagState(t *testing.T) {
	states := []struct {
		a *pb.GBBFlagsState
		c *pb.GBBFlagsState
	}{
		{state(flags(), flags()), state(flags(), flags())},
		{state(flags(0), flags()), state(flags(0), flags())},
		{state(flags(), flags(0)), state(flags(), flags(0))},
		{state(flags(1, 2, 3), flags(4, 5, 6)), state(flags(1, 2, 3), flags(4, 5, 6))},
		{state(flags(3, 2, 1), flags(6, 5, 4)), state(flags(1, 2, 3), flags(4, 5, 6))},
	}

	for _, s := range states {
		cA := canonicalGBBFlagsState(s.a)
		if !reflect.DeepEqual(cA.Clear, s.c.Clear) {
			t.Errorf("Clear incorrect for canonical %v: \ngot\n%v\nwant\n%v\n\n", s.a.Clear, cA.Clear, s.c.Clear)
		}
		if !reflect.DeepEqual(cA.Set, s.c.Set) {
			t.Errorf("Set incorrect for canonical %v: \ngot\n%v\nwant\n%v\n\n", s.a.Set, cA.Set, s.c.Set)
		}
	}
}

func TestGBBFlagsStatesEqual(t *testing.T) {
	states := []struct {
		a    *pb.GBBFlagsState
		b    *pb.GBBFlagsState
		want bool
	}{
		{state(flags(), flags()), state(flags(), flags()), true},
		{state(flags(0), flags()), state(flags(0), flags()), true},
		{state(flags(), flags(0)), state(flags(), flags(0)), true},
		{state(flags(1, 2, 3), flags(4, 5, 6)), state(flags(1, 2, 3), flags(4, 5, 6)), true},
		{state(flags(3, 2, 1), flags(6, 5, 4)), state(flags(1, 2, 3), flags(4, 5, 6)), true},
		{state(flags(3, 2, 1), flags(6, 5, 4)), state(flags(3, 2, 1), flags(6, 5, 4)), true},
		{state(flags(), flags()), state(flags(0), flags()), false},
		{state(flags(0), flags()), state(flags(), flags(0)), false},
		{state(flags(0), flags(0)), state(flags(0), flags(1)), false},
		{state(flags(0), flags(0)), state(flags(1), flags(0)), false},
		{state(flags(0, 1), flags(0, 1)), state(flags(0, 1), flags(0, 2)), false},
	}

	for _, s := range states {
		got := GBBFlagsStatesEqual(s.a, s.b)
		if got != s.want {
			t.Errorf("Comparing\n%+v\nand\n%+v\ngot %v, want %v", s.a, s.b, got, s.want)
		}
	}
}

func TestGBBFlagsChanged(t *testing.T) {
	states := []struct {
		a    *pb.GBBFlagsState
		b    *pb.GBBFlagsState
		f    []pb.GBBFlag
		want bool
	}{
		{state(flags(), flags()), state(flags(), flags()), flags(0), false},
		{state(flags(0), flags(1)), state(flags(1), flags(0)), flags(2), false},
		{state(flags(0), flags()), state(flags(0), flags()), flags(0), false},
		{state(flags(), flags(0)), state(flags(), flags(0)), flags(0), false},
		{state(flags(), flags(0)), state(flags(), flags(0)), flags(0), false},
		{state(flags(0), flags()), state(flags(1), flags()), flags(0), false},
		{state(flags(0), flags()), state(flags(), flags(0)), flags(0), true},
		{state(flags(), flags(0)), state(flags(0), flags()), flags(0), true},
		{state(flags(), flags(0)), state(flags(0), flags()), flags(0), true},
		{state(flags(0, 1), flags(0, 1, 2)), state(flags(0, 2), flags()), flags(0), true},
	}
	for _, s := range states {
		got := GBBFlagsChanged(s.a, s.b, s.f)
		if got != s.want {
			t.Errorf("Flags %v changed from\n%v\nto%v\ngot %v, want %v", s.f, s.a, s.b, got, s.want)
		}
	}
}

func TestAllGBBFlags(t *testing.T) {
	var all []int
	for i := 0; i < len(pb.GBBFlag_value); i++ {
		all = append(all, i)
	}
	want := flags(all...)
	got := AllGBBFlags()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("All flags\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCalcGBBBits(t *testing.T) {
	tests := []struct {
		curr  uint32
		clear uint32
		set   uint32
		want  uint32
	}{
		{0b0101, 0b0000, 0b0000, 0b0101},
		{0b0101, 0b1111, 0b0000, 0b0000},
		{0b0100, 0b0000, 0b0001, 0b0101},
		{0b0010, 0b0010, 0b0001, 0b0001},
		{0b0011, 0b1100, 0b1100, 0b1111},
		{0b0101, 0b1010, 0b0101, 0b0101},
	}

	for _, tc := range tests {
		got := CalcGBBBits(tc.curr, tc.clear, tc.set)
		if got != tc.want {
			t.Errorf("calcGBBBits, updating %04b with %04b(clear) and %04b(set), got %04b, want %04b", tc.curr, tc.clear, tc.set, got, tc.want)
		}
	}
}

func TestCalcGBB(t *testing.T) {
	// 1 bit
	m := CalcGBBMask([]pb.GBBFlag{pb.GBBFlag_DEV_SCREEN_SHORT_DELAY})
	if m != 0x0001<<pb.GBBFlag_DEV_SCREEN_SHORT_DELAY {
		t.Fatalf("unexpected mask for 1 bit: %v", m)
	}

	f := CalcGBBFlags(m)
	if len(f) != 1 || f[0] != pb.GBBFlag_DEV_SCREEN_SHORT_DELAY {
		t.Fatalf("unexpected flagfor 1 bit: %v", f)
	}

	// 2 bits
	m = CalcGBBMask([]pb.GBBFlag{pb.GBBFlag_DEV_SCREEN_SHORT_DELAY, pb.GBBFlag_FORCE_MANUAL_RECOVERY})

	if m != (0x0001<<pb.GBBFlag_DEV_SCREEN_SHORT_DELAY)|(0x0001<<pb.GBBFlag_FORCE_MANUAL_RECOVERY) {
		t.Fatalf("unexpected mask for 2 bits: %v", m)
	}

	f = CalcGBBFlags(m)
	if len(f) != 2 || f[0] != pb.GBBFlag_DEV_SCREEN_SHORT_DELAY || f[1] != pb.GBBFlag_FORCE_MANUAL_RECOVERY {
		t.Fatalf("unexpected flags for 2 bits: %v", f)
	}
}
