// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/kernel"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterKernelServiceServer(srv, &KernelService{s: s})
		},
	})
}

// KernelService implements tast.cros.firmware.KernelService.
type KernelService struct {
	s *testing.ServiceState
}

// GetCgptTable returns structure containing metadata with CGPT partitions.
func (ks *KernelService) GetCgptTable(ctx context.Context, req *pb.GetCgptTableRequest) (resp *pb.GetCgptTableResponse, err error) {
	testing.ContextLog(ctx, "Reading CGPT table for device ", req.BlockDevice)
	rootDevWithoutPart, _ := kernel.SplitRootDevAndPart(ctx, req.BlockDevice)
	partitionTable, err := kernel.GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cgpt table")
	}

	return &pb.GetCgptTableResponse{CgptTable: partitionTable}, nil
}

// GetRawHeader returns the raw header of CGPT partition (first 4096 bytes)
func (ks *KernelService) GetRawHeader(ctx context.Context, req *pb.GetRawHeaderRequest) (*pb.GetRawHeaderResponse, error) {
	if err := testexec.CommandContext(ctx, "dd", "if="+req.PartitionPath, "of=/tmp/cgpt-header", "bs=4096", "count=1", "conv=sync").Run(); err != nil {
		return nil, errors.Wrap(err, "failed to read raw header from partition")
	}
	defer os.Remove("/tmp/cgpt-header")
	rawHeader, err := os.ReadFile("/tmp/cgpt-header")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read raw header dump")
	}
	return &pb.GetRawHeaderResponse{
		RawHeader: rawHeader,
	}, nil
}

// WriteRawHeader writes the raw CGPT header into chosen partitionpartition
func (ks *KernelService) WriteRawHeader(ctx context.Context, req *pb.WriteRawHeaderRequest) (*empty.Empty, error) {
	if err := os.WriteFile("/tmp/cgpt-header", req.RawHeader, os.FileMode(0666)); err != nil {
		return nil, errors.Wrap(err, "failed to save raw header into temporary file")
	}
	defer os.Remove("/tmp/cgpt-header")
	if err := testexec.CommandContext(ctx, "dd", "if=/tmp/cgpt-header", "of="+req.PartitionPath, "bs=4096", "count=1", "conv=sync").Run(); err != nil {
		return nil, errors.Wrap(err, "failed to write raw header into partition")
	}
	return &empty.Empty{}, nil
}

// RestoreCgptAttributes restores CGPT partition attributes directly dumped from GetCgptTable
func (ks *KernelService) RestoreCgptAttributes(ctx context.Context, req *pb.RestoreCgptAttributesRequest) (*empty.Empty, error) {
	rootDevWithoutPart, partitionNumber := kernel.SplitRootDevAndPart(ctx, req.BlockDevice)
	testing.ContextLog(ctx, "Restoring passed CGPT attributes to: ", req.BlockDevice)
	for _, part := range req.CgptTable {
		if len(part.Attrs) == 0 {
			continue
		}
		cgptAddCmdline := []string{"add", "-i", strconv.Itoa(partitionNumber)}
		for _, attr := range part.Attrs {
			switch attr.Name {
			case "legacy_boot":
				cgptAddCmdline = append(cgptAddCmdline, "-B", strconv.Itoa(int(attr.Value)))
			case "priority":
				cgptAddCmdline = append(cgptAddCmdline, "-P", strconv.Itoa(int(attr.Value)))
			case "tries":
				cgptAddCmdline = append(cgptAddCmdline, "-T", strconv.Itoa(int(attr.Value)))
			case "successful":
				cgptAddCmdline = append(cgptAddCmdline, "-S", strconv.Itoa(int(attr.Value)))
			case "required":
				cgptAddCmdline = append(cgptAddCmdline, "-R", strconv.Itoa(int(attr.Value)))
			}
		}
		cgptAddCmdline = append(cgptAddCmdline, rootDevWithoutPart)
		testing.ContextLog(ctx, "Restoring CGPT metadata: ", strings.Join(cgptAddCmdline, " "))
		if err := testexec.CommandContext(ctx, "cgpt", cgptAddCmdline...).Run(testexec.DumpLogOnError); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to restore cgpt attributes")
		}
	}

	return &empty.Empty{}, nil
}

// BackupPartition backs up partition and saves to a file.
func (ks *KernelService) BackupPartition(ctx context.Context, req *pb.Partition) (*pb.PartitionInfo, error) {
	var rootDevWithPart string
	if req.RootDev != "" {
		rootDevWithPart = req.RootDev
	} else {
		var err error
		rootDevWithPart, err = kernel.GetCurrentRootDevice(ctx, true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get root device")
		}
	}
	rootDevWithoutPart, _ := kernel.SplitRootDevAndPart(ctx, rootDevWithPart)

	table, path, err := kernel.BackupPartition(ctx, rootDevWithoutPart, kernel.PartitionNameCopyToLabel(req.Name, req.Copy))
	if err != nil {
		return nil, errors.Wrap(err, "could not back up partition")
	}

	return &pb.PartitionInfo{
		Name:       req.Name,
		Copy:       req.Copy,
		BackupPath: path,
		RootDev:    rootDevWithPart,
		Table:      table,
	}, nil
}

// RestorePartition restores partition from backup to current boot device.
func (ks *KernelService) RestorePartition(ctx context.Context, req *pb.PartitionInfo) (*empty.Empty, error) {
	rootDevWithoutPart, err := kernel.GetCurrentRootDevice(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't get current rootdev")
	}
	rootDevWithPart := kernel.RootDevPartitionPath(rootDevWithoutPart, int(req.Table.PartitionNumber))

	if err := kernel.RestorePartition(ctx, req.BackupPath, rootDevWithPart); err != nil {
		return nil, errors.Wrap(err, "could not restore partition from backup")
	}

	label := kernel.PartitionNameCopyToLabel(req.Name, req.Copy)
	if _, err := ks.RestoreCgptAttributes(ctx, &pb.RestoreCgptAttributesRequest{
		BlockDevice: rootDevWithPart, // RootDev includes partition number.
		CgptTable:   map[string]*pb.CgptPartition{label: req.Table},
	}); err != nil {
		return nil, errors.Wrap(err, "failed to retore CGPT attributes")
	}

	return &empty.Empty{}, nil
}

// BackupKernel backs up both kernel A and B copies, and corresponding ROOTFS verity hashes and saves them to a file.
func (ks *KernelService) BackupKernel(ctx context.Context, req *pb.KernelBackup) (*pb.KernelBackup, error) {
	kernA, err := ks.BackupPartition(ctx, &pb.Partition{
		Name:    pb.PartitionName_KERNEL,
		Copy:    pb.PartitionCopy_A,
		RootDev: req.RootDev,
	})
	if err != nil {
		// If backing up partition fails, unfinished backup file is already deleted.
		return nil, errors.Wrap(err, "failed to back up KERN-A")
	}

	kernB, err := ks.BackupPartition(ctx, &pb.Partition{
		Name:    pb.PartitionName_KERNEL,
		Copy:    pb.PartitionCopy_B,
		RootDev: req.RootDev,
	})
	if err != nil {
		// If backing up KERN-B fails, make sure KERN-A back up is cleaned up, KERN-B tmpfile will already be cleaned up.
		os.Remove(kernA.BackupPath)
		return nil, errors.Wrap(err, "failed to back up KERN-B")
	}

	req.KernA = kernA
	req.KernB = kernB
	req.RootDev = kernA.RootDev

	if req.BackupRootfs {
		rootA, err := ks.BackupPartition(ctx, &pb.Partition{
			Name:    pb.PartitionName_ROOTFS,
			Copy:    pb.PartitionCopy_A,
			RootDev: req.RootDev,
		})
		if err != nil {
			os.Remove(kernA.BackupPath)
			os.Remove(kernB.BackupPath)
			return nil, errors.Wrap(err, "failed to back up ROOTFS-A")
		}

		rootB, err := ks.BackupPartition(ctx, &pb.Partition{
			Name:    pb.PartitionName_ROOTFS,
			Copy:    pb.PartitionCopy_B,
			RootDev: req.RootDev,
		})
		if err != nil {
			os.Remove(kernA.BackupPath)
			os.Remove(kernB.BackupPath)
			os.Remove(rootA.BackupPath)
			return nil, errors.Wrap(err, "failed to back up ROOTFS-B")
		}

		req.RootA = rootA
		req.RootB = rootB
	}

	return req, nil
}

// RestoreKernel restores both kernel A and B, and corresponding rootfs from back ups.
func (ks *KernelService) RestoreKernel(ctx context.Context, req *pb.KernelBackup) (*empty.Empty, error) {
	var retErr error
	if _, err := ks.RestorePartition(ctx, req.KernA); err != nil {
		// If restoring A failed, try to restore B instead of returning immediately.
		retErr = errors.Wrap(err, "failed to restore KERN-A")
	}

	if _, err := ks.RestorePartition(ctx, req.KernB); err != nil {
		if retErr != nil {
			// If restoring both A and B fails, report both errors instead of just latest.
			retErr = errors.Wrap(errors.Wrap(err, "failed to restore KERN-B"), retErr.Error())
		} else {
			retErr = errors.Wrap(err, "failed to restore KERN-B")
		}
	}

	if req.BackupRootfs {
		if _, err := ks.RestorePartition(ctx, req.RootA); err != nil {
			retErr = errors.Wrap(err, "failed to restore ROOT-A")
		}

		if _, err := ks.RestorePartition(ctx, req.RootB); err != nil {
			if retErr != nil {
				// If restoring both A and B fails, report both errors instead of just latest.
				retErr = errors.Wrap(errors.Wrap(err, "failed to restore ROOT-B"), retErr.Error())
			} else {
				retErr = errors.Wrap(err, "failed to restore ROOT-B")
			}
		}
	}
	return &empty.Empty{}, retErr
}

// EnsureBothKernelCopiesBootable makes sure both kernel copies are identical and bootable.
func (ks *KernelService) EnsureBothKernelCopiesBootable(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	rootDevWithPart, err := kernel.GetCurrentRootDevice(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current root device")
	}
	if err := kernel.EnsureBothKernelCopiesBootable(ctx, rootDevWithPart); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// PrioritizeKernelCopy ensures DUT boots to expected kernel copy on next reboot (eg. KERN-A or KERN-B).
func (ks *KernelService) PrioritizeKernelCopy(ctx context.Context, req *pb.Partition) (*empty.Empty, error) {
	rootDevWithoutPart, err := kernel.GetCurrentRootDevice(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current root device")
	}
	if err := kernel.PrioritizeKernelCopy(ctx, rootDevWithoutPart, kernel.PartitionNameCopyToLabel(req.Name, req.Copy)); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// GetCurrentCopy returns the current label DUT is using.
func (ks *KernelService) GetCurrentCopy(ctx context.Context, req *pb.Partition) (*pb.Partition, error) {
	rootDevWithPart, err := kernel.GetCurrentRootDevice(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current root device")
	}

	currPart, err := kernel.GetPartitionTable(ctx, rootDevWithPart)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query current kernel copy")
	}
	testing.ContextLog(ctx, "DUT is currently booted to ", currPart.Label)

	name, err := kernel.GetNameFromLabel(currPart.Label)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get name from label")
	}
	copy, err := kernel.GetCopyFromLabel(currPart.Label)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get copy from label")
	}

	return &pb.Partition{
		Name:    kernel.PartNameToNameEnum[name],
		Copy:    kernel.CopyToCopyEnum[copy],
		RootDev: rootDevWithPart,
	}, nil
}

// VerifyKernelCopy checks that DUT is currently booted to expected kernel copy.
// Provide rootdev in req to it check it's the right device (eg. disk or usb) as well as copy.
func (ks *KernelService) VerifyKernelCopy(ctx context.Context, req *pb.Partition) (*empty.Empty, error) {
	currRootDevWithPart, err := kernel.GetCurrentRootDevice(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get root device")
	}
	currPart, err := kernel.GetPartitionTable(ctx, currRootDevWithPart)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query current kernel copy")
	}
	testing.ContextLogf(ctx, "DUT is currently booted from %s (label: %q)", currPart.PartitionPath, currPart.Label)

	currCopy, _ := kernel.GetCopyFromLabel(currPart.Label)
	expCopy := kernel.CopyEnumToCopy[req.Copy]
	if currCopy != expCopy {
		return nil, errors.Errorf("expected kernel copy to be %q but was booted to %q", expCopy, currPart.Label)
	}
	return &empty.Empty{}, nil
}

// BackupRootfsVerityHash saves the verity hash for given kernel from the corresponding rootfs partition.
func (ks *KernelService) BackupRootfsVerityHash(ctx context.Context, req *pb.PartitionInfo) (*pb.RootfsVerityHashBackup, error) {
	var rootDevWithPart string
	if req.RootDev != "" {
		rootDevWithPart = req.RootDev
	} else {
		var err error
		rootDevWithPart, err = kernel.GetCurrentRootDevice(ctx, true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get root device")
		}
	}

	offset, size, backupPath, err := kernel.BackupRootfsVerityHash(ctx, rootDevWithPart, req.Copy)
	if err != nil {
		return nil, errors.Wrap(err, "could not back up rootfs verity hash")
	}

	return &pb.RootfsVerityHashBackup{
		Name:       req.Name,
		Copy:       req.Copy,
		RootDev:    req.RootDev,
		Offset:     offset,
		HashSize:   size,
		BackupPath: backupPath,
	}, nil
}

// RestoreRootfsVerityHash sets the verity hash for the current root device from backup.
func (ks *KernelService) RestoreRootfsVerityHash(ctx context.Context, req *pb.RootfsVerityHashBackup) (*empty.Empty, error) {
	rootDevWithoutPart, err := kernel.GetCurrentRootDevice(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get root device")
	}
	if err := kernel.RestoreRootfsVerityHash(ctx, req.Offset, req.BackupPath, rootDevWithoutPart, req.Copy); err != nil {
		return nil, errors.Wrap(err, "could not restore rootfs verity hash")
	}

	return &empty.Empty{}, nil
}

// CorruptRootfsVerityHash corrupts verity hash for given kernel copy on current root device.
func (ks *KernelService) CorruptRootfsVerityHash(ctx context.Context, req *pb.RootfsVerityHashBackup) (*empty.Empty, error) {
	rootDevWithoutPart, err := kernel.GetCurrentRootDevice(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get root device")
	}
	if corruptErr := kernel.CorruptRootfsVerityHash(ctx, req.Offset, req.HashSize, rootDevWithoutPart, req.Copy); corruptErr != nil {
		if restoreErr := kernel.RestoreRootfsVerityHash(ctx, req.Offset, req.BackupPath, rootDevWithoutPart, req.Copy); restoreErr != nil {
			return nil, errors.Wrapf(restoreErr, "could not corrupt rootfs verity hash and failed to restore it back to original hash: %v", corruptErr)
		}
		return nil, errors.Wrap(corruptErr, "could not corrupt rootfs verity hash")
	}

	return &empty.Empty{}, nil
}

// GetKernelVersion uses vbutil_kernel to get the kernel version for a given partition.
func (ks *KernelService) GetKernelVersion(ctx context.Context, req *pb.Partition) (*pb.KernelVersion, error) {
	var rootDevWithPart string
	if req.RootDev != "" {
		rootDevWithPart = req.RootDev
	} else {
		var err error
		rootDevWithPart, err = kernel.GetCurrentRootDevice(ctx, true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get root device")
		}
	}
	rootDevWithoutPart, _ := kernel.SplitRootDevAndPart(ctx, rootDevWithPart)

	version, table, err := kernel.GetKernelVersion(ctx, rootDevWithoutPart, kernel.PartitionNameCopyToLabel(req.Name, req.Copy))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kernel version")
	}

	return &pb.KernelVersion{
		RootDev: rootDevWithPart,
		Version: version,
		Table:   table,
	}, nil
}

// SetKernelVersion uses vbutil_kernel to set the kernel version for a given partition.
func (ks *KernelService) SetKernelVersion(ctx context.Context, req *pb.KernelVersion) (*empty.Empty, error) {
	var rootDevWithPart string
	if req.RootDev != "" {
		rootDevWithPart = req.RootDev
	} else {
		var err error
		rootDevWithPart, err = kernel.GetCurrentRootDevice(ctx, true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get root device")
		}
	}
	rootDevWithoutPart, _ := kernel.SplitRootDevAndPart(ctx, rootDevWithPart)

	if err := kernel.SetKernelVersion(ctx, rootDevWithoutPart, req.Table.Label, req.Version); err != nil {
		return nil, errors.Wrapf(err, "failed to set kernel version to %q", req.Version)
	}
	return &empty.Empty{}, nil
}

// SetKernelHeaderMagic sets the header magic for the kernel.
func (ks *KernelService) SetKernelHeaderMagic(ctx context.Context, req *pb.KernelHeaderMagicInfo) (*empty.Empty, error) {
	var rootDevWithPart string
	if req.RootDev != "" {
		rootDevWithPart = req.RootDev
	} else {
		var err error
		rootDevWithPart, err = kernel.GetCurrentRootDevice(ctx, true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get root device")
		}
	}
	rootDevWithoutPart, _ := kernel.SplitRootDevAndPart(ctx, rootDevWithPart)

	label := kernel.PartitionNameCopyToLabel(req.Name, req.Copy)
	if err := kernel.SetKernelHeaderMagic(ctx, rootDevWithoutPart, label, kernel.HeaderMagicEnumToMagic[req.Magic], req.ForceBoot); err != nil {
		return nil, errors.Wrap(err, "failed to set kernel header magic")
	}
	return &empty.Empty{}, nil
}

// GetCurrentRootDevice gets the path to the current root device with part number.
func (ks *KernelService) GetCurrentRootDevice(ctx context.Context, req *empty.Empty) (*pb.Partition, error) {
	rootDevWithPart, err := kernel.GetCurrentRootDevice(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current root device")
	}
	return &pb.Partition{RootDev: rootDevWithPart}, nil
}
