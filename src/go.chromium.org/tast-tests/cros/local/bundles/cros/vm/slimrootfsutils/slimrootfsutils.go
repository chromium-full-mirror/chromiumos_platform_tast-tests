// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package slimrootfsutils provides the util functions needed for slim rootfs VM tests.
package slimrootfsutils

const (
	tatlKernelDataFile = "tatl_slim_vm_kernel_amd64"
	tatlRootfsDataFile = "tatl_slim_vm_rootfs_amd64"
	taelKernelDataFile = "tael_slim_vm_kernel_amd64"
	taelRootfsDataFile = "tael_slim_vm_rootfs_amd64"
	// StatefulDiskSizeBytes is the default size of a slim rootfs VM.
	StatefulDiskSizeBytes = 50 * 1024 * 1024
	// DefaultVMName is the default name of a slim rootfs VM.
	DefaultVMName = "slimvm"
)

// GetDataBasedOnBoards returns data files based on the board type.
func GetDataBasedOnBoards(board string) []string {
	if board == "amd64" {
		return []string{tatlKernelDataFile, tatlRootfsDataFile}
	}
	return []string{taelKernelDataFile, taelRootfsDataFile}
}
