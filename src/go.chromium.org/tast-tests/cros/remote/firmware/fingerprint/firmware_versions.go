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
		"bloonchipper_v2.0.4277-9f652bb3-RO_v2.0.28117-ef558fa-RW.bin": {
			sha256sum: "5c6d6755551836a002ee171cfe7c8b26c80e69cc059369dd9f9f23787bc4e422",
			roVersion: "bloonchipper_v2.0.4277-9f652bb3",
			rwVersion: "bloonchipper-v2.0.28117-ef558fa",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
		"bloonchipper_v2.0.5938-197506c1-RO_v2.0.28117-ef558fa-RW.bin": {
			sha256sum: "55211a49411ea7b8bdb4615168f7e0d800d451aa18280cac5a6d7d898dae3980",
			roVersion: "bloonchipper_v2.0.5938-197506c1",
			rwVersion: "bloonchipper-v2.0.28117-ef558fa",
			keyID:     "1c590ef36399f6a2b2ef87079c135b69ef89eb60",
		},
	},
	fingerprint.BoardNameBuccaneer: {
		"buccaneer_v2.0.26328-821504380b-RO_v2.0.28107-258bcf04b5-RW.bin": {
			sha256sum: "cd7af69fbfb7b1a14ac592abbba02f4985a49117a1260f4c498b25903a603109",
			roVersion: "buccaneer_v2.0.26328-821504380b",
			rwVersion: "buccaneer_v2.0.28107-258bcf04b5",
			keyID:     "95fb0d0a5f1c1f658a0526430a3a184301421e32",
		},
	},
	fingerprint.BoardNameDartmonkey: {
		"dartmonkey_v2.0.2887-311310808-RO_v2.0.28102-64c3d5971-RW.bin": {
			sha256sum: "56c1c4cfca67342e27579b7a445bbd01fe493af043f5c8fb94d41f5603ed16a8",
			roVersion: "dartmonkey_v2.0.2887-311310808",
			rwVersion: "dartmonkey_v2.0.28102-64c3d5971",
			keyID:     "257a0aa3ac9e81aa4bc3aabdb6d3d079117c5799",
		},
	},
	fingerprint.BoardNameGwendolin: {
		"gwendolin_v2.0.27079-f272542298.bin": {
			sha256sum: "96b168f60fb7d601e394280ff1c18b28c002c4a61407e92a3feaf44e1bf07cf2",
			roVersion: "gwendolin_v2.0.27079-f272542298",
			rwVersion: "gwendolin_v2.0.27079-f272542298",
			keyID:     "1a6cb4aaf9e68488b4e92a36bfd532d0774dffd1",
		},
	},
	fingerprint.BoardNameHelipilot: {
		"helipilot_v2.0.24337-2726e9f149-RO_v2.0.28107-258bcf04b5-RW.bin": {
			sha256sum: "546a30d1ec4b4ec373185ac8402ae215c866f977a87a35bcda78dbae9a2126a2",
			roVersion: "helipilot_v2.0.24337-2726e9f149",
			rwVersion: "helipilot_v2.0.28107-258bcf04b5",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
		"helipilot_v2.0.27609-ac26a0796b-RO_v2.0.28107-258bcf04b5-RW.bin": {
			sha256sum: "9584dd432d27329029f6a0153b74cc65f5b39a2cc65eb5322ea3a979b15593ae",
			roVersion: "helipilot_v2.0.27609-ac26a0796b",
			rwVersion: "helipilot_v2.0.28107-258bcf04b5",
			keyID:     "3c0b147809e06f279ba0cf221c18995d7b4e3f1a",
		},
	},
	fingerprint.BoardNameNami: {
		"nami_fp_v2.2.144-7a08e07eb-RO_v2.0.28102-64c3d5971e-RW.bin": {
			sha256sum: "117c462b2c220506897b587451edcce6ac8c462457ea482454fac6b1d6902075",
			roVersion: "nami_fp_v2.2.144-7a08e07eb",
			rwVersion: "nami_fp_v2.0.28102-64c3d5971e",
			keyID:     "35486c0090ca390408f1fbbf2a182966084fe2f8",
		},
	},
	fingerprint.BoardNameNocturne: {
		"nocturne_fp_v2.2.64-58cf5974e-RO_v2.0.28102-64c3d597-RW.bin": {
			sha256sum: "215422e8f9469f7a526d22a32fdd6703ace62eadc5fddc9144df12d7c3720b7a",
			roVersion: "nocturne_fp_v2.2.64-58cf5974e",
			rwVersion: "nocturne_fp_v2.0.28102-64c3d597",
			keyID:     "6f38c866182bd9bf7a4462c06ac04fa6a0074351",
		},
	},
}
