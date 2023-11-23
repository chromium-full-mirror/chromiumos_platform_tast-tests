// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ureadahead

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// MinHostPackSizeKB is set from normally generated host ureadahead pack being >300MB of data.
	MinHostPackSizeKB = 300 * 1024

	// MinGuestPackSizeKB is set from lab data showing guest ureadahead pack could be smaller than host.
	MinGuestPackSizeKB = 200 * 1024
)

// DumpHostPack dumps the ureadahead pack in the host OS. The pack path can be
// different between production and non-production test environments.
func DumpHostPack(ctx context.Context, packPath, logPath string) error {
	if _, err := os.Stat(packPath); err != nil {
		if !os.IsNotExist(err) {
			errors.Wrapf(err, "failed to check ureadahead pack exists %s", packPath)
		}
		return errors.Wrapf(err, "expected ureadahead pack %s does not exist", packPath)
	}

	cmd := testexec.CommandContext(ctx, "/sbin/ureadahead", "--dump", "--verbose", packPath)

	logFile, err := os.Create(logPath)
	if err != nil {
		return errors.Wrap(err, "failed to create log file")
	}
	cmd.Stdout = logFile
	// Don't set cmd.Stderr, so it goes to default log buffer
	// and DumpLogOnError can dump it.
	err = cmd.Run(testexec.DumpLogOnError)
	logFile.Close()

	if err != nil {
		return errors.Wrapf(err, "failed to get the ureadahead stats %s", packPath)
	}

	// Verify the host pack file dump.
	logFile, err = os.Open(logPath)
	if err != nil {
		return errors.Wrap(err, "failed to open log file")
	}
	defer logFile.Close()
	return nil
}

// DumpGuestPack dumps the ureadahead pack in the guest OS. Guest ureadahead pack is
// stored in a tmpfs mount so the existing ARC instance needs to be passed in
// non-production test environment. Otherwise, pack will disappear after reboot.
func DumpGuestPack(ctx context.Context, a *arc.ARC, logPath string) error {
	// File path for ureadahead pack in the quest OS.
	const ureadaheadDataDir = "/var/lib/ureadahead"

	// Check for existence of newly generated pack file on guest side.
	srcPath := filepath.Join(ureadaheadDataDir, "pack")
	if _, err := a.FileSize(ctx, srcPath); err != nil {
		return errors.Wrap(err, "failed to ensure pack file exists")
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		return errors.Wrap(err, "failed to create log file")
	}
	defer logFile.Close()

	// TODO(b:312323135): Use ureadahead.sh only after rolling to
	// ureadahead prebuilt.
	bins := []string{"/system/bin/ureadahead", "/system/bin/ureadahead.sh"}
	for _, bin := range bins {
		if a.Command(ctx, "/system/bin/stat", bin).Run() != nil {
			continue
		}
		// Capture stdout into log file.
		cmd := a.Command(ctx, bin, "--dump", "--verbose")
		cmd.Stdout = logFile
		return cmd.Run(testexec.DumpLogOnError)
	}

	return errors.New("ureadahead is not found")

}

// CheckPackFileDump verifies the validity of the generated pack file using
// ureadahead's own dump functionality.
func CheckPackFileDump(ctx context.Context, logPath string, minPackSize int) error {
	logFile, err := os.Open(logPath)
	if err != nil {
		return errors.Wrap(err, "failed to open log file")
	}
	defer logFile.Close()

	// Example output:
	// <text>
	// 0 inode groups, 1638 files, 2350 blocks (805408 kB)
	// <text>
	re := regexp.MustCompile(`^(\d+) inode groups, (\d+) files, (\d+) blocks \((\d+) kB\)$`)
	scanner := bufio.NewScanner(logFile)

	matchFound := false
	sizeKB := -1
	for scanner.Scan() {
		str := scanner.Text()
		result := re.FindStringSubmatch(str)
		if result == nil {
			continue
		}
		if matchFound {
			return errors.Wrapf(err, "failed with more than 1 match found. Last match: %q", str)
		}

		// Parsing (\d+) group that represents number of Kb handled by this ureadahead pack
		sizeKB, err = strconv.Atoi(result[4])
		if err != nil {
			return errors.Wrapf(err, "failed to parse group %q from %q", result[4], str)
		}
		matchFound = true
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to read log file")
	}

	if !matchFound {
		return errors.Wrap(err, "failed to parse ureadahead pack dump")
	}

	testing.ContextLogf(ctx, "Found ureadahead pack at %s with size of %d kB", logPath, sizeKB)
	if sizeKB < minPackSize {
		return errors.Errorf("failed due to pack size %d kB too small. It is expected to be min %d kB", sizeKB, minPackSize)
	}

	return nil
}

// CheckHostCompatibility verifies host ureadahead is compatible for use in guest.
func CheckHostCompatibility(ctx context.Context, outDir string) error {
	const (
		// Must use BootstrapCommand since exec-cros-binary is not accessible by adb.
		cmd               = "/system/bin/exec-cros-binary"
		crosUreadaheadBin = "/var/run/arc/sbin/ureadahead"
		logName           = "cros_ureadahead_check.log"
	)

	logPath := filepath.Join(outDir, logName)
	logFile, err := os.Create(logPath)
	if err != nil {
		return errors.Wrap(err, "failed to create log file")
	}
	defer logFile.Close()

	// Verify generate mode can run to completion with timeout since otherwise ureadahead process doesn't complete.
	outGenerate, err := arc.BootstrapCommand(ctx, cmd, crosUreadaheadBin, "--force-trace", "--use-existing-trace-events",
		"--force-ssd-mode", "--verbose", "--timeout=5").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to run CrOS ureadahead trace in guest OS")
	}
	generateStr := string(outGenerate[:])
	// Example output:
	// Counted 12 CPUs
	// <text>
	reGenerate := regexp.MustCompile(`Counted (\d+) CPUs`)
	if _, err = logFile.WriteString(generateStr + "\n\n"); err != nil {
		return errors.Wrap(err, "failed to write file")
	}
	if matched := reGenerate.MatchString(generateStr); !matched {
		return errors.Errorf("failed to verify compatibility in generate mode, see %s for details", logPath)
	}

	// Verify readahead mode can run to completion.
	outReadahead, err := arc.BootstrapCommand(ctx, cmd, crosUreadaheadBin, "--use-existing-trace-events", "--verbose").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to run CrOS ureadahead pack read in guest OS")
	}
	readaheadStr := string(outReadahead[:])
	// Example output:
	// <text>
	// Read pack: 0.000s
	// Readahead: 0.004s
	reReadahead := regexp.MustCompile(`Read pack:*(.+)[\r\n]Readahead:*(.*)`)
	if _, err = logFile.WriteString(readaheadStr + "\n"); err != nil {
		return errors.Wrap(err, "failed to write file")
	}
	if matched := reReadahead.MatchString(readaheadStr); !matched {
		return errors.Errorf("failed to verify compatibility in readahead mode, see %s for details", logPath)
	}
	return nil
}
