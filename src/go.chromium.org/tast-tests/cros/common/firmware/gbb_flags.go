// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/servo"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/testing"
)

// allGBBFlags has all the GBB Flags in sorted order.
var allGBBFlags []pb.GBBFlag
var nonpreciousGBBFlags []pb.GBBFlag

func init() {
	for _, v := range pb.GBBFlag_value {
		allGBBFlags = append(allGBBFlags, pb.GBBFlag(v))
		if pb.GBBFlag(v) != pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC {
			nonpreciousGBBFlags = append(nonpreciousGBBFlags, pb.GBBFlag(v))
		}
	}
	sort.Slice(allGBBFlags, func(i, j int) bool { return allGBBFlags[i] < allGBBFlags[j] })
	sort.Slice(nonpreciousGBBFlags, func(i, j int) bool { return nonpreciousGBBFlags[i] < nonpreciousGBBFlags[j] })
}

// AllGBBFlags returns all the GBB Flags in order by their int values.
func AllGBBFlags() []pb.GBBFlag {
	return allGBBFlags
}

// NonpreciousGBBFlags returns all the GBB Flags in order by their int values, except for the precious ones like DISABLE_EC_SOFTWARE_SYNC.
func NonpreciousGBBFlags() []pb.GBBFlag {
	return nonpreciousGBBFlags
}

// FAFTGBBFlags returns the flags that faft sets before starting a test.
func FAFTGBBFlags() []pb.GBBFlag {
	return []pb.GBBFlag{pb.GBBFlag_RUNNING_FAFT}
}

// RebootRequiredGBBFlags returns flags that require a DUT reboot after they are changed.
func RebootRequiredGBBFlags() []pb.GBBFlag {
	return []pb.GBBFlag{pb.GBBFlag_FORCE_DEV_SWITCH_ON, pb.GBBFlag_DISABLE_EC_SOFTWARE_SYNC, pb.GBBFlag_FORCE_DEV_BOOT_USB}
}

// GBBFlagsStatesEqual determines if 2 GBBFlagsState are the same.
func GBBFlagsStatesEqual(a, b *pb.GBBFlagsState) bool {
	canonicalA := canonicalGBBFlagsState(a)
	canonicalB := canonicalGBBFlagsState(b)

	return reflect.DeepEqual(canonicalA.Clear, canonicalB.Clear) && reflect.DeepEqual(canonicalA.Set, canonicalB.Set)
}

// GBBFlagsChanged determines if any of the flags definitely have changed between a and b.
func GBBFlagsChanged(a, b *pb.GBBFlagsState, flags []pb.GBBFlag) bool {
	a = canonicalGBBFlagsState(a)
	b = canonicalGBBFlagsState(b)

	aClear := makeFlagsMap(a.Clear)
	aSet := makeFlagsMap(a.Set)
	bClear := makeFlagsMap(b.Clear)
	bSet := makeFlagsMap(b.Set)

	for _, f := range flags {
		_, inAClear := aClear[f]
		_, inASet := aSet[f]
		_, inBClear := bClear[f]
		_, inBSet := bSet[f]
		if (inAClear && inBSet) || (inASet && inBClear) {
			return true
		}
	}
	return false
}

// getGBBFlagsInt gets the flags that are set as an integer.
func getGBBFlagsInt(ctx context.Context, dut *dut.DUT) (uint32, error) {
	out, err := dut.Conn().CommandContext(ctx, "futility", "gbb", "--get", "--flags", "--flash").Output(exec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "futility gbb --get")
	}
	return getGBBFlagsIntConvert(out)
}

// getGBBFlagsIntByServo gets the flags that are set as an integer.
func getGBBFlagsIntByServo(ctx context.Context, pxy *servo.Proxy) (uint32, error) {
	out, err := pxy.OutputCommand(ctx, true, "futility", "gbb", "--get", "--servo_port", fmt.Sprintf("%d", pxy.GetPort()), "--flags")
	if err != nil {
		return 0, errors.Wrap(err, "futility gbb --get --servo_port")
	}
	return getGBBFlagsIntConvert(out)
}

// getGBBFlagsIntConvert converts the output of gbb flag reads from []byte to uint32
func getGBBFlagsIntConvert(out []byte) (uint32, error) {
	re, err := regexp.Compile(`flags: (0x[0-9a-fA-F]+)`)
	if err != nil {
		return 0, errors.Wrap(err, "parse gbb regex")
	}
	matches := re.FindSubmatch(out)
	if matches == nil {
		return 0, errors.Errorf("failed to find gbb flags in %s", string(out))
	}
	currentGBB64, err := strconv.ParseUint(string(matches[1]), 0, 32)
	if err != nil {
		return 0, errors.Wrapf(err, "parse gbb %q", string(matches[1]))
	}
	return uint32(currentGBB64), nil
}

// GetGBBFlags gets the flags that are cleared and set.
func GetGBBFlags(ctx context.Context, dut *dut.DUT) (*pb.GBBFlagsState, error) {
	currentGBB, err := getGBBFlagsInt(ctx, dut)
	if err != nil {
		return nil, err
	}
	testing.ContextLogf(ctx, "Current GBB flags = %#x", currentGBB)
	return &pb.GBBFlagsState{
		Clear: CalcGBBFlags(^currentGBB),
		Set:   CalcGBBFlags(currentGBB),
	}, nil
}

// GetGBBFlagsByServo gets the flags that are cleared and set.
// Uses servo connection
func GetGBBFlagsByServo(ctx context.Context, pxy *servo.Proxy) (*pb.GBBFlagsState, error) {
	currentGBB, err := getGBBFlagsIntByServo(ctx, pxy)
	if err != nil {
		return nil, err
	}
	testing.ContextLogf(ctx, "Current GBB flags = %#x", currentGBB)
	return &pb.GBBFlagsState{
		Clear: CalcGBBFlags(^currentGBB),
		Set:   CalcGBBFlags(currentGBB),
	}, nil
}

type getGbbFlagsFunc func(context.Context) (uint32, error)
type setGbbFlagsFunc func(context.Context, uint32) error

// ClearAndSetGBBFlags clears and sets specified GBB flags, leaving the rest unchanged.
func ClearAndSetGBBFlags(ctx context.Context, dut *dut.DUT, state *pb.GBBFlagsState) (bool, error) {
	return clearAndSetGBBFlagsImpl(
		ctx,
		state,
		func(ctx context.Context) (uint32, error) { return getGBBFlagsInt(ctx, dut) },
		func(ctx context.Context, newFlags uint32) error {
			return dut.Conn().CommandContext(ctx, "futility", "gbb", "--set", "--flash", fmt.Sprintf("--flags=%#x", newFlags)).Run(exec.DumpLogOnError)
		})
}

// ClearAndSetGBBFlagsByServo clears and sets specified GBB flags, leaving the rest unchanged.
// Uses servo connection
func ClearAndSetGBBFlagsByServo(ctx context.Context, pxy *servo.Proxy, state *pb.GBBFlagsState) (bool, error) {
	return clearAndSetGBBFlagsImpl(
		ctx,
		state,
		func(ctx context.Context) (uint32, error) { return getGBBFlagsIntByServo(ctx, pxy) },
		func(ctx context.Context, newFlags uint32) error {
			return pxy.RunCommand(ctx, true, "futility", "gbb", "--servo_port", fmt.Sprintf("%d", pxy.GetPort()), "--set", "--flags", fmt.Sprintf("%#x", newFlags))
		})
}

func clearAndSetGBBFlagsImpl(ctx context.Context, state *pb.GBBFlagsState, getFlags getGbbFlagsFunc, setFlags setGbbFlagsFunc) (bool, error) {
	gbbFlagChanged := false
	state = canonicalGBBFlagsState(state)
	currentGBB, err := getFlags(ctx)
	if err != nil {
		return gbbFlagChanged, err
	}
	clearMask := CalcGBBMask(state.Clear)
	setMask := CalcGBBMask(state.Set)
	testing.ContextLogf(ctx, "Current GBB flags = %#x, want clear %#x, set %#x", currentGBB, clearMask, setMask)
	newGBB := CalcGBBBits(currentGBB, clearMask, setMask)
	if newGBB != currentGBB {
		testing.ContextLogf(ctx, "Setting GBB flags = %#x", newGBB)
		if err := setFlags(ctx, newGBB); err != nil {
			return gbbFlagChanged, errors.Wrap(err, "futility gbb --set")
		}
		gbbFlagChanged = true
	} else {
		testing.ContextLog(ctx, "No GBB change required")
	}
	return gbbFlagChanged, nil
}

// GBBFlagsStateClearSet takes an initial state, and applies the changes in apply to it. It is identical to ClearAndSetGBBFlags, except that it acts on a proto instead of the DUT.
func GBBFlagsStateClearSet(initial, apply *pb.GBBFlagsState) *pb.GBBFlagsState {
	currentGBB := CalcGBBMask(initial.Set)
	clearMask := CalcGBBMask(apply.Clear)
	setMask := CalcGBBMask(apply.Set)
	newGBB := CalcGBBBits(currentGBB, clearMask, setMask)
	return &pb.GBBFlagsState{
		Set: CalcGBBFlags(newGBB),
	}
}

// CalcGBBBits returns the final GBB bits after applying clear and set to curr.
// Set has precedence over clear in the same bit position.
func CalcGBBBits(curr, clear, set uint32) uint32 {
	return (curr & ^clear) | set
}

// SetGBBFlags ignores the previous GBB flags and sets them to the specified flags.
func SetGBBFlags(ctx context.Context, dut *dut.DUT, flags []pb.GBBFlag) error {
	setMask := CalcGBBMask(flags)
	testing.ContextLogf(ctx, "Setting GBB flags = %#x", setMask)
	if err := dut.Conn().CommandContext(ctx, "futility", "gbb", "--set", "--flash", fmt.Sprintf("--flags=%#x", setMask)).Run(exec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "futility gbb --set")
	}
	return nil
}

// CalcGBBFlags interprets mask as a GBBFlag bit mask and returns the set flags.
func CalcGBBFlags(mask uint32) []pb.GBBFlag {
	var res []pb.GBBFlag
	for _, pos := range AllGBBFlags() {
		if mask&(0x0001<<pos) != 0 {
			res = append(res, pb.GBBFlag(pos))
		}
	}
	return res
}

// CalcGBBMask returns the bit mask corresponding to the list of GBBFlags.
func CalcGBBMask(flags []pb.GBBFlag) uint32 {
	var mask uint32
	for _, f := range flags {
		mask |= 0x0001 << f
	}
	return mask
}

// makeFlagsMap converts a slice of GBBFlags into a map for easy lookup.
func makeFlagsMap(f []pb.GBBFlag) map[pb.GBBFlag]bool {
	m := make(map[pb.GBBFlag]bool)
	for _, f := range f {
		m[f] = true
	}
	return m
}

// canonicalGBBFlagsState standardizes the GBBFlagsState so that they can be more readily compared.  In particular, a flag in both Set and Clear will be deleted from Clear.  The flags are also sorted.
func canonicalGBBFlagsState(s *pb.GBBFlagsState) *pb.GBBFlagsState {
	setMap := makeFlagsMap(s.Set)
	clearMap := makeFlagsMap(s.Clear)

	var canonicalClear []pb.GBBFlag
	var canonicalSet []pb.GBBFlag

	for _, v := range allGBBFlags {
		if _, sOk := setMap[v]; sOk {
			canonicalSet = append(canonicalSet, v)
		} else if _, cOk := clearMap[v]; cOk {
			canonicalClear = append(canonicalClear, v)
		}
	}

	return &pb.GBBFlagsState{Clear: canonicalClear, Set: canonicalSet}
}

// GBBToggle adds `flag` to `flags` if it is missing, or removes it if it is present. Returns a new list, and does not modify the `flags` slice.
func GBBToggle(flags []pb.GBBFlag, flag pb.GBBFlag) []pb.GBBFlag {
	var ret []pb.GBBFlag
	found := false
	for _, v := range flags {
		if v == flag {
			found = true
		} else {
			ret = append(ret, v)
		}
	}
	if !found {
		ret = append(ret, flag)
	}
	return ret
}

// GBBAddFlag modifies `s` to add all flags in `flags`.
func GBBAddFlag(s *pb.GBBFlagsState, flags ...pb.GBBFlag) {
	s.Set = append(s.Set, flags...)
	newS := canonicalGBBFlagsState(s)
	s.Set = newS.Set
	s.Clear = newS.Clear
}

// CopyGBBFlags returns a new GBBFlagsState that is a copy of `s`.
func CopyGBBFlags(s *pb.GBBFlagsState) *pb.GBBFlagsState {
	// Depends on the behavior of canonicalGBBFlagsState to always return a copy with new Set & Clear arrays.
	ret := canonicalGBBFlagsState(s)
	return ret
}

// GBBFlagsContains returns true if `s` contains the requested GBB flag `flag`.
func GBBFlagsContains(s *pb.GBBFlagsState, flag pb.GBBFlag) bool {
	for _, f := range s.Set {
		if f == flag {
			return true
		}
	}
	return false
}
