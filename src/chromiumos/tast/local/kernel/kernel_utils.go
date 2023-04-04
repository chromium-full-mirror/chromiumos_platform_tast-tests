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
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/sysutil"
	pb "chromiumos/tast/services/cros/firmware"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
)

const (
	// KernelPrivateKeyPath is the path to private key for kernel.
	KernelPrivateKeyPath string = "/usr/share/vboot/devkeys/kernel_data_key.vbprivk"
	// KernelKeyblockPath is the path to kernel keyblock.
	KernelKeyblockPath string = "/usr/share/vboot/devkeys/kernel.keyblock"
)

// LabelEnumToLabel maps the PartitionLabel enum to the partition label name from cgpt table.
var LabelEnumToLabel = map[pb.PartitionLabel]string{
	pb.PartitionLabel_KERNEL_A: "KERN-A",
	pb.PartitionLabel_KERNEL_B: "KERN-B",
	pb.PartitionLabel_MINIOS_A: "MINIOS-A",
	pb.PartitionLabel_MINIOS_B: "MINIOS-B",
	pb.PartitionLabel_ROOTFS_A: "ROOT-A",
	pb.PartitionLabel_ROOTFS_B: "ROOT-B",
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
	configs, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config")
	}
	return configs, nil
}

// GetCgptTable returns structure containing metadata with CGPT partitions.
func GetCgptTable(ctx context.Context, blockdev string) (map[string]*pb.CgptPartition, error) {
	cgptOut, err := testexec.CommandContext(ctx, "cgpt", "show", blockdev).Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve cgpt table")
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

		var partitionPath string
		lastChar := blockdev[len(blockdev)-1:]
		if _, err := strconv.Atoi(lastChar); err != nil {
			// if last char is not number, don't need 'p' between device path and partition number.
			partitionPath = fmt.Sprintf("%s%d", blockdev, partitionNumber)
		} else {
			partitionPath = fmt.Sprintf("%sp%d", blockdev, partitionNumber)
		}

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

// BackupPartition backs up partition and saves to a file.
func BackupPartition(ctx context.Context, rootDev string, partLabel pb.PartitionLabel) (*pb.CgptPartition, string, error) {
	partition := LabelEnumToLabel[partLabel]

	partitionTables, err := GetCgptTable(ctx, rootDev)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to get cgpt table")
	}

	// Look for table with expected label (eg. KERN-A, MINIOS-B, ROOT-A).
	table := partitionTables[partition]
	testing.ContextLogf(ctx, "Partition %s saved at path: %v has size %v", partition, table.PartitionPath, table.Size)

	backupPath, err := ioutil.TempFile("/usr/local/share/tast", fmt.Sprintf("%s_", partition))
	if err != nil {
		os.Remove(backupPath.Name())
		return nil, "", errors.Wrapf(err, "creating tmpfile for backing up partition %s", partition)
	}

	cmd := fmt.Sprintf("cat %s > %s", table.PartitionPath, backupPath.Name())
	if err := testexec.CommandContext(ctx, "sh", "-c", cmd).Run(testexec.DumpLogOnError); err != nil {
		os.Remove(backupPath.Name())
		return nil, "", errors.Wrap(err, "failed to save partition to file")
	}

	return table, backupPath.Name(), nil
}

// RestorePartition restores a partition from backup file. Leaves backup file for users to delete.
func RestorePartition(ctx context.Context, backupPath, partitionPath string) error {
	testing.ContextLogf(ctx, "Restoring partition at %q from backup at %q ", partitionPath, backupPath)
	cmd := fmt.Sprintf("cat %s > %s", backupPath, partitionPath)
	if err := testexec.CommandContext(ctx, "sh", "-c", cmd).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to restore kernel from backup file")
	}
	return nil
}

// PrioritizeKernelCopy makes both kernel copies (KERN-A and KERN-B) identical and ensures DUT boots to expected kernel copy on next reboot.
func PrioritizeKernelCopy(ctx context.Context, targetKernelLabel pb.PartitionLabel) error {
	currKernel, err := GetCurrentKernel(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current cgpt table for current kernel copy")
	}

	rootDev, err := GetCurrentRootDevice(ctx, false)
	if err != nil {
		return errors.Wrap(err, "failed to get root device")
	}

	partitionTable, err := GetCgptTable(ctx, rootDev)
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

	if !match {
		srcKern, dstKern := kernA, kernB
		srcRoot, dstRoot := rootA, rootB
		if copy, _ := GetCopyFromLabel(currKernel.Label); copy == "B" {
			srcKern, dstKern = kernB, kernA
			srcRoot, dstRoot = rootB, rootA
		}

		testing.ContextLogf(ctx, "copying kernel from %q to %q", srcKern.Label, dstKern.Label)
		if err := copyFile(srcKern.PartitionPath, dstKern.PartitionPath); err != nil {
			return errors.Wrap(err, "failed to make kernel a and b identical")
		}

		testing.ContextLogf(ctx, "copying rootfs from %q to %q", srcRoot.Label, dstRoot.Label)
		if err := copyFile(srcRoot.PartitionPath, dstRoot.PartitionPath); err != nil {
			return errors.Wrap(err, "failed to make rootfs a and b identical")
		}
	}

	cmd := testexec.CommandContext(ctx, "cgpt", "add", fmt.Sprintf("-i%d", kernA.PartitionNumber), "-P1", "-S1", "-T0", rootDev)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to make KERN-A bootable")
	}

	cmd = testexec.CommandContext(ctx, "cgpt", "add", fmt.Sprintf("-i%d", kernB.PartitionNumber), "-P2", "-S1", "-T0", rootDev)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to make KERN-B bootable")
	}

	testing.ContextLog(ctx, "Prioritizing partition ", LabelEnumToLabel[targetKernelLabel])
	targetTable := partitionTable[LabelEnumToLabel[targetKernelLabel]]
	cmd = testexec.CommandContext(ctx, "cgpt", "prioritize", fmt.Sprintf("-i%d", targetTable.PartitionNumber), rootDev)
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to make prioritize kernel copy %q", LabelEnumToLabel[targetKernelLabel])
	}

	return nil
}

// GetCurrentKernel returns the partition table for the kernel copy currently being used.
func GetCurrentKernel(ctx context.Context) (*pb.CgptPartition, error) {
	rootDevWithPart, err := GetCurrentRootDevice(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get root device with part")
	}

	// Partition path looks like /dev/{device}p?{partition} with a 'p' between only if `device` ends with a digit.
	partStr := regexp.MustCompile(`/dev/\S+?(\d+)$`).FindStringSubmatch(rootDevWithPart)
	part, _ := strconv.Atoi(partStr[1]) // Get just the partition number for current kernel copy.

	rootDev, err := GetCurrentRootDevice(ctx, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get root device")
	}

	partitionTable, err := GetCgptTable(ctx, rootDev)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cgpt table")
	}

	var currKernel *pb.CgptPartition
	for _, t := range partitionTable {
		if int(t.PartitionNumber) == part {
			currKernel = t
		}
	}
	return currKernel, nil
}

// BackupRootfsVerityHash saves the verity hash for given kernel from the corresponding rootfs partition.
func BackupRootfsVerityHash(ctx context.Context, rootDev string, table *pb.CgptPartition) (int, int, string, error) {
	out, err := testexec.CommandContext(ctx, "vbutil_kernel", "--verify", table.PartitionPath, "--verbose").Output(testexec.DumpLogOnError)
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

	sectorSize, err := getSectorSize(ctx, table.PartitionPath)
	if err != nil {
		return 0, 0, "", errors.Wrap(err, "failed to get size of sectors")
	}

	hashStartBlock, _ := strconv.Atoi(string(match[2]))
	// Multiplying by sector size converts from block count to bytes.
	hashStartBytes := hashStartBlock * sectorSize
	// The size / 4096 * 64 + 512 - calculation of the size of the merkle tree of dm-verity protected partition
	// (64 byte long hashes for each 4096 bytes off the partition plus the size of the intermediate layer of the tree).
	// TODO(tij@): Verify with dlunev@ this calculation makes sense and these numbers are not variable.
	hashSize := hashStartBytes/4096*64 + 512

	partitionTables, err := GetCgptTable(ctx, rootDev)
	if err != nil {
		return 0, 0, "", errors.Wrap(err, "failed to get cgpt table")
	}

	section, err := GetCopyFromLabel(table.Label)
	if err != nil {
		return 0, 0, "", errors.Wrap(err, "failed to get partition label copy")
	}
	rootfsLabel := fmt.Sprintf("ROOT-%s", section)
	rootfsTable := partitionTables[rootfsLabel]

	backupPath, err := ioutil.TempFile("/usr/local/share/tast", fmt.Sprintf("rootfsVerityHash%s_", table.Label))
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

	testing.ContextLogf(ctx, "Rootfs verity hash saved at path: %v has size %v", backupPath.Name(), hashSize)

	return hashStartBytes, hashSize, backupPath.Name(), nil
}

// RestoreRootfsVerityHash restores saved verity hash for given kernel copy to rootfs partition from backup file.
func RestoreRootfsVerityHash(ctx context.Context, offset int64, backupPath, rootDev string, label pb.PartitionLabel) error {
	partitionTables, err := GetCgptTable(ctx, rootDev)
	if err != nil {
		return errors.Wrap(err, "failed to get cgpt table")
	}

	section, err := GetCopyFromLabel(LabelEnumToLabel[label])
	if err != nil {
		return errors.Wrap(err, "failed to get partition label copy")
	}
	rootfsLabel := fmt.Sprintf("ROOT-%s", section)
	rootfsTable := partitionTables[rootfsLabel]

	testing.ContextLogf(ctx, "Restoring rootfs verity hash from backup at %q ", backupPath)
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
func CorruptRootfsVerityHash(ctx context.Context, offset, size int64, rootDev string, label pb.PartitionLabel) error {
	partitionTables, err := GetCgptTable(ctx, rootDev)
	if err != nil {
		return errors.Wrap(err, "failed to get cgpt table")
	}

	section, err := GetCopyFromLabel(LabelEnumToLabel[label])
	if err != nil {
		return errors.Wrap(err, "failed to get partition label copy")
	}
	rootfsLabel := fmt.Sprintf("ROOT-%s", section)
	rootfsTable := partitionTables[rootfsLabel]

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
func GetCopyFromLabel(label string) (string, error) {
	// Example label: KERN-A -> A or ROOT-B -> B.
	match := regexp.MustCompile(`(?:\S+)-(\S+)`).FindStringSubmatch(label)
	if match == nil || len(match) < 2 {
		return "", errors.Errorf("label %q doesn't inlcude a specific section", label)
	}
	return match[1], nil
}

// GetKernelVersion uses vbutil_kernel to get the kernel version for a given partition.
func GetKernelVersion(ctx context.Context, rootDev string, label pb.PartitionLabel) (string, *pb.CgptPartition, error) {
	partitionTables, err := GetCgptTable(ctx, rootDev)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to get cgpt table")
	}

	table := partitionTables[LabelEnumToLabel[label]]

	out, err := testexec.CommandContext(ctx, "vbutil_kernel", "--verify", table.PartitionPath).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to get vbutil kernel")
	}

	match := regexp.MustCompile(`Kernel version:\s*(\S+)`).FindStringSubmatch(string(out))
	if match == nil || len(match) < 2 {
		return "", nil, errors.Errorf("failed to parse kernel version for label %q, got output: %v", table.Label, string(out))
	}

	return match[1], table, nil
}

// SetKernelVersion uses vbutil_kernel to set the kernel version for a given partition.
func SetKernelVersion(ctx context.Context, table *pb.CgptPartition, version string) error {
	tmpFile, err := ioutil.TempFile("/usr/local/share/tast", fmt.Sprintf("%s-repack_*.bin", table.Label))
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

func getSectorSize(ctx context.Context, rootDev string) (int, error) {
	out, err := testexec.CommandContext(ctx, "fdisk", "-l", rootDev).Output(testexec.DumpLogOnError)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to get fdisk output for disk %q", rootDev)
	}

	// Example output: "Units: sectors of 1 * 512 = 512 bytes".
	sizePattern := regexp.MustCompile(`Units: sectors of (\d+) \* (\d+) = (\d+) bytes`)
	match := sizePattern.FindStringSubmatch(string(out))
	if match == nil {
		return -1, errors.Errorf("failed to get size of sectors, got output: %v", string(out))
	}

	return strconv.Atoi(match[3])
}

func rootDevPartitionPath(device string, partitionNum int) string {
	lastChar := device[len(device)-1:]
	if _, err := strconv.Atoi(lastChar); err != nil {
		// if last char of device is not number, don't need 'p' between device path and partition number.
		return fmt.Sprintf("%s%d", device, partitionNum)
	}
	return fmt.Sprintf("%sp%d", device, partitionNum)
}
