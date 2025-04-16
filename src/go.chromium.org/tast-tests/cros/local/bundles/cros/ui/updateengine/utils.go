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

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// PayloadFilename is the filename of the payload for nebraska server.
	PayloadFilename = "cuj_au_payload_20250415.bin"
	// MetadataFilename is the filename of the metadata for nebraska server.
	MetadataFilename = "cuj_au_payload_20250415.bin.json"
)

// Update starts a nebraska server, sets update payload and metadata, and
// uses update_engine_client to start the update.
func Update(ctx context.Context, imagePayloadPath, imageMetadataPath string) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	imagePayloadDir := filepath.Dir(imagePayloadPath)
	imageMetadataDir := filepath.Dir(imageMetadataPath)

	updateServer, err := nebraska.New(ctx, nebraska.ConfigureUpdateEngine())
	if err != nil {
		return errors.Wrap(err, "failed to start nebraska")
	}
	defer updateServer.Close(cleanupCtx)

	if err := updateServer.SetUpdateMetadata(ctx, imageMetadataDir); err != nil {
		return errors.Wrap(err, "failed to set update metadata")
	}
	if err := updateServer.SetUpdatePayloadsAddress(ctx, fmt.Sprint("file://", imagePayloadDir)); err != nil {
		return errors.Wrap(err, "failed to set update payload address")
	}
	if err := updateServer.SetCriticalUpdate(ctx, true); err != nil {
		return errors.Wrap(err, "failed to configure Nebraska with critical update")
	}
	if err := updateServer.SetIgnoreAppID(ctx, true); err != nil {
		return errors.Wrap(err, "failed to configure Nebraska with ignore app id")
	}

	testing.ContextLog(ctx, "Update starting")
	output, _ := testexec.CommandContext(ctx,
		"update_engine_client", fmt.Sprintf("--omaha_url=%s", nebraska.UpdateURL(updateServer.Port)),
		"--update").CombinedOutput(testexec.DumpLogOnError)
	if !strings.Contains(string(output), "ErrorCode::kPayloadHashMismatchError") {
		return errors.New("failed to find ErrorCode::kPayloadHashMismatchError in output")
	}
	testing.ContextLog(ctx, "Update finished")

	return nil
}
