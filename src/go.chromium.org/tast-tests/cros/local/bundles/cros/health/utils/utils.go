// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils provides util functions for health tast.
package utils

import (
	"os"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// For mocking
var readFile = os.ReadFile

// ReadStringFileWithLeadingSpaces reads a file and returns its content as
// string while keeping the leading spaces.
func ReadStringFileWithLeadingSpaces(fpath string) (string, error) {
	v, err := readFile(fpath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read file: %v", fpath)
	}
	return strings.TrimRight(string(v), " \n"), nil
}

// ReadStringFile reads a file and returns its content as string.
func ReadStringFile(fpath string) (string, error) {
	v, err := readFile(fpath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read file: %v", fpath)
	}
	return strings.Trim(string(v), " \n"), nil
}

// ReadOptionalStringFile returns nil if file not found.
func ReadOptionalStringFile(fpath string) (*string, error) {
	v, err := ReadStringFile(fpath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}
