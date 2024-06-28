// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/errors"
)

// EffectState tells whether an effect is in use.
type EffectState int

// Effect states.
const (
	EffectUnavailable EffectState = iota
	EffectDisabled
	EffectEnabled
)

func (s EffectState) String() string {
	switch s {
	case EffectUnavailable:
		return "EffectUnavailable"
	case EffectDisabled:
		return "EffectDisabled"
	case EffectEnabled:
		return "EffectEnabled"
	default:
		return fmt.Sprintf("EffectState%d", int(s))
	}
}

func amixerCget(ctx context.Context, cardID, nameExpr string) (EffectState, error) {
	available, err := hasControl(ctx, cardID, nameExpr)
	if err != nil {
		return EffectUnavailable, errors.Wrap(err, "hasControl()")
	}
	if !available {
		return EffectUnavailable, nil
	}

	bout, berr, err := testexec.CommandContext(ctx, "amixer", "-c"+cardID, "cget", nameExpr).SeparatedOutput()
	stdout := string(bout)
	stderr := string(berr)
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok &&
			exitErr.ExitCode() == 1 &&
			strings.Contains(stderr, "Cannot find the given element from control") {
			return EffectUnavailable, nil
		}
		// Other exit codes are unexpected.
		return EffectUnavailable, errors.Wrapf(err, "cannot get %q; stderr: %q", nameExpr, stderr)
	}
	if strings.Contains(stdout, ": values=off") {
		return EffectDisabled, nil
	}
	if strings.Contains(stdout, ": values=on") {
		return EffectEnabled, nil
	}
	return EffectUnavailable, errors.Errorf("cannot tell %q state from %q; stderr: %q", nameExpr, stdout, stderr)
}

func hasCardID(cards []audio.Card, id string) bool {
	for _, card := range cards {
		if card.ID == id {
			return true
		}
	}
	return false
}

// DSPEchoCancellationState tells whether DSP echo cancellation is active.
func DSPEchoCancellationState(ctx context.Context) (EffectState, error) {
	cards, err := audio.GetSoundCards()
	if err != nil {
		return EffectUnavailable, errors.Wrap(err, "failed to GetSoundCards()")
	}

	// Redrix and friends.
	if id := "sofrt5682"; hasCardID(cards, id) {
		const nameExpr = "name='GOOGLE_RTC_AUDIO_PROCESSING10.0 Config'"

		available, err := hasControl(ctx, id, nameExpr)
		if err != nil {
			return EffectUnavailable, errors.Wrap(err, "hasControl()")
		}
		if !available {
			return EffectUnavailable, nil
		}

		b, err := testexec.CommandContext(ctx, "sof-ctl", "-Dhw:"+id, "-c", nameExpr, "-r", "-b").CombinedOutput(testexec.DumpLogOnError)
		if err != nil {
			return EffectUnavailable, errors.Wrap(err, "error running sof-ctl")
		}
		s := string(b)
		if strings.Contains(s, "00000000 040a 0232 0008") {
			return EffectDisabled, nil
		}
		if strings.Contains(s, "00000000 040a 0232 0108") {
			return EffectEnabled, nil
		}
		return EffectUnavailable, errors.Errorf("cannot tell DSP AEC status from %q", s)
	}

	return EffectUnavailable, nil
}

// DSPNoiseCancellationState tells whether DSP noise cancellation is active.
func DSPNoiseCancellationState(ctx context.Context) (EffectState, error) {
	cards, err := audio.GetSoundCards()
	if err != nil {
		return EffectUnavailable, errors.Wrap(err, "failed to GetSoundCards()")
	}

	// Control used by redrix and friends.
	if id := "sofrt5682"; hasCardID(cards, id) {
		return amixerCget(ctx, id, "name='RTNR10.0 rtnr_enable_10'")
	}
	// Control used by dojo.
	if id := "sofm8195m983905"; hasCardID(cards, id) {
		return amixerCget(ctx, id, "name='RTNR3.0 rtnr_enable_3'")
	}
	return EffectUnavailable, nil
}

func hasControl(ctx context.Context, cardID, nameExpr string) (bool, error) {
	output, err := testexec.CommandContext(ctx, "amixer", "-c"+cardID, "controls").Output(testexec.DumpLogOnError)
	if err != nil {
		return false, errors.Wrap(err, "error running amixer")
	}
	return strings.Contains(string(output), nameExpr), nil
}
