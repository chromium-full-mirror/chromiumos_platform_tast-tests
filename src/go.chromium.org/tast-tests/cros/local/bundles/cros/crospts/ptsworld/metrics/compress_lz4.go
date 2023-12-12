// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const compresslz4Title = "LZ4 Compression"

// compresslz4CrosboltNameTable maps the PTS description to crosbolt name.
var compresslz4CrosboltNameTable = map[string]string{
	"Compression Level: 3 - Decompression Speed": "Decompression_Level_3",
	"Compression Level: 9 - Compression Speed":   "Compression_Level_9",
}
