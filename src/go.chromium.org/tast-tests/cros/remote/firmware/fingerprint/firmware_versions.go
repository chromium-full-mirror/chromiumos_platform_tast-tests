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
		"bloonchipper_v2.0.4277-9f652bb3-RO_v2.0.30778-e07b67c-RW.bin": {
			sha256sum: "2e7379284452c0b33a42b02d81649b23ed3668e4359eb5f4f8236ee90a106586",
			roVersion: "bloonchipper_v2.0.4277-9f652bb3",
			rwVersion: "bloonchipper-v2.0.30778-e07b67c",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
		"bloonchipper_v2.0.5938-197506c1-RO_v2.0.30778-e07b67c-RW.bin": {
			sha256sum: "9006646051f09ec76f8433acb55907d2e058e30bdef80382a9f7b0ab0dcbb40d",
			roVersion: "bloonchipper_v2.0.5938-197506c1",
			rwVersion: "bloonchipper-v2.0.30778-e07b67c",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
	},
	fingerprint.BoardNameBuccaneer: {
		"buccaneer_v2.0.26328-821504380b-RO_v2.0.30480-e5a8f3e0c7-RW.bin": {
			sha256sum: "bece0f72e0a56ebdcd6b4722489edd501b3221ad1f9fec6d1a866738243f1d44",
			roVersion: "buccaneer_v2.0.26328-821504380b",
			rwVersion: "buccaneer_v2.0.30480-e5a8f3e0c7",
			keyID:     "95fb0d0a5f1c1f658a0526430a3a184301421e32",
		},
	},
	fingerprint.BoardNameDartmonkey: {
		"dartmonkey_v2.0.2887-311310808-RO_v2.0.29923-1ca568934-RW.bin": {
			sha256sum: "8f2e9b9805b4ad58042bd557380cdc7f1c06df3114647cd5d9ea40e8d59c3cf7",
			roVersion: "dartmonkey_v2.0.2887-311310808",
			rwVersion: "dartmonkey_v2.0.29923-1ca568934",
			keyID:     "257a0aa3ac9e81aa4bc3aabdb6d3d079117c5799",
		},
	},
	fingerprint.BoardNameHelipilot: {
		"helipilot_v2.0.24337-2726e9f149-RO_v2.0.30480-e5a8f3e0c7-RW.bin": {
			sha256sum: "61b8bed96c390872f9b2bcd2d1ab9e437e3f21db81e06cd07c8adde2e450c317",
			roVersion: "helipilot_v2.0.24337-2726e9f149",
			rwVersion: "helipilot_v2.0.30480-e5a8f3e0c7",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
		"helipilot_v2.0.27609-ac26a0796b-RO_v2.0.30480-e5a8f3e0c7-RW.bin": {
			sha256sum: "25b1f8d604b75fe0217f6a110862f68c499e039eb2baf2c0e08120d5d636db42",
			roVersion: "helipilot_v2.0.27609-ac26a0796b",
			rwVersion: "helipilot_v2.0.30480-e5a8f3e0c7",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
	},
	fingerprint.BoardNameNami: {
		"nami_fp_v2.2.144-7a08e07eb-RO_v2.0.29923-1ca5689343-RW.bin": {
			sha256sum: "82d2aa808e5b1318f83c96646cae06da8fedae6aea8d4c61eb3c72dbe3abcd00",
			roVersion: "nami_fp_v2.2.144-7a08e07eb",
			rwVersion: "nami_fp_v2.0.29923-1ca5689343",
			keyID:     "35486c0090ca390408f1fbbf2a182966084fe2f8",
		},
	},
	fingerprint.BoardNameNocturne: {
		"nocturne_fp_v2.2.64-58cf5974e-RO_v2.0.29923-1ca56893-RW.bin": {
			sha256sum: "50e4d98681474e1f4b529e05d2d6e9e9e8d7f828e42cd00d9889b83d8bd9458e",
			roVersion: "nocturne_fp_v2.2.64-58cf5974e",
			rwVersion: "nocturne_fp_v2.0.29923-1ca56893",
			keyID:     "6f38c866182bd9bf7a4462c06ac04fa6a0074351",
		},
	},
}
