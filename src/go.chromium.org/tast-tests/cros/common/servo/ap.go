// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"

	"go.chromium.org/tast/core/errors"
)

// These are the AP Servo controls which can be get/set with a string value.
const (
	APUARTCmd    StringControl = "cpu_uart_cmd"
	APUARTRegexp StringControl = "cpu_uart_regexp"
	APUARTStream StringControl = "cpu_uart_stream"
)

// These controls accept only "on" and "off" as values.
const (
	APUARTCapture OnOffControl = "cpu_uart_capture"
)

// RunAPCommand runs the given command on the AP-firmware console on the device.
func (s *Servo) RunAPCommand(ctx context.Context, cmd string) error {
	if err := s.SetString(ctx, APUARTRegexp, "None"); err != nil {
		return errors.Wrap(err, "Clearing AP UART Regexp")
	}
	return s.SetString(ctx, APUARTCmd, cmd)
}

// RunAPCommandGetOutput runs the given command on the AP on the device and
// returns the output matching patterns.
func (s *Servo) RunAPCommandGetOutput(ctx context.Context, cmd string, patterns []string) ([][]string, error) {
	err := s.SetStringList(ctx, APUARTRegexp, patterns)
	if err != nil {
		return nil, errors.Wrapf(err, "setting APUARTRegexp to %s", patterns)
	}
	defer s.SetString(ctx, APUARTRegexp, "None")
	err = s.SetString(ctx, APUARTCmd, cmd)
	if err != nil {
		return nil, errors.Wrapf(err, "setting APUARTCmd to %s", cmd)
	}
	iList, err := s.GetStringList(ctx, APUARTCmd)
	if err != nil {
		return nil, errors.Wrap(err, "decoding string list")
	}
	return ConvertToStringArrayArray(ctx, iList)
}
