// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crastestclient

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

// SampleFormat is a sample format used for the cras_test_client --format argument.
type SampleFormat string

// Known SampleFormat values supported by cras_tast_client.
const (
	// SampleFormatS16LE is the "S16_LE" SampleFormat.
	SampleFormatS16LE SampleFormat = "S16_LE"

	// SampleFormatS24LE is the "S24_LE" SampleFormat.
	SampleFormatS24LE SampleFormat = "S24_LE"

	// SampleFormatS32LE is the "S32_LE" SampleFormat.
	SampleFormatS32LE SampleFormat = "S32_LE"
)

// String returns SampleFormat as a string.
func (sf SampleFormat) String() string {
	return string(sf)
}

const cmdPath = "/usr/bin/cras_test_client"

// CmdBuilder is a utility for building cras_test_client commands, providing
// functions for known arguments. Call Build to get the resultant Cmd.
type CmdBuilder struct {
	playbackFile     string
	captureFile      string
	listenForHotword string
	loopbackFile     string
	pinDevice        string
	durationSeconds  int
	channels         int
	blockSize        int
	sampleRate       int
	sampleFormat     SampleFormat
}

// NewCmdBuilder creates a new CmdBuilder with reasonable argument defaults set.
//
// Deprecated: Use cras_tests with testexec.CommandContext instead.
// See go/cras_test_client_deprecation.
func NewCmdBuilder() *CmdBuilder {
	return &CmdBuilder{
		channels:   2,
		sampleRate: 48000,
	}
}

// Build returns a new testexec.Cmd for the cras_audio_client cmd with all set
// arguments added.
func (b *CmdBuilder) Build(ctx context.Context) *testexec.Cmd {
	var args []string
	if b.playbackFile != "" {
		args = append(args, "--playback_file", b.playbackFile)
	}
	if b.captureFile != "" {
		args = append(args, "--capture_file", b.captureFile)
	}
	if b.listenForHotword != "" {
		args = append(args, "--listen_for_hotword", b.listenForHotword)
	}
	if b.loopbackFile != "" {
		args = append(args, "--loopback_file", b.loopbackFile)
	}
	if b.pinDevice != "" {
		args = append(args, "--pin_device", b.pinDevice)
	}
	if b.blockSize > 0 {
		args = append(args, "--block_size", strconv.Itoa(b.blockSize))
	}
	if b.durationSeconds > 0 {
		args = append(args, "--duration", strconv.Itoa(b.durationSeconds))
	}
	if b.channels > 0 {
		args = append(args, "--num_channels", strconv.Itoa(b.channels))
	}
	if b.sampleRate > 0 {
		args = append(args, "--rate", strconv.Itoa(b.sampleRate))
	}
	if b.sampleFormat != "" {
		args = append(args, b.sampleFormat.String())
	}
	return testexec.CommandContext(ctx, cmdPath, args...)
}

// PlaybackFile sets the --playback_file argument value.
//
// This specifies the file to playback.
func (b *CmdBuilder) PlaybackFile(playbackFile string) *CmdBuilder {
	b.playbackFile = playbackFile
	return b
}

// CaptureFile sets the --capture_file argument value.
//
// The specifies the to save captured audio to.
func (b *CmdBuilder) CaptureFile(captureFile string) *CmdBuilder {
	b.captureFile = captureFile
	return b
}

// ListenForHotword sets the --listen_for_hotword argument value.
//
// This specifies the hotword to listen for.
func (b *CmdBuilder) ListenForHotword(listenForHotword string) *CmdBuilder {
	b.listenForHotword = listenForHotword
	return b
}

// LoopbackFile sets the --loopback_file argument value.
//
// This specified the file to save loopback to.
func (b *CmdBuilder) LoopbackFile(loopbackFile string) *CmdBuilder {
	b.loopbackFile = loopbackFile
	return b
}

// PinDevice sets the --pin_device argument value.
//
// This specified the device id to record or play from.
func (b *CmdBuilder) PinDevice(pinDevice string) *CmdBuilder {
	b.pinDevice = pinDevice
	return b
}

// BlockSize sets the --block_size argument value.
//
// This specifies the number of frames per callback (dictates latency).
func (b *CmdBuilder) BlockSize(blockSize int) *CmdBuilder {
	b.blockSize = blockSize
	return b
}

// Duration sets the --duration argument value.
//
// This specifies the duration, in seconds, of audio to capture or playback.
// When left unset, the process continues until terminated.
func (b *CmdBuilder) Duration(durationSeconds int) *CmdBuilder {
	b.durationSeconds = durationSeconds
	return b
}

// Channels sets the --num_channels argument value.
//
// This specifies the number of audio channels.
func (b *CmdBuilder) Channels(channels int) *CmdBuilder {
	b.channels = channels
	return b
}

// SampleRate sets the --rate argument value.
//
// This specifies the sampling rate.
func (b *CmdBuilder) SampleRate(sampleRate int) *CmdBuilder {
	b.sampleRate = sampleRate
	return b
}

// SampleFormat sets the --format argument value.
//
// This specifies the sampling format when capturing audio.
func (b *CmdBuilder) SampleFormat(sampleFormat SampleFormat) *CmdBuilder {
	b.sampleFormat = sampleFormat
	return b
}
