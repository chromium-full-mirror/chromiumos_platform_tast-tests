// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const leveldbTitle = "LevelDB"

// leveldbCrosboltNameTable maps the PTS description to crosbolt name.
var leveldbCrosboltNameTable = map[string]string{
	"Benchmark: Sequential Fill": "Sequential_Fill",
	"Benchmark: Random Fill":     "Random_Fill",
	"Benchmark: Overwrite":       "Overwrite",
	"Benchmark: Fill Sync":       "Fill_Sync",
	"Benchmark: Random Read":     "Random_Read",
	"Benchmark: Random Delete":   "Random_Delete",
	"Benchmark: Hot Read":        "Hot_Read",
	"Benchmark: Seek Random":     "Seek_Random",
}
