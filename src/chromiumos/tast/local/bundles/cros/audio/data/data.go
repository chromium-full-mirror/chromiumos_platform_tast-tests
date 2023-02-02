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
	TheQuickBrownFoxWav         = "the-quick-brown-fox.wav"
	TheQuickBrownFoxWavDuration = 3410 * time.Millisecond
)
