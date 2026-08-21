// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

// Model list.
var (
	// Models that support DSP AEC.
	DSPAECModels = []string{"yaviks", "yavikso"}
	// Models that support both DSP AEC and DSP NC.
	DSPAECNCModels = []string{"yaviks", "yavikso"}
	// Models that support DSP NC but not DSP AEC.
	DSPNCOnlyModels = []string{"dojo"}
	// Models that support Waves output processing.
	WavesOutputModels = []string{"obiwan", "quigon"}
)
