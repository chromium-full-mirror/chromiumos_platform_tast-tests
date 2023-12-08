// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const mbwTitle = "MBW"

// mbwCrosboltNameTable maps the PTS description to crosbolt name.
var mbwCrosboltNameTable = map[string]string{
	"Test: Memory Copy - Array Size: 1024 MiB":                   "Memory_Copy_Array_Size_1024",
	"Test: Memory Copy, Fixed Block Size - Array Size: 1024 MiB": "Memory_Copy_Fixed_Block_Size_Array_Size_1024",
}
