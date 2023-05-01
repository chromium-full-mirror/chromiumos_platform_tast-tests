// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"

	"chromiumos/tast/common/utils"

	"go.chromium.org/tast/core/errors"
)

// QualRun holds the power qual run information.
type QualRun struct {
	Config *Config
	Tests  []string
}

// NewQualRun returns a new QualRun from a test configuration URL.
func NewQualRun(ctx context.Context, url string) (*QualRun, error) {
	configJSON, err := utils.FetchFromURL(ctx, url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch configuration from %s", url)
	}

	config := &Config{}
	if err := json.Unmarshal([]byte(configJSON), config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal configuration")
	}

	tests, err := validateConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate configuration")
	}

	return &QualRun{Config: config, Tests: tests}, nil
}

// GenerateReport generates the power qual run test report.
func (r *QualRun) GenerateReport(ctx context.Context, skippedTests []string, inputDir, outputDir string) error {
	// TODO(b/274972858): generate combined test report.
	return nil
}
