// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/filesnapshot"
	"go.chromium.org/tast-tests/cros/local/hwsec/enckey"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
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
	if _, err := os.Stat(hwsec.AttestationDBPath); err == nil {
		if err := ali.snapshot.Stash(hwsec.AttestationDBPath); err != nil {
			return errors.Wrap(err, "failed to stash attestation database")
		}
		ali.dbStashed = true
	} else if !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to check stat of attestation database")
	}

	cleanupCtx := ctx
	// The shortened ctx's associated cancel() is not called intentionally as it will
	// be used in FakePCAAgent created below. The object will hold the ctx and it
	// outlives this function. We rely on the ctx's parent to cancel it.
	ctx, _ = ctxutil.Shorten(ctx, 5*time.Second)

	defer func(ctx context.Context) {
		if lastErr != nil && ali.dbStashed {
			if err := ali.snapshot.Pop(hwsec.AttestationDBPath); err != nil {
				testing.ContextLog(ctx, "Failed to pop attestation database back: ", err)
			}
		}
	}(cleanupCtx)

	// Restore the fake attestation database.
	if err := fsutil.CopyFile(attestationDBBackupPath, hwsec.AttestationDBPath); err != nil {
		return errors.Wrap(err, "failed to restore the fake attestation database")
	}

	if err := ali.enableFakePCAAgent(ctx); err != nil {
		return errors.Wrap(err, "failed to enable fake pca agent")
	}

	if err := enckey.InjectWellKnownGoogleKeysAndRestart(ctx, ali.dc); err != nil {
		return errors.Wrap(err, "failed to inject well-known keys")
	}

	// Revert the key injection if other parts of this function fails.
	defer func(ctx context.Context) {
		if lastErr != nil {
			if err := enckey.InjectNormalGoogleKeys(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to inject the normal key back: ", err)
			}
		}
	}(cleanupCtx)

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
	if err := ali.disableFakePCAAgent(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to disable fake pca agent: ", err)
		lastErr = errors.Wrap(err, "failed to disable fake pca agent")
	}
	if err := enckey.InjectNormalGoogleKeysAndRestart(ctx, ali.dc); err != nil {
		testing.ContextLog(ctx, "Failed to inject the normal key back: ", err)
		lastErr = errors.Wrap(err, "failed to inject the normal key back")
	}
	return lastErr
}

// enableFakePCAAgent stops the normal pca agent and starts the fake one.
func (ali *AttestationLocalInfra) enableFakePCAAgent(ctx context.Context) (lastErr error) {
	if err := ali.dc.Stop(ctx, hwsec.PCAAgentDaemon); err != nil {
		return errors.Wrap(err, "failed to stop normal pca agent")
	}

	cleanupCtx := ctx
	// The shortened ctx's associated cancel() is not called intentionally as it will
	// be used in FakePCAAgent created below. The object will hold the ctx and it
	// outlives this function. We rely on the ctx's parent to cancel it.
	ctx, _ = ctxutil.Shorten(ctx, 3*time.Second)
	defer func(ctx context.Context) {
		if lastErr != nil {
			if err := ali.dc.Start(ctx, hwsec.PCAAgentDaemon); err != nil {
				testing.ContextLog(ctx, "Failed to stop start normal pca agent: ", err)
			}
		}
	}(cleanupCtx)

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
