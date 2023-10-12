// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package kernel contains kernel-related utility functions for local tests.
package kernel

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const (
	// KernelPrivateKeyPath is the path to private key for kernel.
	KernelPrivateKeyPath string = "/usr/share/vboot/devkeys/kernel_data_key.vbprivk"
	// KernelKeyblockPath is the path to kernel keyblock.
	KernelKeyblockPath string = "/usr/share/vboot/devkeys/kernel.keyblock"
)

// HeaderMagic holds the possible values for the kernel header magic.
type HeaderMagic string

const (
	// CHROMEOS is the default header magic for kernel stored in the first 8 bytes of the partition.
	CHROMEOS HeaderMagic = "CHROMEOS"
	// CORRUPTD is the corrupted header magic for kernel, this makes the kernel appear corrupted.
	CORRUPTD HeaderMagic = "CORRUPTD"
)

// HeaderMagicEnumToMagic maps the KernelHeaderMagic enum to the relevant HeaderMagic.
var HeaderMagicEnumToMagic = map[pb.KernelHeaderMagic]HeaderMagic{
	pb.KernelHeaderMagic_CHROMEOS: CHROMEOS,
	pb.KernelHeaderMagic_CORRUPTD: CORRUPTD,
}

// PartNameEnumToName maps the PartitionName enum to the name from cgpt table.
var PartNameEnumToName = map[pb.PartitionName]string{
	pb.PartitionName_KERNEL: "KERN",
	pb.PartitionName_MINIOS: "MINIOS",
	pb.PartitionName_ROOTFS: "ROOT",
}

// CopyEnumToCopy maps the PartitionCopy enum to the copy from cgpt table.
var CopyEnumToCopy = map[pb.PartitionCopy]string{
	pb.PartitionCopy_A: "A",
	pb.PartitionCopy_B: "B",
	pb.PartitionCopy_C: "C",
}

// PartNameToNameEnum maps the partition label name from cgpt table to the PartitionName enum.
var PartNameToNameEnum = map[string]pb.PartitionName{
	"KERN":   pb.PartitionName_KERNEL,
	"MINIOS": pb.PartitionName_MINIOS,
	"ROOT":   pb.PartitionName_ROOTFS,
}

// CopyToCopyEnum maps the partition label name from cgpt table to the PartitionName enum.
var CopyToCopyEnum = map[string]pb.PartitionCopy{
	"A": pb.PartitionCopy_A,
	"B": pb.PartitionCopy_B,
	"C": pb.PartitionCopy_C,
}

// ReadKernelConfig reads the kernel config key value pairs trimming CONFIG_ prefix from the keys.
func ReadKernelConfig(ctx context.Context) (map[string]string, error) {
	configs, err := readKernelConfigBytes(ctx)
	if err != nil {
		return nil, err
	}
	res := make(map[string]string)

	for _, line := range strings.Split(string(configs), "\n") {
		line := strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) < 2 || kv[1] == "" {
			return nil, errors.Errorf("unexpected config line %q", line)
		}
		const configPrefix = "CONFIG_"
		if !strings.HasPrefix(kv[0], configPrefix) {
			return nil, errors.Errorf("config %q doesn't start with %s unexpectedly", kv[0], configPrefix)
		}
		res[strings.TrimPrefix(kv[0], configPrefix)] = kv[1]
	}
	return res, nil
}

// readKernelConfigBytes reads the kernel config bytes
func readKernelConfigBytes(ctx context.Context) ([]byte, error) {
	const filename = "/proc/config.gz"
	// Load configs module to generate /proc/config.gz.
	if err := testexec.CommandContext(ctx, "modprobe", "configs").Run(); err != nil {
		return nil, errors.Wrap(err, "failed to generate kernel config file")
	}
	var r io.ReadCloser
	f, err := os.Open(filename)
	if err != nil {
		testing.ContextLogf(ctx, "Falling back: failed to open %s: %v", filename, err)
		u, err := sysutil.Uname()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get uname")
		}
		fallbackFile := "/boot/config-" + u.Release
		r, err = os.Open(fallbackFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open %s", fallbackFile)
		}
	} else { // Normal path.
		defer f.Close()
		r, err = gzip.NewReader(f)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create gzip reader for %s", filename)
		}
	}
	defer r.Close()
	configs, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config")
	}
	return configs, nil
}

// GetCgptTable returns structure containing metadata with CGPT partitions.
func GetCgptTable(ctx context.Context, rootDevWithoutPart string) (map[string]*pb.CgptPartition, error) {
	cgptOut, err := testexec.CommandContext(ctx, "cgpt", "show", rootDevWithoutPart).CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve cgpt table, got output: %v", string(cgptOut))
	}

	cgptOutLines := strings.Split(string(cgptOut), "\n")
	partitionTable := make(map[string]*pb.CgptPartition)

	for idx, line := range cgptOutLines {
		if !strings.Contains(line, "Label:") {
			continue
		}

		fields := strings.Fields(line)

		partitionStart, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse partition start offset")
		}

		// NOTE: This size is output in number of blocks by cgpt show, NOT bytes.
		partitionSize, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse partition size")
		}

		partitionNumber, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse partition number")
		}

		partitionPath := RootDevPartitionPath(rootDevWithoutPart, partitionNumber)

		partitionLabel := strings.ReplaceAll(fields[4], "\"", "")

		var partitionType string
		var partitionUUID string
		partitionAttrs := make([]*pb.CgptPartitionAttribute, 0)

		if strings.Contains(cgptOutLines[idx+1], "Type:") {
			partitionTypeFields := strings.Fields(cgptOutLines[idx+1])
			partitionType = strings.Join(partitionTypeFields[1:], " ")
		}

		if strings.Contains(cgptOutLines[idx+2], "UUID:") {
			partitionUUIDFields := strings.Fields(cgptOutLines[idx+2])
			partitionUUID = partitionUUIDFields[1]
		}

		if strings.Contains(cgptOutLines[idx+3], "Attr:") {
			partitionAttrFields := strings.Fields(cgptOutLines[idx+3])
			for _, field := range partitionAttrFields[1:] {
				attrFields := strings.Split(field, "=")

				attrName := attrFields[0]
				attrValue, err := strconv.Atoi(attrFields[1])
				if err != nil {
					return nil, errors.Wrap(err, "failed to parse partition attribute value")
				}

				partitionAttribute := &pb.CgptPartitionAttribute{
					Name:  attrName,
					Value: int32(attrValue),
				}
				partitionAttrs = append(partitionAttrs, partitionAttribute)
			}
		}

		partition := &pb.CgptPartition{
			PartitionPath:   partitionPath,
			PartitionNumber: int32(partitionNumber),
			Start:           int32(partitionStart),
			Size:            int32(partitionSize),
			Label:           partitionLabel,
			Type:            partitionType,
			UUID:            partitionUUID,
			Attrs:           partitionAttrs,
		}

		partitionTable[partitionLabel] = partition
	}

	return partitionTable, nil
}

// RestoreCgptAttributes restores CGPT partition attributes directly dumped from GetCgptTable.
func RestoreCgptAttributes(ctx context.Context, rootDevWithoutPart string, cgptTable map[string]*pb.CgptPartition) error {
	cgptTable, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return errors.Wrap(err, "failed to get current cgpt table")
	}
	testing.ContextLog(ctx, "Restoring passed CGPT attributes to: ", rootDevWithoutPart)
	for label, part := range cgptTable {
		partitionNumber := cgptTable[label].PartitionNumber
		rootDevWithPart := RootDevPartitionPath(rootDevWithoutPart, int(partitionNumber))
		if len(part.Attrs) == 0 {
			continue
		}
		cgptAddCmdline := []string{"add", "-i", strconv.Itoa(int(partitionNumber))}
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
		testing.ContextLogf(ctx, "Restoring CGPT metadata for %s: %s", rootDevWithPart, strings.Join(cgptAddCmdline, " "))
		if err := testexec.CommandContext(ctx, "cgpt", cgptAddCmdline...).Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to restore cgpt attributes")
		}
	}

	return nil
}

// GetCurrentRootDevice gets the path to the current root device.
func GetCurrentRootDevice(ctx context.Context, includePart bool) (string, error) {
	args := []string{"-s"}
	if !includePart {
		args = append(args, "-d")
	}
	rootDev, err := testexec.CommandContext(ctx, "rootdev", args...).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to acquire current root device")
	}
	rootDev = []byte(strings.TrimSuffix(string(rootDev), "\n"))
	return string(rootDev), nil
}

// PartitionNameCopyToLabel combines the partition name and copy to a label found in cgpt table.
func PartitionNameCopyToLabel(part pb.PartitionName, copy pb.PartitionCopy) string {
	return fmt.Sprintf("%s-%s", PartNameEnumToName[part], CopyEnumToCopy[copy])
}

// BackupPartition backs up partition and saves to a file.
func BackupPartition(ctx context.Context, rootDevWithoutPart, label string) (*pb.CgptPartition, string, error) {
	partitionTables, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to get cgpt table")
	}

	// Look for table with expected label (eg. KERN-A, MINIOS-B, ROOT-A).
	table := partitionTables[label]

	backupPath, err := os.CreateTemp("/var/tmp", fmt.Sprintf("%s_", label))
	if err != nil {
		os.Remove(backupPath.Name())
		return nil, "", errors.Wrapf(err, "creating tmpfile for backing up partition %s", label)
	}

	cmd := fmt.Sprintf("cat %s > %s", table.PartitionPath, backupPath.Name())
	if err := testexec.CommandContext(ctx, "sh", "-c", cmd).Run(testexec.DumpLogOnError); err != nil {
		os.Remove(backupPath.Name())
		return nil, "", errors.Wrap(err, "failed to save partition to file")
	}

	// This is valuable to log in situations where the backups are saved on a different device (eg. a USB).
	var currRootDev string
	currRootDev, err = GetCurrentRootDevice(ctx, true)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get current root device while backing up partition")
	}

	testing.ContextLogf(ctx, "Partition %q saved from %q to %q on device %q, size %v", label, table.PartitionPath, backupPath.Name(), currRootDev, table.Size)
	return table, backupPath.Name(), nil
}

// RestorePartition restores a partition on booted disk from backup file. Leaves backup file for users to delete.
func RestorePartition(ctx context.Context, backupPath, partitionPath string) error {
	testing.ContextLogf(ctx, "Restoring partition at %q from backup at %q ", partitionPath, backupPath)
	cmd := fmt.Sprintf("cat %s > %s", backupPath, partitionPath)
	if err := testexec.CommandContext(ctx, "sh", "-c", cmd).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to restore partition from backup file")
	}
	return nil
}

// EnsureBothKernelCopiesBootable makes sure both kernel copies are identical and bootable.
func EnsureBothKernelCopiesBootable(ctx context.Context, rootDevWithPart string) error {
	currKernel, err := GetPartitionTable(ctx, rootDevWithPart)
	if err != nil {
		return errors.Wrap(err, "failed to get current cgpt table for current kernel copy")
	}

	rootDevWithoutPart, _ := SplitRootDevAndPart(ctx, rootDevWithPart)
	partitionTable, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return errors.Wrap(err, "failed to read cgpt table")
	}

	kernA := partitionTable["KERN-A"]
	kernB := partitionTable["KERN-B"]

	rootA := partitionTable["ROOT-A"]
	rootB := partitionTable["ROOT-B"]

	// Compare first 64Kb so it checks hash, headers, preamble etc should be sufficient to check files are identical.
	limit := 64000
	match, err := comparePartitions(ctx, kernA.PartitionPath, kernB.PartitionPath, limit)
	if err != nil {
		return errors.Wrap(err, "comparing KERN-A and KERN-B failed")
	}

	copyFile := func(src, dst string) error {
		args := fmt.Sprintf("cat %s > %s", src, dst)
		if out, err := testexec.CommandContext(ctx, "sh", "-c", args).Output(testexec.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "failed to write %q to %q, got output: %v", src, dst, string(out))
		}
		return nil
	}
	testing.ContextLogf(ctx, "Currently in partition %q (label %q)", currKernel.PartitionPath, currKernel.Label)

	srcKern, dstKern := kernA, kernB
	srcRoot, dstRoot := rootA, rootB
	if copy := GetCopyFromLabel(currKernel.Label); copy == "B" {
		srcKern, dstKern = kernB, kernA
		srcRoot, dstRoot = rootB, rootA
	}

	if !match {
		testing.ContextLogf(ctx, "copying kernel from %q to %q", srcKern.Label, dstKern.Label)
		if err := copyFile(srcKern.PartitionPath, dstKern.PartitionPath); err != nil {
			return errors.Wrap(err, "failed to make kernel a and b identical")
		}

		testing.ContextLogf(ctx, "copying rootfs from %q to %q", srcRoot.Label, dstRoot.Label)
		if err := copyFile(srcRoot.PartitionPath, dstRoot.PartitionPath); err != nil {
			return errors.Wrap(err, "failed to make rootfs a and b identical")
		}

		if err := sha1sumsMatch(ctx, srcKern.PartitionPath, dstKern.PartitionPath); err != nil {
			return errors.Wrap(err, "Copying KERN failed unexpectedly")
		}

		if err := sha1sumsMatch(ctx, srcRoot.PartitionPath, dstRoot.PartitionPath); err != nil {
			return errors.Wrap(err, "Copying ROOT failed unexpectedly")
		}

	}

	// Set attributes of both copies to attributes of currently booted copy.
	if err := RestoreCgptAttributes(ctx, rootDevWithoutPart, map[string]*pb.CgptPartition{
		srcKern.Label: srcKern,
		dstKern.Label: srcKern,
		srcRoot.Label: srcRoot,
		dstRoot.Label: srcRoot,
	}); err != nil {
		return errors.Wrap(err, "failed to make cgpt attributes identical and force both partitions bootable")
	}

	return nil
}

func sha1sumsMatch(ctx context.Context, partitionPath1, partitionPath2 string) error {
	sha1sumCmd := testexec.CommandContext(ctx, "sha1sum", partitionPath1, partitionPath2)
	out, err := sha1sumCmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to get sha1sums")
	}
	/*
		Sample output from sha1sum looks like:
		$ sha1sum /dev/mmcblk1p4 /dev/mmcblk1p2
		699616b2a1e7d436e432f23d9db5c98647de22f8  /dev/mmcblk1p4
		e2d44a0de5edf38adb4e8e9611c48601b46484a8  /dev/mmcblk1p2
	*/
	lines := strings.Split(string(out), "\n")
	sum1 := strings.TrimSpace(strings.Split(lines[0], " ")[0])
	sum2 := strings.TrimSpace(strings.Split(lines[1], " ")[0])

	if sum1 != sum2 {
		return errors.Errorf("sha1sum for %s %s did not match, got %s and %s respectively", partitionPath1, partitionPath2, sum1, sum2)
	}
	testing.ContextLogf(ctx, "sha1sum for %s %s matched, got %s and %s respectively", partitionPath1, partitionPath2, sum1, sum2)
	return nil
}

// PrioritizeKernelCopy ensures DUT boots to expected kernel copy on next reboot (eg. KERN-A or KERN-B).
func PrioritizeKernelCopy(ctx context.Context, rootDevWithoutPart string, copy pb.PartitionCopy) error {
	partitionTable, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return errors.Wrap(err, "failed to read cgpt table")
	}

	testing.ContextLog(ctx, "Prioritizing partition copy ", CopyEnumToCopy[copy])
	label := PartitionNameCopyToLabel(pb.PartitionName_KERNEL, copy)
	targetTable := partitionTable[label]
	cmd := testexec.CommandContext(ctx, "cgpt", "prioritize", fmt.Sprintf("-i%d", targetTable.PartitionNumber), rootDevWithoutPart)
	if out, err := cmd.CombinedOutput(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to make prioritize kernel copy %q", label)
	} else if string(out) != "" {
		testing.ContextLog(ctx, "cgpt prioritize output: ", string(out))
	}

	newPartitionTable, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return errors.Wrap(err, "failed to get updated cgpt table")
	}

	otherLabel := PartitionNameCopyToLabel(pb.PartitionName_KERNEL, pb.PartitionCopy_B)
	if copy == pb.PartitionCopy_B {
		otherLabel = PartitionNameCopyToLabel(pb.PartitionName_KERNEL, pb.PartitionCopy_A)
	}

	targetTable = newPartitionTable[label]
	otherTable := newPartitionTable[otherLabel]

	getPriority := func(attrs []*pb.CgptPartitionAttribute) int32 {
		for _, attr := range attrs {
			if attr.Name == "priority" {
				return attr.Value
			}
		}
		return -1
	}

	targetPriority := getPriority(targetTable.Attrs)
	otherCopyPriority := getPriority(otherTable.Attrs)

	if otherCopyPriority >= targetPriority {
		return errors.Errorf("%s unexpectedly had lower priority %d than %s with priority %d", label, targetPriority, otherLabel, otherCopyPriority)
	}

	return nil
}

// SplitRootDevAndPart takes a rootdev path and splits the device and part num, inverse of RootDevPartitionPath.
func SplitRootDevAndPart(ctx context.Context, rootDevWithPart string) (string, int) {
	// Partition path looks like /dev/{device}p?{partition} with a 'p' between only if `device` ends with a digit.
	// e.g. /dev/mmcblk1p1 -> (/dev/mmcblk1, 1) and /dev/sda1 -> (/dev/sda, 1)
	match := regexp.MustCompile(`/dev/(?:(?:(\S+?\d)p)|(\S+[A-Za-z]))(\d+)$`).FindStringSubmatch(rootDevWithPart)
	part, _ := strconv.Atoi(match[len(match)-1]) // Get just the partition number for current kernel copy.
	var device string
	// The device could be in match[1] or match[2] depending on format so iterate and find the non-empty one.
	for _, str := range match[1:] {
		if str != "" {
			device = str
			break
		}
	}
	rootDevWithoutPart := "/dev/" + device

	return rootDevWithoutPart, part
}

// RootDevPartitionPath takes a device and partition number and joins them, inverse of SplitRootDevAndPart.
func RootDevPartitionPath(rootDevWithoutPart string, partitionNum int) string {
	lastChar := rootDevWithoutPart[len(rootDevWithoutPart)-1:]
	if _, err := strconv.Atoi(lastChar); err != nil {
		// if last char of rootdev is not number, don't need 'p' between device path and partition number.
		return fmt.Sprintf("%s%d", rootDevWithoutPart, partitionNum)
	}
	return fmt.Sprintf("%sp%d", rootDevWithoutPart, partitionNum)
}

// GetPartitionTable returns the partition table for the provided partition.
func GetPartitionTable(ctx context.Context, rootDevWithPart string) (*pb.CgptPartition, error) {
	rootDevWithoutPart, part := SplitRootDevAndPart(ctx, rootDevWithPart)
	partitionTable, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cgpt table")
	}

	for _, t := range partitionTable {
		if int(t.PartitionNumber) == part {
			return t, nil
		}
	}
	return nil, errors.Errorf("failed to find partition %d in cgpt table", part)
}

// BackupRootfsVerityHash saves the verity hash for given kernel from the corresponding rootfs partition.
func BackupRootfsVerityHash(ctx context.Context, rootDevWithoutPart string, partitionCopy pb.PartitionCopy) (int64, int64, string, error) {
	partitionTables, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return 0, 0, "", errors.Wrap(err, "failed to read cgpt table")
	}
	kernelLabel := PartitionNameCopyToLabel(pb.PartitionName_KERNEL, partitionCopy)
	rootfsLabel := PartitionNameCopyToLabel(pb.PartitionName_ROOTFS, partitionCopy)
	kernelTable := partitionTables[kernelLabel]
	rootfsTable := partitionTables[rootfsLabel]

	out, err := testexec.CommandContext(ctx, "vbutil_kernel", "--verify", kernelTable.PartitionPath, "--verbose").Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, 0, "", errors.Wrap(err, "failed to get vbutil kernel")
	}

	// TODO(tij@): There is another more explicit declaration of hash start block in the console output in vbutil_kernel cmd.
	// It would probably be a better idea to eventually parse for that instead as it's more explicit.
	dmRegex := regexp.MustCompile(`dm=\"(?:1 )?vroot none ro(?: 1)?,(0 (\d+) .+)\"`)
	match := dmRegex.FindSubmatch(out)
	if match == nil || len(match) < 3 {
		return 0, 0, "", errors.Errorf("failed to parse dm table for rootfs, got output: %v", string(out))
	}

	sectorSize, err := getSectorSize(ctx, kernelTable.PartitionPath)
	if err != nil {
		return 0, 0, "", errors.Wrap(err, "failed to get size of sectors")
	}

	hashStartBlock, _ := strconv.ParseInt(string(match[2]), 10, 64)
	// Multiplying by sector size converts from block count to bytes.
	hashStartBytes := hashStartBlock * sectorSize
	// The size / 4096 * 64 + 512 - calculation of the size of the merkle tree of dm-verity protected partition
	// (64 byte long hashes for each 4096 bytes off the partition plus the size of the intermediate layer of the tree).
	// TODO(tij@): Verify with dlunev@ this calculation makes sense and these numbers are not variable.
	hashSize := hashStartBytes/4096*64 + 512

	copy := CopyEnumToCopy[partitionCopy]
	backupPath, err := os.CreateTemp("/var/tmp", fmt.Sprintf("rootfsVerityHash%s_", copy))
	if err != nil {
		os.Remove(backupPath.Name())
		return 0, 0, "", errors.Wrap(err, "creating tmpfile for backing up verity hash")
	}

	args := []string{
		fmt.Sprintf("if=%s", rootfsTable.PartitionPath),
		fmt.Sprintf("of=%s", backupPath.Name()),
		fmt.Sprintf("skip=%d", hashStartBytes),
		fmt.Sprintf("count=%d", hashSize),
		"iflag=count_bytes,skip_bytes",
	}
	if err := testexec.CommandContext(ctx, "dd", args...).Run(testexec.DumpLogOnError); err != nil {
		os.Remove(backupPath.Name())
		return 0, 0, "", errors.Wrap(err, "failed to save rootfs verity hash to file")
	}

	testing.ContextLogf(ctx, "Rootfs verity hash for device %q saved at path: %v, has size %v", kernelTable.PartitionPath, backupPath.Name(), hashSize)

	return hashStartBytes, hashSize, backupPath.Name(), nil
}

// RestoreRootfsVerityHash restores saved verity hash for given kernel copy to rootfs partition from backup file.
func RestoreRootfsVerityHash(ctx context.Context, offset int64, backupPath, rootDevWithoutPart string, partitionCopy pb.PartitionCopy) error {
	partitionTables, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return errors.Wrap(err, "failed to get cgpt table")
	}

	// Uses to copy to build label for corresponding Rootfs label (e.g. A -> ROOT-A).
	rootfsLabel := PartitionNameCopyToLabel(pb.PartitionName_ROOTFS, partitionCopy)
	rootfsTable := partitionTables[rootfsLabel]

	testing.ContextLogf(ctx, "Restoring rootfs verity hash to %q from backup at %q ", rootfsTable.PartitionPath, backupPath)
	args := []string{
		fmt.Sprintf("if=%s", backupPath),
		fmt.Sprintf("of=%s", rootfsTable.PartitionPath),
		fmt.Sprintf("seek=%d", offset), // Go to offset in output file (rootfs partition)
		"bs=1M",
		"oflag=seek_bytes",
	}
	if err := testexec.CommandContext(ctx, "dd", args...).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to write saved verity hash to rootfs path")
	}
	return nil
}

// CorruptRootfsVerityHash corrupts verity hash for given kernel copy.
func CorruptRootfsVerityHash(ctx context.Context, offset, size int64, rootDevWithoutPart string, partitionCopy pb.PartitionCopy) error {
	partitionTables, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return errors.Wrap(err, "failed to get cgpt table")
	}

	// Uses to copy to build label for corresponding Rootfs label (e.g. A -> ROOT-A).
	rootfsLabel := PartitionNameCopyToLabel(pb.PartitionName_ROOTFS, partitionCopy)
	rootfsTable := partitionTables[rootfsLabel]

	testing.ContextLogf(ctx, "Corrupting rootfs verity hash on %q", rootfsTable.PartitionPath)
	args := []string{
		"if=/dev/zero",
		fmt.Sprintf("of=%s", rootfsTable.PartitionPath),
		fmt.Sprintf("seek=%d", offset), // Go to offset in output file (rootfs partition) where hash is stored.
		fmt.Sprintf("count=%d", size),
		"iflag=count_bytes",
		"oflag=seek_bytes",
	}
	if err := testexec.CommandContext(ctx, "dd", args...).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to write saved verity hash to rootfs path")
	}
	return nil
}

// GetCopyFromLabel returns the copy of the partition from the label, e.g. A from KERN-A or B from ROOT-B.
func GetCopyFromLabel(label string) string {
	// Example label: KERN-A -> A or ROOT-B -> B.
	match := regexp.MustCompile(`(?:\S+)-(\S+)`).FindStringSubmatch(label)
	if match == nil || len(match) < 2 {
		return ""
	}
	return match[1]
}

// GetNameFromLabel returns the name of the partition from the label, e.g. KERN from KERN-A or ROOT from ROOT-B.
func GetNameFromLabel(label string) (string, error) {
	// Example label: KERN-A -> KERN or ROOT-B -> ROOT.
	match := regexp.MustCompile(`(\S+)-(?:\S+)`).FindStringSubmatch(label)
	if match == nil || len(match) < 2 {
		return "", errors.Errorf("label %q isn't in expected format", label)
	}
	return match[1], nil
}

// GetKernelVersion uses vbutil_kernel to get the kernel version for a given partition.
func GetKernelVersion(ctx context.Context, rootDevWithoutPart string, copy pb.PartitionCopy) (string, error) {
	partitionTables, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return "", errors.Wrap(err, "failed to get cgpt table")
	}

	label := PartitionNameCopyToLabel(pb.PartitionName_KERNEL, copy)
	table := partitionTables[label]

	testing.ContextLogf(ctx, "Getting kernel version for %s (label: %s)", table.PartitionPath, table.Label)
	// TODO(tij@): Update this to use the futility vbutil_kernel library after it gets implemented.
	out, err := testexec.CommandContext(ctx, "vbutil_kernel", "--verify", table.PartitionPath).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to get vbutil kernel")
	}

	match := regexp.MustCompile(`Kernel version:\s*(\S+)`).FindStringSubmatch(string(out))
	if match == nil || len(match) < 2 {
		return "", errors.Errorf("failed to parse kernel version for label %q, got output: %v", table.Label, string(out))
	}

	return match[1], nil
}

// SetKernelVersion uses vbutil_kernel to set the kernel version for a given partition.
func SetKernelVersion(ctx context.Context, rootDevWithoutPart string, copy pb.PartitionCopy, version string) error {
	partitionTables, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return errors.Wrap(err, "failed to get cgpt table")
	}

	label := PartitionNameCopyToLabel(pb.PartitionName_KERNEL, copy)
	table := partitionTables[label]

	tmpFile, err := os.CreateTemp("/var/tmp", fmt.Sprintf("%s-repack_*.bin", table.Label))
	if err != nil {
		os.Remove(tmpFile.Name())
		return errors.Wrap(err, "creating tmpfile for storing modified kernel with new version")
	}
	defer os.Remove(tmpFile.Name())

	args := []string{
		"--repack", tmpFile.Name(),
		"--oldblob", table.PartitionPath,
		"--signprivate", KernelPrivateKeyPath,
		"--keyblock", KernelKeyblockPath,
		"--version", version,
	}
	// TODO(tij@): Update this to use the futility vbutil_kernel library after it gets implemented.
	out, err := testexec.CommandContext(ctx, "vbutil_kernel", args...).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "failed to load repack kernel from %s with version %s: %s", table.Label, version, string(out))
	}

	args = []string{
		fmt.Sprintf("if=%s", tmpFile.Name()),
		fmt.Sprintf("of=%s", table.PartitionPath),
		"conv=sync",
	}
	if err := testexec.CommandContext(ctx, "dd", args...).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to write new kernel")
	}

	return nil
}

// SetKernelHeaderMagic sets the kernel header magic for provided partition table.
func SetKernelHeaderMagic(ctx context.Context, rootDevWithoutPart, label string, magic HeaderMagic, forceBoot bool) error {
	partitionTables, err := GetCgptTable(ctx, rootDevWithoutPart)
	if err != nil {
		return errors.Wrap(err, "failed to get cgpt table")
	}
	table := partitionTables[label]

	testing.ContextLogf(ctx, "Setting header to %s on device %s (label %q)", magic, table.PartitionPath, table.Label)
	args := []string{
		fmt.Sprintf("of=%s", table.PartitionPath),
		"conv=notrunc,nocreat",
		"oflag=sync",
	}
	cmd := testexec.CommandContext(ctx, "dd", args...)
	cmd.Stdin = strings.NewReader(string(magic))

	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed setting header magic for partition %s (%s) to %s", table.PartitionPath, table.Label, string(magic))
	}

	if forceBoot {
		rootDevWithoutPart, _ := SplitRootDevAndPart(ctx, table.PartitionPath)
		// If restoring after corruption it might be necessary to reset cgpt attributes to make restored kernel bootable again.
		if err := forcePartitionBootable(ctx, table.PartitionPath, 1); err != nil {
			testing.ContextLogf(ctx, "Failed to reset cgpt attributes for %q: %v", table.Label, err)
		}
		if err := PrioritizeKernelCopy(ctx, rootDevWithoutPart, CopyToCopyEnum[GetCopyFromLabel(table.Label)]); err != nil {
			testing.ContextLogf(ctx, "Failed to make %q bootable after restoring header magic, got error: %v", table.Label, err)
		}
	}
	return nil
}

// comparePartitions compares two files at paths up to n bytes and returns true if they're identical.
func comparePartitions(ctx context.Context, pathA, pathB string, n int) (bool, error) {
	if err := testexec.CommandContext(ctx, "cmp", "-n", strconv.Itoa(n), pathA, pathB).Run(ssh.DumpLogOnError); err != nil {
		// Cmp error code 0 == files match, 1 == files differ, 2 == error in running cmp.
		if errCode, ok := testexec.ExitCode(err); !ok || errCode == 2 {
			return false, errors.Wrapf(err, "failed to compare %q and %q using 'cmp'", pathA, pathB)
		}
		// If error code == 1, then files differ.
		return false, nil
	}
	return true, nil
}

func getSectorSize(ctx context.Context, rootDevWithPart string) (int64, error) {
	out, err := testexec.CommandContext(ctx, "fdisk", "-l", rootDevWithPart).Output(testexec.DumpLogOnError)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to get fdisk output for disk %q", rootDevWithPart)
	}

	// Example output: "Units: sectors of 1 * 512 = 512 bytes".
	sizePattern := regexp.MustCompile(`Units: sectors of (\d+) \* (\d+) = (\d+) bytes`)
	match := sizePattern.FindStringSubmatch(string(out))
	if match == nil {
		return -1, errors.Errorf("failed to get size of sectors, got output: %v", string(out))
	}

	return strconv.ParseInt(match[3], 10, 64)
}

func forcePartitionBootable(ctx context.Context, rootDevWithPart string, priority int) error {
	rootDevWithoutPart, part := SplitRootDevAndPart(ctx, rootDevWithPart)
	args := []string{
		"add",
		fmt.Sprintf("-i%d", part),
		fmt.Sprintf("-P%d", priority),
		"-S1", "-T0", rootDevWithoutPart,
	}
	cmd := testexec.CommandContext(ctx, "cgpt", args...)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to make %s bootable", rootDevWithPart)
	}
	return nil
}
