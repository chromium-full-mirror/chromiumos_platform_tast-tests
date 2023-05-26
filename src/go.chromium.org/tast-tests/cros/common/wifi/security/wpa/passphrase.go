// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wpa

import (
	"strings"

	"go.chromium.org/tast/core/errors"
)

// Valid passphrase for WPA protected network.
//  1. 8~63 characters, any character.
//  2. 64 characters, HEX character (0-9, a-f, A-F) only.

// GenPassphrase generates a valid passphrase with specified length for WPA protected network.
func GenPassphrase(length int) (string, error) {
	if length < 8 || length > 64 {
		return "", errors.Errorf("WPA requires passphrases to be between 8 and 64 characters but a length of %d was provided", length)
	}

	if length == 64 {
		return hexLetters(length), nil
	}
	return asciiLetters(length), nil
}

// GenInvalidPassphrase generates an invalid passphrase with specified length for WPA protected network.
func GenInvalidPassphrase(length int) (string, error) {
	if length >= 8 && length <= 63 {
		return "", errors.Errorf("WPA allows passphrases to be between 8 and 64 characters, so an invalid passphrase with %d characters does not exist", length)
	}

	if length == 64 {
		return hexLetters(63) + asciiLetters(1), nil
	}
	return asciiLetters(length), nil
}

func hexLetters(length int) string {
	const hex = "1234567890ABCDEFabcdef"
	s := strings.Repeat(hex, 1+length/len(hex))
	return s[:length]
}

func asciiLetters(length int) string {
	const ascii = "!@#$%^&*()_+qwertyuiop[]asdfghjkl;'zxcvbnm,./"
	s := strings.Repeat(ascii, 1+length/len(ascii))
	return s[:length]
}
