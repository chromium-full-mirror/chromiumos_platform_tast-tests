// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import "chromiumos/tast/testing/hwdep"

// This file contains some shared constants for autoupdate tests.
const (
	TestFile              = "compat_testing_file"
	EncstatefulFile       = "/mnt/stateful_partition/encrypted/file"
	TestFileContent       = "content"
	ChromeDefaultUsername = "testuser@gmail.com"
	ChromeDefaultPassword = "testpass"
)

var hwsecRepresentativeModels = []string{
	// TPM1.2, ARM.
	"hana",
	// TPM1.2, x86.
	"asuka",
	// Cr50, ARM. A strongbad model.
	"homestar",
	// Cr50, x86. A nami model.
	"ekko",
	// Ti50, ARM. A corsola model.
	"tentacool",
	// Ti50, x86. A nissa model.
	"craaskbowl",
}

// HwsecRepresentativeModels is the hardware dependency to filter the models
// specified in hwsecRepresentativeModels.
// Autoupdate tests often fail on certain models which have weaker support
// by OMAHA (b/269211787). These tests aim for testing compatibility among
// versions. And compatibility for hwsec is more likely related to the
// combination of TPM type x architecture, so we don't need to run the
// autoupdate tests on all models. Instead we can just run them on the
// representative models that cover all the TPM type x arch combinations.
var HwsecRepresentativeModels = hwdep.Model(hwsecRepresentativeModels...)
