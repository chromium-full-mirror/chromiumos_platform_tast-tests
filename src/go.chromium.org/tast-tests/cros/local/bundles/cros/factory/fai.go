// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package factory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	factoryInstallerPath = "/usr/local/sbin/factory_installer"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     FAI,
		Desc:     "Test if Factory First Article Inspection (FAI) is able to collect all expected components",
		Contacts: []string{"chromeos-factory-fai@google.com", "wyuang@google.com"},
		// ChromeOS > Platform > Enablement > Factory
		BugComponent: "b:167224",
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"factory_flow", "gsc"},
		Attr:         []string{"group:mainline"},
		Timeout:      3 * time.Minute,
	})
}

// getKnownFailedItems returns the elements that factory_fai might fail to collect.
func getKnownFailedItems() []string {
	return []string{
		// Statful partition is unable to mount in test image.
		"release_image_stateful_partition",
		// These info stored in release rootfs partition which varies in
		// the lab devices.
		"release_image_info",
		"signing_keys",
	}
}

// getKnownEmptyItems returns the elements that are expected to be empty on some projects.
func getKnownEmptyItems() []string {
	return []string{
		"cbi_data",
	}
}

func prepareFaiConfig(ctx context.Context) (string, error) {
	tempFile, err := os.CreateTemp("", "")
	if err != nil {
		return "", errors.Wrap(err, "failed to create config file")
	}
	defer tempFile.Close()

	rawConfig, err := testexec.CommandContext(ctx, factoryInstallerPath, "fai", "--dump-config").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to dump fai config")
	}

	var jsonConfig map[string]interface{}
	json.Unmarshal(rawConfig, &jsonConfig)

	// Removes the fields that factory_fai might fail to collect.
	// Plus, we don't care about these values on lab devices.
	for _, key := range getKnownFailedItems() {
		delete(jsonConfig, key)
	}

	modifiedConfig, err := json.Marshal(jsonConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to modify fai config")
	}

	if _, err := tempFile.Write(modifiedConfig); err != nil {
		return "", errors.Wrap(err, "failed to write fai config")
	}

	return tempFile.Name(), nil
}

func FAI(ctx context.Context, s *testing.State) {
	configPath, err := prepareFaiConfig(ctx)
	if err != nil {
		s.Fatal("Failed to prepare fai config: ", err)
	}
	defer os.Remove(configPath)

	// File to store the FAI result.
	faiResultFile := filepath.Join(s.OutDir(), "factory_fai_result.json")

	_, stderr, err := testexec.CommandContext(ctx, factoryInstallerPath, "fai", "-c", configPath, "--output-path", faiResultFile).SeparatedOutput()
	if len(stderr) > 0 {
		s.Log("factory_installer stderr:", string(stderr))
	}
	if err != nil {
		s.Fatal("Failed to execute factory_installer: ", err)
	}

	re := regexp.MustCompile(`Error: Failed to collect "(?P<component>\w+)".`)
	matches := re.FindAllStringSubmatch(string(stderr), -1)
	if matches != nil {
		var components []string
		for _, match := range matches {
			components = append(components, match[1])
		}
		s.Error("Failed to collect the components: ", components)
	}

	faiResult, err := os.ReadFile(faiResultFile)
	if err != nil {
		s.Fatal("Failed to read the FAI result file: ", err)
	}
	var jsonResult map[string]interface{}
	json.Unmarshal(faiResult, &jsonResult)

	if len(jsonResult) == 0 {
		s.Fatal("FAI result is empty")
	}

	// Removes the items that are known to be empty on some projects.
	for _, key := range getKnownEmptyItems() {
		delete(jsonResult, key)
	}

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
