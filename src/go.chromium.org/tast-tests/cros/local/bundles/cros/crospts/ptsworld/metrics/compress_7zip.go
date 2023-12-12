// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const compress7zipTitle = "7-Zip Compression"

// compress7zipCrosboltNameTable maps the PTS description to crosbolt name.
var compress7zipCrosboltNameTable = map[string]string{
	"Test: Compression Rating":   "Compression_Rating",
	"Test: Decompression Rating": "Decompression_Rating",
}
