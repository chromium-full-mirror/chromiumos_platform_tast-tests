// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crastestclient

import (
	"context"
	"testing"

	audiopb "go.chromium.org/tast-tests/cros/services/cros/audio"
)

func TestCmdBuilder(t *testing.T) {
	type applyArgs func(b *CmdBuilder) *CmdBuilder
	tests := []struct {
		name      string
		applyArgs applyArgs
		wantCmd   string
	}{
		{
			"no args",
			func(b *CmdBuilder) *CmdBuilder {
				return b
			},
			cmdPath,
		},
		{
			"one int arg",
			func(b *CmdBuilder) *CmdBuilder {
				return b.Rate(123)
			},
			cmdPath + " --rate 123",
		},
		{
			"one string arg",
			func(b *CmdBuilder) *CmdBuilder {
				return b.PlaybackFile("some/file/path")
			},
			cmdPath + " --playback_file some/file/path",
		},
		{
			"one true bool arg",
			func(b *CmdBuilder) *CmdBuilder {
				return b.SetWbsEnabled(true)
			},
			cmdPath + " --set_wbs_enabled 1",
		},
		{
			"one false bool arg",
			func(b *CmdBuilder) *CmdBuilder {
				return b.SetWbsEnabled(false)
			},
			cmdPath + " --set_wbs_enabled 0",
		},
		{
			"two different args",
			func(b *CmdBuilder) *CmdBuilder {
				return b.Rate(123).SetWbsEnabled(false)
			},
			cmdPath + " --rate 123 --set_wbs_enabled 0",
		},
		{
			"two same args",
			func(b *CmdBuilder) *CmdBuilder {
				return b.Rate(123).Rate(456)
			},
			// Note: This is acceptable by builder, but relies on the CLI itself for
			// validity.
			cmdPath + " --rate 123 --rate 456",
		},
		{
			"effects single",
			func(b *CmdBuilder) *CmdBuilder {
				return b.Effects(audiopb.CaptureEffect_CAPTURE_EFFECT_AEC)
			},
			cmdPath + " --effects 0x01",
		},
		{
			"effects multiple",
			func(b *CmdBuilder) *CmdBuilder {
				return b.Effects(audiopb.CaptureEffect_CAPTURE_EFFECT_AGC, audiopb.CaptureEffect_CAPTURE_EFFECT_AGC_ON_DSP_ALLOWED)
			},
			cmdPath + " --effects 0x44",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewCmdBuilder()
			b = tt.applyArgs(b)
			cmd := b.Build(context.Background())
			if got := cmd.String(); got != tt.wantCmd {
				t.Errorf("Built command is '%s', want '%s'", got, tt.wantCmd)
			}
		})
	}
}
