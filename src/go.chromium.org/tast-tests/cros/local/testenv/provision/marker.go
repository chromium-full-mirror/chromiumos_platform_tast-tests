// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package provision contains any utils that help provision test environments on a DUT.
package provision

import (
	"os"
	"sync"

	"go.chromium.org/tast/core/errors"
)

type refCounted struct {
	refCount int
	lock     *sync.Mutex
}

// Marker is a singleton instance that manages the marker files to ensure that the infra repairs a DUT from any unrecoverable issues.
// It maintains the reference count to delete them when they are no longer needed.
var Marker = &refCounted{
	refCount: 0,
	lock:     &sync.Mutex{},
}

// If these files exist on a DUT, the test infra will attempt to reprovision the DUT for repair.
var markerFiles = []string{
	"/mnt/stateful_partition/unencrypted/provision_failed",
	"/mnt/stateful_partition/.force_provision",
}

// Acquire writes the marker files.
func (r *refCounted) Acquire() error {
	r.lock.Lock()
	defer r.lock.Unlock()

	// Write the marker files only the first time it's requested.
	if r.refCount == 0 {
		for _, f := range markerFiles {
			if err := os.WriteFile(f, nil, 0644); err != nil {
				return errors.Wrapf(err, "failed to write provision markers for recovery: %v", f)
			}
		}
	}
	r.refCount++
	return nil
}

// Release deletes the marker files when no longer needed.
func (r *refCounted) Release() (err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.refCount--
	if r.refCount == 0 {
		for _, f := range markerFiles {
			if _, err = os.Stat(f); err != nil {
				continue
			}
			if err = os.Remove(f); err != nil {
				continue
			}
		}
	}
	return err // return the last error
}
