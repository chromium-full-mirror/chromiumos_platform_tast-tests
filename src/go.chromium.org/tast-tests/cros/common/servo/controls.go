// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"regexp"

	"go.chromium.org/tast/core/errors"
)

// ClearServoAccumulators clears underlying accumulators given the controls.
func ClearServoAccumulators(ctx context.Context, s *Servo, clearCtrls []IntControl) error {
	for _, ctrl := range clearCtrls {
		if err := s.SetInt(ctx, ctrl, 1); err != nil {
			return errors.Errorf("failed to clear %s: %s", ctrl, err)
		}
	}

	return nil
}

func findPowerRails(ctx context.Context, s *Servo, useAccum bool) ([]FloatControl, []IntControl, error) {
	railType := "avg_power_rails"
	if !useAccum {
		railType = "power_rails"
	}

	ctrlStrs, err := s.GetStringArray(ctx, railType)
	if err != nil {
		return []FloatControl{}, []IntControl{}, errors.Wrap(err, "failed to get response from servo instance")
	}
	if len(ctrlStrs) == 0 {
		return []FloatControl{}, []IntControl{}, errors.Errorf("failed to detect support for rails with tag: %q", railType)
	}
	var ctrls []FloatControl
	for _, c := range ctrlStrs {
		ctrls = append(ctrls, FloatControl(c))
	}

	// Return early if using power_rails.
	if !useAccum {
		return ctrls, []IntControl{}, nil
	}

	// Collect accumulator rails.
	clearCtrlStrs, err := s.GetStringArray(ctx, "accum_clear_ctrls")
	if err != nil {
		return []FloatControl{}, []IntControl{}, errors.Wrap(err, "failed to get response from servo instance")
	}
	if (len(clearCtrlStrs) == 0) || len(ctrlStrs) != len(clearCtrlStrs) {
		return []FloatControl{}, []IntControl{}, errors.New("failed to detect support for accum rails")
	}
	var clearCtrls []IntControl
	for _, c := range clearCtrlStrs {
		clearCtrls = append(clearCtrls, IntControl(c))
	}
	return ctrls, clearCtrls, nil
}

// FindPowerRailsWithFilter gets results for power rails and applies given filter to results.
//
// If useAccumulators is false then this will fallback to using 'power_rails' rather than 'avg_power_rails'
func FindPowerRailsWithFilter(ctx context.Context, s *Servo, useAccumulators bool, filters []*regexp.Regexp) ([]FloatControl, []IntControl, error) {
	ctrls, clearCtrls, err := findPowerRails(ctx, s, useAccumulators)
	if err != nil {
		return []FloatControl{}, []IntControl{}, errors.Wrap(err, "failed to get power rails")
	}
	if len(filters) == 0 {
		return ctrls, clearCtrls, nil
	}
	var rails []FloatControl
	for _, ctrl := range ctrls {
		matchesAll := true
		for _, filter := range filters {
			if !filter.MatchString(string(ctrl)) {
				matchesAll = false
				break
			}
		}
		if matchesAll {
			rails = append(rails, ctrl)
		}
	}
	var clearRails []IntControl
	for _, ctrl := range clearCtrls {
		matchesAll := true
		for _, filter := range filters {
			if !filter.MatchString(string(ctrl)) {
				matchesAll = false
				break
			}
		}
		if matchesAll {
			clearRails = append(clearRails, ctrl)
		}
	}
	return rails, clearRails, nil
}
