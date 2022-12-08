// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"encoding/json"
)

// TestStruct is a structure used for testing Tast's handling of fixture value of
// struct time. Please don't use it outside meta package.
type TestStruct struct {
	TestNum    int
	TestString string
}

const (
	// RemoteFixtureExpectedStringVal is the expected string value for tests verifying
	// Tast's support on local tests accessing remote fixture value.
	RemoteFixtureExpectedStringVal = "String Value Example"
)

var (
	// RemoteFixtureExpectedStructVal is the expected struct value for tests verifying
	// Tast's support on local tests accessing remote fixture value.
	RemoteFixtureExpectedStructVal = &TestStruct{TestNum: 99, TestString: "Any"}
)

// DeserializedTestStructVal deserializes data with TestStruct type.
func DeserializedTestStructVal(data []byte) (*TestStruct, error) {
	result := TestStruct{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeserializedTestStringVal deserializes data with string type.
func DeserializedTestStringVal(data []byte) (string, error) {
	result := ""
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	return result, nil
}
