// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/metrics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const metricsReportingWaitingTime = 10 * time.Second

// RollbackPolicyActivatedMetric full name
const RollbackPolicyActivatedMetric = "cros::RollbackEnterprise::RollbackPolicyActivated"

// RollbackOobeConfigSaveMetric full name
const RollbackOobeConfigSaveMetric = "cros::RollbackEnterprise::RollbackOobeConfigSave"

// RollbackOobeConfigRestoreMetric full name
const RollbackOobeConfigRestoreMetric = "cros::RollbackEnterprise::RollbackOobeConfigRestore"

// RollbackMetricsFileWithPolicyActivatedEvent contains rollback metrics file data name.
const RollbackMetricsFileWithPolicyActivatedEvent = "rollback_smoke_metrics_file"

// PlaceRollbackMetricsFile creates the rollback metrics file with the data provided.
func PlaceRollbackMetricsFile(ctx context.Context, metricsFileDataPath string) error {
	data, err := os.ReadFile(metricsFileDataPath)
	if err != nil {
		return errors.Wrap(err, "failed to read file")
	}
	if err := os.WriteFile(MetricsData, data, 0664); err != nil {
		return errors.Wrap(err, "failed to create metrics file")
	}
	return nil
}

// PlaceCorruptedRollbackMetrics creates a corrupted rollback metrics file.
func PlaceCorruptedRollbackMetrics(ctx context.Context) error {
	if err := os.WriteFile(MetricsData, []byte(CorruptData), 0664); err != nil {
		return errors.Wrap(err, "failed to create corrupted metrics file")
	}
	return nil
}

// WaitForEventsTobeReported waits until all the rollback metrics events provided have been reported.
func WaitForEventsTobeReported(ctx context.Context, events []string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := CheckEventsHaveBeenReported(ctx, events); err != nil {
			return errors.Wrap(err, "failure when checking that all rollback metrics have been reported")
		}
		return nil
	}, &testing.PollOptions{Timeout: metricsReportingWaitingTime}); err != nil {
		return errors.Wrap(err, "failure while waiting for metrics reporting")
	}
	return nil
}

// CheckEventsHaveBeenReported checks all the rollback metrics events provided have been reported.
func CheckEventsHaveBeenReported(ctx context.Context, events []string) error {
	for _, event := range events {
		result, err := metrics.HasStructuredEventBeenReported(ctx, event)
		if err != nil {
			return errors.Wrap(err, "failed to check if the event has been reported")
		}
		if !result {
			return errors.Errorf("event %s has not been reported", event)
		}
	}
	return nil
}

// CheckEventsHaveNotBeenReported checks none of the rollback metrics events provided have been reported.
func CheckEventsHaveNotBeenReported(ctx context.Context, events []string) error {
	for _, event := range events {
		result, err := metrics.HasStructuredEventBeenReported(ctx, event)
		if err != nil {
			return errors.Wrap(err, "failed to check if the event has been reported")
		}
		if result {
			return errors.Errorf("event %s has been reported", event)
		}
	}
	return nil
}
