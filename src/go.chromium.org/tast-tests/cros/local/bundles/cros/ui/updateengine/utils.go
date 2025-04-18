// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package updateengine contains utilities for chromeos update tasks.
package updateengine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/async"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// PayloadFilename is the filename of the payload for nebraska server.
	PayloadFilename = "cuj_au_payload_20250415.bin"
	// MetadataFilename is the filename of the metadata for nebraska server.
	MetadataFilename = "cuj_au_payload_20250415.bin.json"
)

// AURunner is a utility to run update engine in the background.
type AURunner struct {
	imagePayloadDir  string
	imageMetadataDir string
	nebraskaServer   *nebraska.Nebraska
	updateCmd        *testexec.Cmd

	isRunning    bool
	runnerStatus chan error
}

// NewAURunner creates a new AURunner.
func NewAURunner(imagePayloadPath, imageMetadataPath string) *AURunner {
	imagePayloadDir := filepath.Dir(imagePayloadPath)
	imageMetadataDir := filepath.Dir(imageMetadataPath)

	return &AURunner{
		imagePayloadDir:  imagePayloadDir,
		imageMetadataDir: imageMetadataDir,
		isRunning:        false,
	}
}

// Start AURunner background.
func (r *AURunner) Start(ctx context.Context) error {
	testing.ContextLog(ctx, "AURunner starting")
	if r.isRunning {
		return errors.New("runner already running")
	}
	r.runnerStatus = make(chan error, 1)

	// Start a nebraska server with given payload and metadata.
	nebraskaServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
	if err != nil {
		return errors.Wrap(err, "failed to start nebraska")
	}
	// Kill the nebraska server if it fails to configure.
	defer func(ctx context.Context) {
		if r.nebraskaServer == nil {
			if err := nebraskaServer.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close nebraska server due to configuration failure: ", err)
			}
		}
	}(ctx)

	if err := nebraskaServer.SetUpdateMetadata(ctx, r.imageMetadataDir); err != nil {
		return errors.Wrap(err, "failed to set update metadata")
	}
	if err := nebraskaServer.SetUpdatePayloadsAddress(ctx, fmt.Sprint("file://", r.imagePayloadDir)); err != nil {
		return errors.Wrap(err, "failed to set update payload address")
	}
	if err := nebraskaServer.SetCriticalUpdate(ctx, true); err != nil {
		return errors.Wrap(err, "failed to configure Nebraska with critical update")
	}
	if err := nebraskaServer.SetIgnoreAppID(ctx, true); err != nil {
		return errors.Wrap(err, "failed to configure Nebraska with ignore app id")
	}
	// Nebraska server is successfully configured.
	r.nebraskaServer = nebraskaServer

	r.isRunning = true
	async.Run(ctx, func(ctx context.Context) {
		for r.isRunning {
			err := r.Update(ctx)
			if err != nil {
				r.runnerStatus <- errors.Wrap(err, "failed to run update background tasks")
				return
			}

			if !r.isRunning {
				break
			}
		}
	}, "AURunner")

	return nil
}

// Stop will finish the background running update task. It will wait for any in-flight
// execution to finish, with a 1 minute timeout.
func (r *AURunner) Stop(ctx context.Context) error {
	testing.ContextLog(ctx, "AURunner stopping")
	if !r.isRunning {
		return errors.New("runner isn't running")
	}
	r.isRunning = false

	if err := r.nebraskaServer.Close(ctx); err != nil {
		return errors.Wrap(err, "failed to close nebraska server")
	}

	var err error
	select {
	case err = <-r.runnerStatus:
		return err
	case <-time.After(time.Minute):
		if err := r.updateCmd.Kill(); err != nil &&
			!errors.Is(err, testexec.ErrNotStarted) &&
			!errors.Is(err, testexec.ErrAlreadyWaited) {
			return errors.Wrap(err, "failed to kill AU runner")
		}
		testing.ContextLog(ctx, "AU runner has been successfully killed or stopped")
	}
	r.runnerStatus = nil

	return err
}

// Update starts a nebraska server, sets update payload and metadata, and
// uses update_engine_client to start the update.
func (r *AURunner) Update(ctx context.Context) error {
	r.updateCmd = testexec.CommandContext(ctx,
		"update_engine_client", fmt.Sprintf("--omaha_url=%s", nebraska.UpdateURL(r.nebraskaServer.Port)),
		"--update")
	testing.ContextLogf(ctx, "%v starting", r.updateCmd.String())
	output, _ := r.updateCmd.CombinedOutput()
	if !strings.Contains(string(output), "ErrorCode::kPayloadHashMismatchError") {
		return errors.New("failed to find ErrorCode::kPayloadHashMismatchError in output")
	}
	testing.ContextLogf(ctx, "%v stopped", r.updateCmd.String())

	return nil
}
