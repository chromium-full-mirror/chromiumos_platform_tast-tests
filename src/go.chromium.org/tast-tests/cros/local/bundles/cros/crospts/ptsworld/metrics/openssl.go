// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const opensslTitle = "OpenSSL"

// opensslCrosboltNameTable maps the PTS description to crosbolt name.
var opensslCrosboltNameTable = map[string]string{
	"Algorithm: SHA256":            "SHA256",
	"Algorithm: AES-256-GCM":       "AES-256-GCM",
	"Algorithm: ChaCha20-Poly1305": "ChaCha20-Poly1305",
}
