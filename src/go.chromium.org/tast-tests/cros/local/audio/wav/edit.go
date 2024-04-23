// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wav

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

type wavConfig struct {
	channels int
	rate     int
}

func newWavConfig() *wavConfig {
	return &wavConfig{
		channels: 2,
		rate:     48000,
	}
}

// Option for WAVE files.
type Option func(*wavConfig)

// Channels returns an Option to set the channels of a wave file.
func Channels(channels int) Option {
	return func(wc *wavConfig) {
		wc.channels = channels
	}
}

// Rate returns an Option to set the rate of a WAVE file.
func Rate(rate int) Option {
	return func(wc *wavConfig) {
		wc.rate = rate
	}
}

// RepeatForDuration repeats then input WAVE file for duration and writes it to output.
func RepeatForDuration(ctx context.Context, input, output string, duration time.Duration, options ...Option) error {
	wc := newWavConfig()
	for _, opt := range options {
		opt(wc)
	}
	return testexec.CommandContext(
		ctx,
		"sox",
		input,
		fmt.Sprintf("--channels=%d", wc.channels),
		fmt.Sprintf("--rate=%d", wc.rate),
		output,
		"repeat", "-",
		"trim", "0", strconv.FormatFloat(duration.Seconds(), 'f', 0, 64),
	).Run(testexec.DumpLogOnError)
}
