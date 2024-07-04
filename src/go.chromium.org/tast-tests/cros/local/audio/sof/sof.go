// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sof

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

// DSPEffect tells the effect type on DSP.
type DSPEffect int

// DSP effect types.
const (
	DSPNoiseCancellation DSPEffect = iota
	DSPEchoCancellation
)

func (e DSPEffect) String() string {
	switch e {
	case DSPNoiseCancellation:
		return "nc"
	case DSPEchoCancellation:
		return "aec"
	default:
		return fmt.Sprintf("unknown%d", int(e))
	}
}

// DSPEffectState tells whether an effect is available or in use.
type DSPEffectState int

// DSP effect states.
const (
	DSPEffectUnavailable DSPEffectState = iota
	DSPEffectExists
	DSPEffectOff
	DSPEffectOn
)

func (s DSPEffectState) String() string {
	switch s {
	case DSPEffectUnavailable:
		return "Unavailable"
	case DSPEffectExists:
		return "Exists"
	case DSPEffectOff:
		return "Off"
	case DSPEffectOn:
		return "On"
	default:
		return fmt.Sprintf("UnknownState%d", int(s))
	}
}

// ProfileArtifact is the inner struct for Profile.
type ProfileArtifact struct {
	Name         string `json:"name"`
	Prefix       string `json:"prefix"`
	IsSymlink    bool   `json:"is_symlink"`
	ResolvedPath string `json:"resolved_path"`
}

// Profile is the unmarshalled struct from sof_helper devtool dump.
type Profile struct {
	Firmware ProfileArtifact `json:"fw"`
	Topology ProfileArtifact `json:"tplg"`
}

var errProfileNoInquiry = errors.New("no inquiry into profile")

// IsProfileNoInquiry tells if the given error is of no inquiry.
func IsProfileNoInquiry(err error) bool {
	return errors.Is(err, errProfileNoInquiry)
}

// GetProfile fetches data in Profile from sof_helper devtool dump.
func GetProfile(ctx context.Context) (*Profile, error) {
	cmd := testexec.CommandContext(ctx, "sof_helper", "profile", "--json")
	stdout, stderr, err := cmd.SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		// There is no inquiry into SOF profile.
		if strings.Contains(string(stderr), "cannot find profile") {
			return nil, errProfileNoInquiry
		}
		return nil, errors.Wrap(err, "call sof_helper")
	}

	prof := &Profile{}
	if err := json.Unmarshal(stdout, prof); err != nil {
		return nil, errors.Wrap(err, "unmarshal SOF profile")
	}
	return prof, nil
}

func dspEffectStateFromString(str string) (DSPEffectState, error) {
	switch str {
	case DSPEffectExists.String():
		return DSPEffectExists, nil
	case DSPEffectOff.String():
		return DSPEffectOff, nil
	case DSPEffectOn.String():
		return DSPEffectOn, nil
	default:
		return DSPEffectUnavailable, errors.Errorf("invalid string=%s", str)
	}
}

// GetCstate fetches component state of DSP effect from sof_helper devtool dump.
func GetCstate(ctx context.Context, effect DSPEffect) (DSPEffectState, error) {
	cmd := testexec.CommandContext(ctx, "sof_helper", "cstate", effect.String(), "--expect")
	stdout, stderr, err := cmd.SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		// Catch errors with no control detected for the given DSP effect.
		if strings.Contains(string(stderr), "no control is detected") {
			return DSPEffectUnavailable, nil
		}
		return DSPEffectUnavailable, errors.Wrap(err, "call sof_tests")
	}
	return dspEffectStateFromString(strings.TrimSpace(string(stdout)))
}
