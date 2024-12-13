// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils contains some common utilities for the peripherals tests.
package utils

import (
	"context"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

// FaillogServiceName is the service needed for capture fail-logs.
var FaillogServiceName = "tast.cros.ui.ChromeUIService"

// DumpUITreeWithScreenshotToFile dumps the UI tree and takes screenshot on error, save as file with specified name.
// It takes a grpc client connection to creates a service to further dumps the UI tree and takes screenshot.
func DumpUITreeWithScreenshotToFile(ctx context.Context, conn *grpc.ClientConn, hasError func() bool, filePrefix string) error {
	if !hasError() {
		return nil
	}

	svc := ui.NewChromeUIServiceClient(conn)
	if _, err := svc.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{FilePrefix: filePrefix}); err != nil {
		return err
	}
	return nil
}

// CopyFilesToRemote mirrors the given file paths from DataPath to a tempdir
// on the remote, and returns the tempdir path.
func CopyFilesToRemote(ctx context.Context, s *testing.State, d *dut.DUT, fileList []string) (string, error) {
	files := map[string]string{}
	tempdir, err := d.Conn().CommandContext(ctx, "mktemp", "-d", "/tmp/peripherals_XXXXXX").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to create remote data path directory")
	}
	dataPath := strings.TrimSpace(string(tempdir))
	for _, value := range fileList {
		key := s.DataPath(value)
		files[key] = filepath.Join(dataPath, value)
	}
	if _, err := linuxssh.PutFiles(ctx, d.Conn(), files, linuxssh.DereferenceSymlinks); err != nil {
		return "", errors.Wrapf(err, "failed to send data to remote data path %v", dataPath)
	}
	return dataPath, nil
}
