// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package storage provides the util functions to set up crosvm's guest storage.
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

// VirtioFSCacheTimeoutSecond represents the duration of virtiofs device's cache.
const VirtioFSCacheTimeoutSecond = 1

// Option holds parameters for a guest storage.
type Option struct {
	Kind     string
	Tag      string
	cache    string
	caseFold bool
}

// NewOption creates a new instance of Option.
func NewOption(kind, cache string, caseFold bool) (Option, error) {
	var opt Option
	opt.Kind = kind
	if kind == "block" || kind == "block_packed" {
		opt.Tag = "/dev/vda"
	} else if kind == "virtiofs" || kind == "virtiofs_dax" || kind == "p9" {
		opt.Tag = "shared"
		opt.cache = cache
		opt.caseFold = caseFold
	} else if kind == "scsi" {
		opt.Tag = "/dev/sda"
	} else if kind == "pmem" {
		opt.Tag = "/dev/pmem0"
	} else {
		return opt, errors.Errorf("invalid storage kind: %v", kind)
	}
	return opt, nil
}

// GenCrosvmCmd constructs a new crosvm command using the given parameters.
func GenCrosvmCmd(socketDir, userDir, outDir, kernel, script string, opt Option, scriptArgs []string) (crosvmParams *vm.CrosvmParams, err error) {
	shared := filepath.Join(userDir, "shared")
	if err := os.Mkdir(shared, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create shared directory")
	}

	block := filepath.Join(userDir, "block")
	f, err := os.Create(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create block device file")
	}
	defer f.Close()

	if err := f.Truncate(8 * 1024 * 1024 * 1024); err != nil {
		return nil, errors.Wrap(err, "failed to set block device file size")
	}

	logFilePath := filepath.Join(outDir, "serial.log")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a input file")
	}

	var storageOpt vm.Option

	if opt.Kind == "block" || opt.Kind == "block_packed" {
		blockOption := fmt.Sprintf("%s,packed-queue=%v", block, opt.Kind == "block_packed")
		storageOpt = vm.RWDisks(blockOption)
	} else if opt.Kind == "virtiofs" || opt.Kind == "virtiofs_dax" {
		storageOpt = vm.SharedDir(vm.SharedDirParam{
			Src: shared, Tag: opt.Tag, FsType: "fs", Cache: opt.cache, Timeout: VirtioFSCacheTimeoutSecond, Writeback: true, DAX: opt.Kind == "virtiofs_dax", CaseFold: opt.caseFold})
	} else if opt.Kind == "p9" {
		storageOpt = vm.SharedDir(vm.SharedDirParam{
			Src: shared, Tag: opt.Tag, FsType: "p9", Timeout: 5, Writeback: false, DAX: false})
	} else if opt.Kind == "scsi" {
		storageOpt = vm.ScsiPaths(block)
	} else if opt.Kind == "pmem" {
		storageOpt = vm.PmemPaths(block)
	} else {
		return nil, errors.Wrap(err, "unknown storage device type")
	}

	kernelArgs := []string{
		"root=root",
		"rootfstype=virtiofs",
		"rw",
		fmt.Sprintf("init=%s", script),
		"--",
	}
	kernelArgs = append(kernelArgs, scriptArgs...)

	return vm.NewCrosvmParams(
		kernel,
		vm.NumCpus(uint(runtime.NumCPU())),
		vm.MemSize(1024),
		vm.Socket(socketDir),
		vm.SharedDir(
			vm.SharedDirParam{
				Src:       "/",
				Tag:       "root",
				FsType:    "fs",
				Cache:     "always",
				Timeout:   5,
				Writeback: false,
				DAX:       false,
			}),
		vm.KernelArgs(kernelArgs...),
		vm.SerialOutput(logFilePath),
		storageOpt,
	), nil
}
