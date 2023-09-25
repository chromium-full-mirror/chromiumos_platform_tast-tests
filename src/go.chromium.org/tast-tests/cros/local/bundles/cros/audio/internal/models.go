// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

// Model list.
var (
	// Models that support DSP AEC.
	DSPAECModels = []string{"redrix", "gimble", "anahera", "yaviks", "yavikso"}
	// Models that support both DSP NC and AP NC.
	DSPAPNCModels = []string{"redrix", "gimble", "anahera", "yaviks", "yavikso"}
	// Models that support DSP NC but not AP NC.
	DSPNCOnlyModels = []string{"dojo"}
)
