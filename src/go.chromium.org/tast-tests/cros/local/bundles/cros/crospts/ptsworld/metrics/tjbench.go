// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const tjbenchTitle = "libjpeg-turbo tjbench"

// tjbenchCrosboltNameTable maps the PTS description to crosbolt name.
var tjbenchCrosboltNameTable = map[string]string{
	"Test: Decompression Throughput": "Decompression_Throughput",
}
