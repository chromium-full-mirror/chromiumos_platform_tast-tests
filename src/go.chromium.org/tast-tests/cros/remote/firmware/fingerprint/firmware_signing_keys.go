// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

// Map from signing key ID to type of signing key.
var keyIDMap = map[string]KeyType{
	// bloonchipper.
	"61382804da86b4156d666cc9a976088f8b647d44": KeyTypeDev,
	"07b1af57220c196e363e68d73a5966047c77011e": KeyTypePreMp,
	"1c590ef36399f6a2b2ef87079c135b69ef89eb60": KeyTypeMp,

	// buccaneer.
	"95fb0d0a5f1c1f658a0526430a3a184301421e32": KeyTypeMp,

	// dartmonkey.
	"257a0aa3ac9e81aa4bc3aabdb6d3d079117c5799": KeyTypeMp,

	// nocturne.
	"8a8fc039a9463271995392f079b83ce33832d07d": KeyTypeDev,
	"6f38c866182bd9bf7a4462c06ac04fa6a0074351": KeyTypeMp,
	"f6f7d96c48bd154dbae7e3fe3a3b4c6268a10934": KeyTypePreMp,

	// nami.
	"754aea623d69975a22998f7b97315dd53115d723": KeyTypePreMp,
	"35486c0090ca390408f1fbbf2a182966084fe2f8": KeyTypeMp,

	// helipilot.
	"ff60ba1fe2cf13f60d0debfb350f7c321115e59a": KeyTypePreMp,
	"3c0b147809e06f279ba0cf221c18995d7b4e3f1a": KeyTypeMp,
}
