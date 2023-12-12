// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const encodeMp3Title = "LAME MP3 Encoding"

// encodeMp3CrosboltNameTable maps the PTS description to crosbolt name.
var encodeMp3CrosboltNameTable = map[string]string{
	"WAV To MP3": "WAV_TO_MP3",
}
