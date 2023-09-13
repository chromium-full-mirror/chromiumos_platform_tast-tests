// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package factory

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	factoryFaiPath = "/usr/local/sbin/factory_fai"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     FAI,
		Desc:     "Test Factory First Article Inspection is able to collect all expected data",
		Contacts: []string{"chromeos-factory-fai@google.com", "wyuang@google.com"},
		// ChromeOS > Platform > Enablement > Factory
		BugComponent: "b:167224",
		SoftwareDeps: []string{"factory_flow"},
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      3 * time.Minute,
	})
}

func getWaivedItems() []string {
	return []string{
		// Statful partition is unable to mount in test image.
		"release_image_stateful_partition",
		// AP RO hash is not stored in Ti50.
		"gsc_ap_ro_hash",
	}
}

func prepareFaiConfig(ctx context.Context) (string, error) {
	tempFile, err := os.CreateTemp("", "")
	if err != nil {
		return "", errors.Wrap(err, "failed to create config file")
	}
	defer tempFile.Close()

	rawConfig, err := testexec.CommandContext(ctx, factoryFaiPath, "--dump-config").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to dump fai config")
	}

	var jsonConfig map[string]interface{}
	json.Unmarshal(rawConfig, &jsonConfig)

	for _, key := range getWaivedItems() {
		delete(jsonConfig, key)
	}

	modifiedConfig, err := json.Marshal(jsonConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to modify fai config")
	}

	if _, err := tempFile.Write(modifiedConfig); err != nil {
		return "", errors.Wrap(err, "failed to wrtie fai config")
	}

	return tempFile.Name(), nil
}

func FAI(ctx context.Context, s *testing.State) {
	configPath, err := prepareFaiConfig(ctx)
	if err != nil {
		s.Fatal("Failed to prepare fai config: ", err)
	}
	defer os.Remove(configPath)
	output, err := testexec.CommandContext(ctx, factoryFaiPath, "-c", configPath).Output()
	if err != nil {
		s.Fatal("Failed to execute factory_fai: ", err)
	}
	var jsonResult map[string]interface{}
	json.Unmarshal(output, &jsonResult)

	for key, value := range jsonResult {
		switch t := value.(type) {
		case string:
			if len(t) == 0 {
				s.Errorf("%s is not collected", key)
			}
		case map[string]interface{}:
			if len(t) == 0 {
				s.Errorf("%s is not collected", key)
			}
		case []interface{}:
			if len(t) == 0 {
				s.Errorf("%s is not collected", key)
			}
		default:
			s.Errorf("Unexpected data type: %s=%s", key, value)
		}
	}
}
