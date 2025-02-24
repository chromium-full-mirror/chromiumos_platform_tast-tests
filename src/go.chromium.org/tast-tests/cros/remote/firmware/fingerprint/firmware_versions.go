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
		"bloonchipper_v2.0.4277-9f652bb3-RO_v2.0.25973-4e2e543-RW.bin": {
			sha256sum: "90f7b57cf38bdda0ee41d82eec7c7ae8ad034e43342b381b07dcc0390c8e7b62",
			roVersion: "bloonchipper_v2.0.4277-9f652bb3",
			rwVersion: "bloonchipper-v2.0.25973-4e2e543",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
		"bloonchipper_v2.0.5938-197506c1-RO_v2.0.25973-4e2e543-RW.bin": {
			sha256sum: "be0c31f66242bb82366250606da5cbe9da332728515b13d43ad4b58c188f3bcb",
			roVersion: "bloonchipper_v2.0.5938-197506c1",
			rwVersion: "bloonchipper-v2.0.25973-4e2e543",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
	},
	fingerprint.BoardNameBuccaneer: {
		"buccaneer_v2.0.26328-821504380b-RO_v2.0.26330-4778869a66-RW.bin": {
			sha256sum: "834f1140ffef8a02dbb3d7cd87c613dc5c2c45a66c6c05c78f68f78282c61cc9",
			roVersion: "buccaneer_v2.0.26328-821504380b",
			rwVersion: "buccaneer_v2.0.26330-4778869a66",
			keyID:     "95fb0d0a5f1c1f658a0526430a3a184301421e32",
		},
		"buccaneer_v2.0.26328-821504380b-RO_v2.0.26878-c9a5670643-RW.bin": {
			sha256sum: "f3f10bf720a17e3b238cd481cf491728ac02c5f548c8b4dd7c22ec4b99d85ff4",
			roVersion: "buccaneer_v2.0.26328-821504380b",
			rwVersion: "buccaneer_v2.0.26878-c9a5670643",
			keyID:     "95fb0d0a5f1c1f658a0526430a3a184301421e32",
		},
		"buccaneer_v2.0.26328-821504380b-RO_v2.0.27609-ac26a0796b-RW.bin": {
			sha256sum: "d06ea92e7f974381c3df00748526399bcef30c8446e27dd6f887bc7e1aaef157",
			roVersion: "buccaneer_v2.0.26328-821504380b",
			rwVersion: "buccaneer_v2.0.27609-ac26a0796b",
			keyID:     "95fb0d0a5f1c1f658a0526430a3a184301421e32",
		},
	},
	fingerprint.BoardNameDartmonkey: {
		"dartmonkey_v2.0.2887-311310808-RO_v2.0.25642-c284c7bb8-RW.bin": {
			sha256sum: "aecb0283137268c1e20563a1425fc0b8d375b30de3cdfb87faabb39a49eb1d56",
			roVersion: "dartmonkey_v2.0.2887-311310808",
			rwVersion: "dartmonkey_v2.0.25642-c284c7bb8",
			keyID:     "257a0aa3ac9e81aa4bc3aabdb6d3d079117c5799",
		},
	},
	fingerprint.BoardNameHelipilot: {
		"helipilot_v2.0.24337-2726e9f149-RO_v2.0.26331-e461368cca-RW.bin": {
			sha256sum: "910b77b1aa8c504166b154f633298073cc497b95b7e1dc33f51de3cfec0062b7",
			roVersion: "helipilot_v2.0.24337-2726e9f149",
			rwVersion: "helipilot_v2.0.26331-e461368cca",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
		"helipilot_v2.0.24337-2726e9f149-RO_v2.0.26878-c9a5670643-RW.bin": {
			sha256sum: "777dbf0f6525dccf60c278156e94c88c10e4e154f7b2b846afae3c08e3bfc1d1",
			roVersion: "helipilot_v2.0.24337-2726e9f149",
			rwVersion: "helipilot_v2.0.26878-c9a5670643",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
	},
	fingerprint.BoardNameNami: {
		"nami_fp_v2.2.144-7a08e07eb-RO_v2.0.25642-c284c7bb87-RW.bin": {
			sha256sum: "5d7f753819947692f91f3b5f6882ab7b286311aff5814fd5f78f38b3a7548d8c",
			roVersion: "nami_fp_v2.2.144-7a08e07eb",
			rwVersion: "nami_fp_v2.0.25642-c284c7bb87",
			keyID:     "35486c0090ca390408f1fbbf2a182966084fe2f8",
		},
	},
	fingerprint.BoardNameNocturne: {
		"nocturne_fp_v2.2.64-58cf5974e-RO_v2.0.25642-c284c7bb-RW.bin": {
			sha256sum: "1c39fb1a65e4e6d7d649a3242933f867b802698ab8e51ab7fb74016c1c03c958",
			roVersion: "nocturne_fp_v2.2.64-58cf5974e",
			rwVersion: "nocturne_fp_v2.0.25642-c284c7bb",
			keyID:     "6f38c866182bd9bf7a4462c06ac04fa6a0074351",
		},
	},
}
