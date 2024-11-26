// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"os"
	"testing"

	"go.chromium.org/tast/core/errors"
)

func TestReadStringFileWithLeadingSpaces(t *testing.T) {
	readFile = func(string) ([]byte, error) { return []byte(" test  \n"), nil }
	if v, _ := ReadStringFileWithLeadingSpaces(""); v != " test" {
		t.Fatal("ReadStringFileWithLeadingSpaces failed to read file, got:", v)
	}
}

func TestReadStringFile(t *testing.T) {
	readFile = func(string) ([]byte, error) { return []byte(" test  \n"), nil }
	if v, _ := ReadStringFile(""); v != "test" {
		t.Fatal("ReadStringFile failed to read file, got:", v)
	}
}

func TestReadOptional(t *testing.T) {
	readFile = func(string) ([]byte, error) { return []byte("test\n"), nil }
	if v, _ := ReadOptionalStringFile(""); v == nil || *v != "test" {
		t.Fatal("ReadOptionalStringFile failed to read file, got:", v)
	}
	readFile = func(string) ([]byte, error) { return nil, errors.New("test") }
	if _, err := ReadOptionalStringFile(""); err == nil {
		t.Fatal("ReadOptionalStringFile should return error")
	}
	readFile = func(string) ([]byte, error) { return nil, errors.Wrap(os.ErrNotExist, "test") }
	if _, err := ReadOptionalStringFile(""); err != nil {
		t.Fatal("ReadOptionalStringFile should not return ErrNotExist")
	}
}
