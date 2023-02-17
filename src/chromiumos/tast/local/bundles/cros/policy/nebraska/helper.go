// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package nebraska contains helpers for Nebraska for policy tests.
package nebraska

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/nebraska"
)

// Helper starts a Nebraska instance in a temporary folder and manages lsb-release.
type Helper struct {
	instance *nebraska.Nebraska
	tmpDir   string

	// clearLSBRelease indicates that the lsb-release file in stateful should be removed when Stop is called.
	clearLSBRelease bool
}

const logFileName = "nebraska.log"
const statefulLSBRelease = "/mnt/stateful_partition/etc/lsb-release"

// Start starts a new instance of Nebraska in a separate temp directory.
func Start(ctx context.Context) (*Helper, error) {
	success := false
	tmpDir, err := ioutil.TempDir("", "nebraska-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp dir")
	}
	defer func() {
		if !success {
			os.RemoveAll(tmpDir)
		}
	}()

	instance, err := nebraska.Start(ctx, tmpDir, []string{
		"--log-file", filepath.Join(tmpDir, logFileName),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to start nebraska")
	}

	success = true
	return &Helper{
		tmpDir:   tmpDir,
		instance: instance,
	}, nil
}

// Stop stops the Nebraska instance and cleans up temporary files.
func (h *Helper) Stop(ctx context.Context) error {
	var retErr error = nil
	if h.clearLSBRelease {
		retErr = os.Remove(statefulLSBRelease)
	}

	if err := h.instance.Stop(ctx); err != nil {
		retErr = err
	}

	if err := os.RemoveAll(h.tmpDir); err != nil {
		retErr = err
	}

	return retErr
}

// ReadLog returns to full Nebraska log.
func (h *Helper) ReadLog(ctx context.Context) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(h.tmpDir, logFileName))
}

// URL returns an url that can be passed to update_engine.
func (h *Helper) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/update", h.instance.Port)
}

// ConfigureStatefulLSBRelease writes the Nebraska instance URL to the stateful lsb-release file.
// Overwrites all other file content.
// lsb-release is removed when Stop is called.
func (h *Helper) ConfigureStatefulLSBRelease() error {
	h.clearLSBRelease = true
	return ioutil.WriteFile(statefulLSBRelease, []byte(fmt.Sprintf("CHROMEOS_AUSERVER=%s", h.URL())), 0666)
}
