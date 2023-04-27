// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	"chromiumos/tast/common/hwsec"
	"chromiumos/tast/errors"
	"chromiumos/tast/fsutil"
	"chromiumos/tast/local/filesnapshot"
	"chromiumos/tast/local/hwsec/enckey"
	"chromiumos/tast/testing"
	"go.chromium.org/tast/core/ctxutil"
)

// AttestationLocalInfra enables/disables the local server implementation on DUT.
type AttestationLocalInfra struct {
	dc        *hwsec.DaemonController
	fpca      *FakePCAAgent
	snapshot  *filesnapshot.Snapshot
	dbStashed bool
}

// NewAttestationLocalInfra creates a new AttestationLocalInfra instance, with dc used to control the D-Bus service daemons.
func NewAttestationLocalInfra(dc *hwsec.DaemonController) *AttestationLocalInfra {
	return &AttestationLocalInfra{dc, nil, filesnapshot.NewSnapshot(), false}
}

// Enable enables the local test infra for attestation flow testing.
func (ali *AttestationLocalInfra) Enable(ctx context.Context) (lastErr error) {
	// Restore the backed up attestation database.
	if err := fsutil.CopyFile(attestationDBBackupPath, hwsec.AttestationDBPath); err != nil {
		return errors.Wrap(err, "failed to restore the fake attestation database")
	}

	if err := enckey.InjectWellKnownGoogleKeysAndRestart(ctx, ali.dc); err != nil {
		return errors.Wrap(err, "failed to inject well-known keys")
	}

	cleanupCtx := ctx
	// We must not cancel the context here as it is being used by the caller.
	ctx, _ = ctxutil.Shorten(ctx, 5*time.Second)

	// Revert the key injection if other parts of this function fails.
	defer func(ctx context.Context) {
		if lastErr != nil {
			if err := enckey.InjectNormalGoogleKeys(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to inject the normal key back: ", err)
			}
		}
	}(cleanupCtx)

	if err := ali.enableFakePCAAgent(ctx); err != nil {
		return errors.Wrap(err, "failed to enable fake pca agent")
	}
	return nil
}

// Disable disables the local test infra for attestation flow testing.
func (ali *AttestationLocalInfra) Disable(ctx context.Context) error {
	var lastErr error
	if ali.dbStashed {
		if err := ali.snapshot.Pop(hwsec.AttestationDBPath); err != nil {
			testing.ContextLog(ctx, "Failed to pop the snapshot of attestation database back: ", err)
			lastErr = errors.Wrap(err, "failed to pop the snapshot of attestation database back")
		}
	}
	if err := enckey.InjectNormalGoogleKeys(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to inject the normal key back: ", err)
		lastErr = errors.Wrap(err, "failed to inject the normal key back")
	}
	if err := ali.disableFakePCAAgent(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to disable fake pca agent: ", err)
		lastErr = errors.Wrap(err, "failed to disable fake pca agent")
	}
	return lastErr
}

// enableFakePCAAgent stops the normal pca agent and starts the fake one.
func (ali *AttestationLocalInfra) enableFakePCAAgent(ctx context.Context) (lastErr error) {
	if err := ali.dc.Stop(ctx, hwsec.PCAAgentDaemon); err != nil {
		return errors.Wrap(err, "failed to stop normal pca agent")
	}
	defer func() {
		if lastErr != nil {
			if err := ali.dc.Start(ctx, hwsec.PCAAgentDaemon); err != nil {
				testing.ContextLog(ctx, "Failed to stop start normal pca agent: ", err)
			}
		}
	}()
	if ali.fpca == nil {
		ali.fpca = FakePCAAgentContext(ctx)
		if err := ali.fpca.Start(); err != nil {
			return errors.Wrap(err, "failed to start fake pca agent")
		}
	}
	return nil
}

// disableFakePCAAgent stops the fake pca agent and starts the normal one.
func (ali *AttestationLocalInfra) disableFakePCAAgent(ctx context.Context) error {
	var firstErr error
	if ali.fpca != nil {
		if err := ali.fpca.Stop(); err != nil {
			testing.ContextLog(ctx, "Failed to stop fake pca agent: ", err)
			firstErr = errors.Wrap(err, "failed to stop fake pca agent")
		} else {
			ali.fpca = nil
		}
	}
	if err := ali.dc.Start(ctx, hwsec.PCAAgentDaemon); err != nil {
		testing.ContextLog(ctx, "Failed to start normal pca agent: ", err)
		if firstErr == nil {
			firstErr = errors.Wrap(err, "failed to start normal pca agent")
		}
	}
	return firstErr
}
