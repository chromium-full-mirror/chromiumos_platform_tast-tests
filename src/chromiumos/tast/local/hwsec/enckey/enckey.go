// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package enckey provides functions and constants for encryption key injection.
package enckey

import (
	"context"
	"os"

	"chromiumos/tast/common/hwsec"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

const googleKeysDataPath = "/run/attestation/google_keys.data"

// InjectWellKnownGoogleKeys creates the well-known Google keys file and restarts attestation service.
func InjectWellKnownGoogleKeys(ctx context.Context) (lastErr error) {
	if _, err := os.Stat(googleKeysDataPath); os.IsNotExist(err) {
		if err := testexec.CommandContext(ctx, "attestation-injected-keys").Run(); err != nil {
			return errors.Wrap(err, "failed to create key file")
		}
	}
	return nil
}

// InjectWellKnownGoogleKeysAndRestart creates the well-known Google keys file and restarts attestation service.
func InjectWellKnownGoogleKeysAndRestart(ctx context.Context, dc *hwsec.DaemonController) (lastErr error) {
	if _, err := os.Stat(googleKeysDataPath); os.IsNotExist(err) {
		if err := testexec.CommandContext(ctx, "attestation-injected-keys").Run(); err != nil {
			return errors.Wrap(err, "failed to create key file")
		}
	}
	defer func() {
		if lastErr != nil {
			if err := os.Remove(googleKeysDataPath); err != nil {
				testing.ContextLog(ctx, "Failed to remove the injected key database: ", err)
			}
		}
	}()
	if err := dc.Restart(ctx, hwsec.AttestationDaemon); err != nil {
		return errors.Wrap(err, "failed to restart attestation")
	}
	return nil
}

// InjectNormalGoogleKeys deletes the well-known Google keys file.
func InjectNormalGoogleKeys(ctx context.Context) error {
	if err := os.Remove(googleKeysDataPath); err != nil {
		return errors.Wrap(err, "failed to remove injected key file")
	}
	return nil
}

// InjectNormalGoogleKeysAndRestart deletes the well-known Google keys file and restarts attestation service.
func InjectNormalGoogleKeysAndRestart(ctx context.Context, dc *hwsec.DaemonController) error {
	if err := os.Remove(googleKeysDataPath); err != nil {
		return errors.Wrap(err, "failed to remove injected key file")
	}
	if err := dc.Restart(ctx, hwsec.AttestationDaemon); err != nil {
		return errors.Wrap(err, "failed to restart attestation")
	}
	return nil
}
