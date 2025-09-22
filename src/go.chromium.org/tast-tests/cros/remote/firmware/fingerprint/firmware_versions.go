// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import "go.chromium.org/tast-tests/cros/common/fingerprint"

// GENERATED CODE
// The following table is partially generated using the
// fpmcu-firmware-binaries/generate_test_versions.py script.

// Map of attributes for a given board's various firmware file releases.
//
// Keep the latest 2 or 3 versions to allow easier testing against other
// release channels, like stable and beta.
//
// Two purposes:
//  1. Documents the exact versions and keys used for a given firmware file.
//  2. Verifies files that end up in the build are valid firmware image files.
//     TODO(b/382782212): Add explicit check for ToT builds to ensure the single
//     latest firmware file was used.
var firmwareVersionMap = map[fingerprint.BoardName]map[string]firmwareMetadata{
	fingerprint.BoardNameBloonchipper: {
		"bloonchipper_v2.0.4277-9f652bb3-RO_v2.0.30777-fc2f44c-RW.bin": {
			sha256sum: "33e545915778c10eddc3810a24acf934e4dceb12e35b2b61996209b985a16c3f",
			roVersion: "bloonchipper_v2.0.4277-9f652bb3",
			rwVersion: "bloonchipper-v2.0.30777-fc2f44c",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
		"bloonchipper_v2.0.5938-197506c1-RO_v2.0.30777-fc2f44c-RW.bin": {
			sha256sum: "998268da5ffcee62028b7ffa73ad5476bee5b056c1b8f364879df29766f669ea",
			roVersion: "bloonchipper_v2.0.5938-197506c1",
			rwVersion: "bloonchipper-v2.0.30777-fc2f44c",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
	},
	fingerprint.BoardNameBuccaneer: {
		"buccaneer_v2.0.26328-821504380b-RO_v2.0.30479-3de9c9a0fe-RW.bin": {
			sha256sum: "d1dcbad8ab2079023be4f7d98149a740d1b5b6780e2d22b881459852844af8b7",
			roVersion: "buccaneer_v2.0.26328-821504380b",
			rwVersion: "buccaneer_v2.0.30479-3de9c9a0fe",
			keyID:     "95fb0d0a5f1c1f658a0526430a3a184301421e32",
		},
	},
	fingerprint.BoardNameDartmonkey: {
		"dartmonkey_v2.0.2887-311310808-RO_v2.0.29922-e70d27b41-RW.bin": {
			sha256sum: "e15017382eec983563f3925268caf4ce62b738d90250372d4c3ee6b87ee57973",
			roVersion: "dartmonkey_v2.0.2887-311310808",
			rwVersion: "dartmonkey_v2.0.29922-e70d27b41",
			keyID:     "257a0aa3ac9e81aa4bc3aabdb6d3d079117c5799",
		},
	},
	fingerprint.BoardNameHelipilot: {
		"helipilot_v2.0.24337-2726e9f149-RO_v2.0.30479-3de9c9a0fe-RW.bin": {
			sha256sum: "077e94ce99281c01d2b40b458a6b193a08e6378d5f0c60f4c2eac55f21cada1d",
			roVersion: "helipilot_v2.0.24337-2726e9f149",
			rwVersion: "helipilot_v2.0.30479-3de9c9a0fe",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
		"helipilot_v2.0.27609-ac26a0796b-RO_v2.0.30479-3de9c9a0fe-RW.bin": {
			sha256sum: "4a186cbd1b48a065601dc1392c820af7324ce44820faf9d1029e47d051b2c4a7",
			roVersion: "helipilot_v2.0.27609-ac26a0796b",
			rwVersion: "helipilot_v2.0.30479-3de9c9a0fe",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
	},
	fingerprint.BoardNameNami: {
		"nami_fp_v2.2.144-7a08e07eb-RO_v2.0.29922-e70d27b412-RW.bin": {
			sha256sum: "dfee55e579446f666d6a05a59dc3f367848da1b404da457ec38381e8750d127e",
			roVersion: "nami_fp_v2.2.144-7a08e07eb",
			rwVersion: "nami_fp_v2.0.29922-e70d27b412",
			keyID:     "35486c0090ca390408f1fbbf2a182966084fe2f8",
		},
	},
	fingerprint.BoardNameNocturne: {
		"nocturne_fp_v2.2.64-58cf5974e-RO_v2.0.29922-e70d27b4-RW.bin": {
			sha256sum: "8baa6f8e7810bd3ae7f742f6f22db27642dff5430f839d74111f7fd170a51834",
			roVersion: "nocturne_fp_v2.2.64-58cf5974e",
			rwVersion: "nocturne_fp_v2.0.29922-e70d27b4",
			keyID:     "6f38c866182bd9bf7a4462c06ac04fa6a0074351",
		},
	},
}
