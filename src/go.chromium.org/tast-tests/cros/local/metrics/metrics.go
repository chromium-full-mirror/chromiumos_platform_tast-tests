// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
)

const (
	metricsClientPath = "/usr/bin/metrics_client"
	// metrics_client "-c: return exit status 0 if user consents to stats,""
	// "1 otherwise, in guest mode always return 1"
	metricsClientConsentStatusArg = "-c"
	// userConsents is the exit status if the user consents to stats.
	userConsents = 0
	// userDoesNotConsent is the exit status if the user does not consent to stats.
	userDoesNotConsent = 1
	// Create consent file such that -c will return 0.
	metricsClientCreateConsentArg = "-C"
	// Delete consent file such that -c will return 1.
	metricsClientDeleteConsentArg = "-D"
)

// HasConsent checks if the system has metrics consent.
func HasConsent(ctx context.Context) (bool, error) {
	// The C++ code reads the (possibly multiple) device policy files, and then
	// falls back to previous files if the latest file isn't valid, and then falls
	// back to enterprise enrollments and legacy consent files.
	// Rather than try to reproduce all that in Go, call a C++ program that runs
	// the exact same code that crash_reporter & crash_sender does.
	err := testexec.CommandContext(ctx, metricsClientPath, metricsClientConsentStatusArg).Run()
	code, ok := testexec.ExitCode(err)
	if !ok {
		return false, errors.Wrapf(err, "could not exec %s %s", metricsClientPath, metricsClientConsentStatusArg)
	}
	switch code {
	case userConsents:
		return true, nil
	case userDoesNotConsent:
		return false, nil
	default:
		return false, errors.Errorf("unexpected exit code from %s %s: %d", metricsClientPath, metricsClientConsentStatusArg, code)
	}
}

// CreateConsent ensures the system has metrics consent.
func CreateConsent(ctx context.Context) error {
	err := testexec.CommandContext(ctx, metricsClientPath, metricsClientCreateConsentArg).Run()
	_, ok := testexec.ExitCode(err)
	if !ok {
		return errors.Wrapf(err, "could not exec %s %s", metricsClientPath, metricsClientCreateConsentArg)
	}

	return nil
}

// DeleteConsent removes metrics consent from the system.
func DeleteConsent(ctx context.Context) error {
	err := testexec.CommandContext(ctx, metricsClientPath, metricsClientDeleteConsentArg).Run()
	_, ok := testexec.ExitCode(err)
	if !ok {
		return errors.Wrapf(err, "could not exec %s %s", metricsClientPath, metricsClientDeleteConsentArg)
	}

	return nil
}
