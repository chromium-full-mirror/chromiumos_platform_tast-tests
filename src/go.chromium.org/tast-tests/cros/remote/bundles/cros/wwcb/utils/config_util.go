// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"encoding/json"
	"os"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	configFileName = "Capabilities.json"
)

// ReadConfigFile is for reading the config file.
func ReadConfigFile(s *testing.State) (map[string]interface{}, error) {
	jsonFile, err := os.ReadFile(s.DataPath(configFileName))
	if err != nil {
		return nil, errors.Wrap(err, "read file")
	}

	var data map[string]interface{}
	err = json.Unmarshal(jsonFile, &data)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal json")
	}
	return data, nil
}

// WriteConfigFile is for writing to the config file.
func WriteConfigFile(s *testing.State, mapToWrite map[string]interface{}) error {
	jsonByte, err := json.MarshalIndent(mapToWrite, "", "    ")
	if err != nil {
		return errors.Wrap(err, "marshal json")
	}

	err = os.WriteFile(s.DataPath(configFileName), jsonByte, 0644)
	if err != nil {
		return errors.Wrap(err, "write file")
	}
	return nil
}
