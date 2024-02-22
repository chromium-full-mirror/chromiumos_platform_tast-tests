// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package storage provides the util functions to set up crosvm's guest storage.
package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/spaced"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CheckFreeSpace checks if the given path has enough free space.
func CheckFreeSpace(ctx context.Context, path string, bytes uint64) error {
	c, err := spaced.NewClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create spaced client")
	}

	avail, err := c.FreeDiskSpace(ctx, path)
	if err != nil {
		return errors.Wrap(err, "failed to query free disk space")
	}

	if uint64(avail) < bytes {
		return errors.Errorf("insufficient free space: %d < %d", avail, bytes)
	}

	return nil
}

// Option holds parameters for a guest storage.
type Option struct {
	Kind            string
	Tag             string
	cache           string
	caseFold        bool
	negativeTimeout time.Duration
}

// NewOption creates a new instance of Option.
func NewOption(kind, cache string, caseFold bool, negativeTimeout int) (Option, error) {
	var opt Option
	opt.Kind = kind
	if strings.HasPrefix(kind, "block") {
		opt.Tag = "/dev/vda"
	} else if kind == "virtiofs" || kind == "virtiofs_dax" || kind == "p9" {
		opt.Tag = "shared"
		opt.cache = cache
		opt.caseFold = caseFold
		opt.negativeTimeout = time.Duration(negativeTimeout) * time.Second
	} else if kind == "scsi" {
		opt.Tag = "/dev/sda"
	} else if kind == "pmem" {
		opt.Tag = "/dev/pmem0"
	} else {
		return opt, errors.Errorf("invalid storage kind: %v", kind)
	}
	return opt, nil
}

func virtiofsTimeout(c string) (time.Duration, error) {
	var timeout time.Duration
	switch c {
	case "always":
		timeout = time.Duration(1) * time.Hour
	case "auto":
		timeout = time.Duration(1) * time.Second
	case "never":
		timeout = time.Duration(0)
	default:
		return 0, errors.Errorf("unknown cache policy: %s", c)
	}
	return timeout, nil
}

// SetUpLogicVolume creates a logic volume with lvName in thinpool
//
// Returns file path of newly created logical volume and cleanup function that removes the logical volume (if no error)
func SetUpLogicVolume(ctx context.Context, lvName string, bytes uint64) (lvPath string, cleanUp func(ctx context.Context), _ error) {
	// Create command to get volume group name
	out, err := testexec.CommandContext(ctx, "vgs", "-o", "vg_name", "--noheadings").Output()
	if err != nil {
		return "", func(_ context.Context) {}, errors.Wrap(err, "failed to get volume group name")
	}

	if len(out) == 0 {
		return "", func(_ context.Context) {}, errors.New("there is no volume group")
	}

	vgNames := strings.Split(string(out), "\n")
	for _, vgName := range vgNames {
		vgName = strings.TrimSpace(string(vgName))
		thinpool := vgName + "/thinpool"
		lvPath = filepath.Join("/dev/mapper/", vgName+"-"+lvName)

		// Check if there the logic volume contains thinpool
		if err := testexec.CommandContext(ctx, "lvdisplay", "-v", thinpool).Run(); err != nil {
			continue
		}
		// Create a logic volume in thinpool
		if err := testexec.CommandContext(ctx, "lvcreate", "-V", strconv.FormatUint(bytes, 10), "-T", thinpool, "-n", lvName).Run(); err != nil {
			return "", func(_ context.Context) {}, errors.Wrap(err, "failed to create logical volume on "+thinpool)
		}

		cleanUp = func(ctx context.Context) {
			if err := testexec.CommandContext(ctx, "lvremove", "-y", lvPath).Run(); err != nil {
				testing.ContextLog(ctx, "Failed to remove logical volume: ", err)
			}
		}

		return lvPath, cleanUp, nil
	}

	return "", func(_ context.Context) {}, errors.New("failed to create a logical volume")
}

// SetUpBlockFile creates a sparse file with the given length and returns the file path
//
// Returns path of created block image file and cleanup function that removes the file (if no error)
func SetUpBlockFile(ctx context.Context, userDir string, bytes uint64) (blockPath string, cleanUp func(ctx context.Context), _ error) {
	blockPath = filepath.Join(userDir, "block")
	f, err := os.Create(blockPath)
	if err != nil {
		return "", func(_ context.Context) {}, errors.Wrap(err, "failed to create block device file")
	}
	defer f.Close()

	cleanUp = func(ctx context.Context) {
		if err := os.Remove(blockPath); err != nil {
			testing.ContextLog(ctx, "Failed to remove host block image: ", err)
		}
	}

	if err := f.Truncate(int64(bytes)); err != nil {
		return "", cleanUp, errors.Wrap(err, "failed to set block device file size")
	}

	return blockPath, cleanUp, nil
}

// GenCrosvmCmd constructs a new crosvm command using the given parameters.
func GenCrosvmCmd(socketDir, userDir, outDir, kernel, block, script string, opt Option, scriptArgs []string) (crosvmParams *vm.CrosvmParams, err error) {
	shared := filepath.Join(userDir, "shared")
	if err := os.Mkdir(shared, 0755); err != nil {
		return nil, errors.Wrap(err, "failed to create shared directory")
	}

	// Some boards (puff, fizz, hatch, soraka, octopus, and nocturne) disabled serial console on arcvm guest kernel.
	// To have valid serial.log in these boards, build arcvm guest kernel by:
	// USE=pcserial emerge-$BOARD sys-kernel/arcvm-kernel-ack-5_10
	logFilePath := filepath.Join(outDir, "serial.log")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a input file")
	}

	var storageOpt vm.Option

	if opt.Kind == "block" || opt.Kind == "block_tpq" || opt.Kind == "block_lvm" {
		isTpq := opt.Kind == "block_tpq" || opt.Kind == "block_packed_tpq"
		isODIRECT := opt.Kind == "block_lvm"
		blockOption := fmt.Sprintf("%s,multiple-workers=%v,o_direct=%v", block, isTpq, isODIRECT)
		storageOpt = vm.RWDisks(blockOption)
	} else if opt.Kind == "virtiofs" || opt.Kind == "virtiofs_dax" {
		timeout, err := virtiofsTimeout(opt.cache)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get timeout for virtiofs")
		}

		storageOpt = vm.SharedDir(vm.SharedDirParam{
			Src:             shared,
			Tag:             opt.Tag,
			FsType:          "fs",
			Cache:           opt.cache,
			Timeout:         uint(timeout.Seconds()),
			Writeback:       true,
			DAX:             opt.Kind == "virtiofs_dax",
			CaseFold:        opt.caseFold,
			NegativeTimeout: uint(opt.negativeTimeout.Seconds()),
		})
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
				Tag:       "/dev/root",
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
