// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

// This file contains some shared constants for local hwsec bundle tests.
const (
	// Example user information
	FirstUsername = "fakeuser1@example.com"
	FirstPassword = "FakePasswordForFakeUser1"
	FirstPin      = "123456"

	// Example user information
	SecondUsername = "fakeuser2@example.com"
	SecondPassword = "FakePasswordForFakeUser2"
	SecondPin      = "654321"

	// Example user information
	ThirdUsername = "fakeuser3@example.com"
	ThirdPassword = "FakePasswordForFakeUser3"
	ThirdPin      = "987654"

	// Example key label
	PasswordLabel = "PasswordLabel"
	PinLabel      = "PinLabel"

	// Example test file
	TestFileName1   = "TESTFILE1"
	TestFileName2   = "TESTFILE2"
	TestFileContent = "TEST_CONTENT"

	// Install attributes x-ver test constants
	NvramTpm2Index   = "0x800004" // Note: currently only TPM2.0 is supported.
	LockboxNvramPath = "/tmp/lockbox.nvram"
)

// InstallAttrsContents list of install-attributes contents to use in x-ver test.
var InstallAttrsContents = map[string]string{
	"Ibuprofen":            "C13H18O2",
	"Acetaminophen":        "C8H9NO2",
	"Acetylsalicylic Acid": "C9H8O4",
}
