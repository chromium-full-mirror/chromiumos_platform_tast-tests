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
		"bloonchipper_v2.0.4277-9f652bb3-RO_v2.0.29318-6543bef-RW.bin": {
			sha256sum: "0af7462a208a09d6a8eed484457cb0980597d0d4ea8e84442fdf71fe84762218",
			roVersion: "bloonchipper_v2.0.4277-9f652bb3",
			rwVersion: "bloonchipper-v2.0.29318-6543bef",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
		"bloonchipper_v2.0.5938-197506c1-RO_v2.0.29318-6543bef-RW.bin": {
			sha256sum: "b8e175fc1454f56684604af7132ab8a584a1acb013ec0b575c3752a0c6c5cebf",
			roVersion: "bloonchipper_v2.0.5938-197506c1",
			rwVersion: "bloonchipper-v2.0.29318-6543bef",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
	},
	fingerprint.BoardNameBuccaneer: {
		"buccaneer_v2.0.26328-821504380b-RO_v2.0.29519-02e4ab90d0-RW.bin": {
			sha256sum: "603e14c607662d388711731eda48c694c98483634db0fb3b11ae927241283fc9",
			roVersion: "buccaneer_v2.0.26328-821504380b",
			rwVersion: "buccaneer_v2.0.29519-02e4ab90d0",
			keyID:     "95fb0d0a5f1c1f658a0526430a3a184301421e32",
		},
	},
	fingerprint.BoardNameDartmonkey: {
		"dartmonkey_v2.0.2887-311310808-RO_v2.0.29303-8425edc75-RW.bin": {
			sha256sum: "0f708e78b10b9b5a5686a34aed0eb1e8c6dfb3695f6964ce0006c93aba025b0d",
			roVersion: "dartmonkey_v2.0.2887-311310808",
			rwVersion: "dartmonkey_v2.0.29303-8425edc75",
			keyID:     "257a0aa3ac9e81aa4bc3aabdb6d3d079117c5799",
		},
	},
	fingerprint.BoardNameHelipilot: {
		"helipilot_v2.0.24337-2726e9f149-RO_v2.0.29519-02e4ab90d0-RW.bin": {
			sha256sum: "2a1367953558a01b0347c40db7c6dfb1d15dd3c8e69a01a877660d6b53573e18",
			roVersion: "helipilot_v2.0.24337-2726e9f149",
			rwVersion: "helipilot_v2.0.29519-02e4ab90d0",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
		"helipilot_v2.0.27609-ac26a0796b-RO_v2.0.29322-b77e8e8a26-RW.bin": {
			sha256sum: "1577ac24244265c2bdaaabcfe201746662529d8bd6b22c894c836d2da928b685",
			roVersion: "helipilot_v2.0.27609-ac26a0796b",
			rwVersion: "helipilot_v2.0.29322-b77e8e8a26",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
	},
	fingerprint.BoardNameNami: {
		"nami_fp_v2.2.144-7a08e07eb-RO_v2.0.29303-8425edc75f-RW.bin": {
			sha256sum: "f270245e69429ef8e0908a0e1cdb4ba08a4307950e10b5662f4560b1a6292880",
			roVersion: "nami_fp_v2.2.144-7a08e07eb",
			rwVersion: "nami_fp_v2.0.29303-8425edc75f",
			keyID:     "35486c0090ca390408f1fbbf2a182966084fe2f8",
		},
	},
	fingerprint.BoardNameNocturne: {
		"nocturne_fp_v2.2.64-58cf5974e-RO_v2.0.29303-8425edc7-RW.bin": {
			sha256sum: "a00e75fc3c73bf7d49949d9bfacdba9eaac5308981df47a5dbf34170f4eaab06",
			roVersion: "nocturne_fp_v2.2.64-58cf5974e",
			rwVersion: "nocturne_fp_v2.0.29303-8425edc7",
			keyID:     "6f38c866182bd9bf7a4462c06ac04fa6a0074351",
		},
	},
}
