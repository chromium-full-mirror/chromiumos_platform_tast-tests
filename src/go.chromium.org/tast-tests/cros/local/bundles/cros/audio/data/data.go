// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package data provide information about data files
package data

import "time"

// the-quick-brown-fox.wav is speech generated with:
//
//	gtts-cli 'The quick brown fox jumps over the lazy dog' --output=the-quick-brown-fox.mp3
//	ffmpeg -i the-quick-brown-fox.mp3 the-quick-brown-fox.wav
const (
	TheQuickBrownFoxWav                 = "the-quick-brown-fox.wav"
	TheQuickBrownFoxWavDuration         = 3410 * time.Millisecond
	TheQuickBrownFoxS16LEStereo48000Wav = "the-quick-brown-fox-s16le-stereo-48000_20240528.wav"
)

const (
	// AudioLong16Wav is from https://source.chromium.org/chromium/chromium/src/+/main:third_party/webrtc/data/voice_engine/audio_long16.wav;l=1;drc=9dc45dad1b14680f2ae0ae75dc497250e9ecd384
	AudioLong16Wav = "audio_long16.wav"
	// AudioShortSine440Wav is a 5ms sinewave with 1s of silence in front.
	AudioShortSine440Wav = "audio_short_sine_440.wav"
)

const (
	// WavesMaxxChromeConfig omits the tuning parameter files.
	WavesMaxxChromeConfig = "maxx_conf.ini"
	// WavesMaxxChromeGoldenInput is the golden data provided by Waves.
	WavesMaxxChromeGoldenInput = "waves_maxx_chrome_golden_input.wav"
	// WavesMaxxChromeGoldenOutput is the golden data provided by Waves.
	WavesMaxxChromeGoldenOutput = "waves_maxx_chrome_golden_output.wav"
)
