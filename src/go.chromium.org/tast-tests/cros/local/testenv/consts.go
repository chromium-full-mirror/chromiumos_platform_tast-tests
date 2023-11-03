// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

// Environment identifiers
const (
	Prod    = "prod"
	Preprod = "preprod"
	Fake    = "fake"
)

// ValidEnvs is a list of the environments used for the end-to-end testing
var ValidEnvs = []string{Prod, Preprod}
