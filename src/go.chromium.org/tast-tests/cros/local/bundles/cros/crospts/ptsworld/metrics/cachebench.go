// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const cachebenchTitle = "CacheBench"

// cachebenchCrosboltNameTable maps the PTS description to crosbolt name.
var cachebenchCrosboltNameTable = map[string]string{
	"Test: Read":                  "Read",
	"Test: Write":                 "Write",
	"Test: Read / Modify / Write": "Read_Modify_Write",
}
