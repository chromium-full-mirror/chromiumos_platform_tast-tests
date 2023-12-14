// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const pyPerformanceTitle = "PyPerformance"

// pyPerformanceCrosboltNameTable maps the PTS description to crosbolt name.
var pyPerformanceCrosboltNameTable = map[string]string{
	"Benchmark: float":          "float",
	"Benchmark: pathlib":        "pathlib",
	"Benchmark: json_loads":     "json_loads",
	"Benchmark: regex_compile":  "regex_compile",
	"Benchmark: python_startup": "python_startup",
}
