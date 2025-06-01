// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/local/firmware"
	"go.chromium.org/tast-tests/cros/local/spaced"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Spaced,
		Desc: "Checks that spaced queries work",
		Contacts: []string{
			"chromeos-storage@google.com",
			"sarthakkukreti@chromium.org",
		},
		BugComponent: "b:974567",
		Attr:         []string{"group:mainline", "informational"},
	})
}

// sysfsRootDeviceSize fetches the root device size from /sys/block/<dev>/size.
func sysfsRootDeviceSize(ctx context.Context) (int64, error) {
	// Check actual root device size.
	rootdev, err := firmware.RootDevice(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to fetch root device")
	}

	fp := fmt.Sprintf("/sys/block/%s/size", filepath.Base(rootdev))
	content, err := os.ReadFile(fp)
	if err != nil {
		return 0, errors.Wrapf(err, "reading filepath %s", fp)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(content)), 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse root device size as int64")
	}

	// Size is in sectors; return in bytes.
	return size * 512, nil
}

// statFreeDiskSpace gets the free space on the filesystem using statfs().
func statFreeDiskSpace(ctx context.Context, path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, errors.Wrapf(err, "failed to get disk stats for %s", path)
	}

	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// statTotalDiskSpace gets the total space on the filesystem using statfs().
func statTotalDiskSpace(ctx context.Context, path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, errors.Wrapf(err, "failed to get disk stats for %s", path)
	}

	return int64(stat.Blocks) * int64(stat.Bsize), nil
}

// parseDiskIOStatsForPathsPrettyPrintResponse Parses a response from
// spaced.DiskIOStatsForPathsPrettyPrint() which should look like:
// <empty line>
// Disk I/O stats for <path>:
// Read Merges: 0
// Read Sectors: 3139112
// Read Ticks: 19255
// Writes IOs: 0
// Write Merges: 0
// Write Sectors: 0
// Write Ticks: 0
// In Flight: 0
// IO Ticks: 6790
// Time In Queue: 19255
// Discard IOs: 0
// Discard Merges: 0
// Discard Sectors: 0
// Discard Ticks: 0
// Flush IOs: 0
// Flush Ticks: 0
// <empty line>
func parseDiskIOStatsForPathsPrettyPrintResponse(dir, response string) error {
	scanner := bufio.NewScanner(strings.NewReader(response))
	// Skip an empty line.
	scanner.Scan()
	// Next, grab and validate the opening line.
	scanner.Scan()
	openingLine := scanner.Text()
	expectedOpeningLine := "Disk I/O stats for " + dir + ":"
	if openingLine != expectedOpeningLine {
		return errors.Errorf("invalid opening line in response from spaced.DiskIOStatsForPathsPrettyPrint: %s", openingLine)
	}
	count := 0
	// Inspect each non-empty line, ensuring a key/value pair holding a valid value.
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			return errors.Errorf("badly formatted line in response from spaced.DiskIOStatsForPathsPrettyPrint: %s", line)
		}
		keyStr := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])
		_, err := strconv.Atoi(valueStr)
		if err != nil {
			return errors.Errorf("invalid value found in response from spaced.DiskIOStatsForPathsPrettyPrint: %s:%s", keyStr, valueStr)
		}
		count++
	}
	// There should be exactly 17 key/value pairs in the response.
	if count != 17 {
		return errors.Errorf("incorrect number of key/value pairs in response from spaced.DiskIOStatsForPathsPrettyPrint: %s", response)
	}
	return nil
}

// parseDiskIOStats parses a response from spaced.DiskIOStats() which should look like:
// <empty line>
// I/O stats for all block devices:
//
//	254      16 dm-16 595 0 54272 997 10217 0 81736 243477 0 267 244474 0 0 0 0 0 0
//	254      17 dm-17 18 0 2304 0 0 0 0 0 0 0 0 0 0 0 0 0 0
//	  7       9 loop9 333 100 32000 201 0 0 0 0 0 201 201 0 0 0 0 0 0
//
// ...
// <empty line>
func parseDiskIOStats(response string) error {
	scanner := bufio.NewScanner(strings.NewReader(response))
	// Skip an empty line.
	scanner.Scan()
	// Next, grab and validate the opening line.
	scanner.Scan()
	openingLine := scanner.Text()
	expectedOpeningLine := "I/O stats for all block devices:"
	if openingLine != expectedOpeningLine {
		return errors.Errorf("invalid opening line in response from spaced.DiskIOStats: %s", openingLine)
	}
	count := 0
	// Inspect each non-empty line, ensuring the correct number of fields and type of each field.
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 20 {
			return errors.Errorf("badly formatted line in response from spaced.DiskIOStats: %s", line)
		}
		for i := 0; i < len(parts); i++ {
			if i == 2 {
				if unicode.IsDigit(rune(parts[i][0])) {
					return errors.Errorf("invalid device name in response from spaced.DiskIOStats: %s", line)
				}
				continue
			}
			valueStr := strings.TrimSpace(parts[i])
			_, err := strconv.Atoi(valueStr)
			if err != nil {
				return errors.Errorf("invalid value found in response from spaced.DiskIOStats: %s", valueStr)
			}
		}
		count++
	}
	// There should be at least one non-empty, valid, line in the response.
	if count == 0 {
		return errors.Errorf("found no valid entries in response from spaced.DiskIOStats: %s", response)
	}
	return nil
}

func Spaced(ctx context.Context, s *testing.State) {
	const (
		// Path to check disk space queries on.
		statefulMount = "/mnt/stateful_partition/"
		// Disk space margin to consider when comparing against expected values.
		spaceMarginBytes = 100 * 1024 * 1024
	)

	spaced, err := spaced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create spaced client: ", err)
	}

	// Check D-Bus queries.
	rootDeviceSize, err := spaced.RootDeviceSize(ctx)
	if err != nil {
		s.Fatal("Failed to query root device size: ", err)
	}

	expectedRootDeviceSize, err := sysfsRootDeviceSize(ctx)
	if err != nil {
		s.Fatal("Failed to get actual root device size: ", err)
	}

	if rootDeviceSize != expectedRootDeviceSize {
		s.Fatalf("Invalid root device size: got %d, want: 0 < size < %d", rootDeviceSize, expectedRootDeviceSize)
	}

	freeDiskSpace, err := spaced.FreeDiskSpace(ctx, statefulMount)
	if err != nil {
		s.Fatal("Failed to query free disk space: ", err)
	}

	expectedFreeDiskSpace, err := statFreeDiskSpace(ctx, statefulMount)
	if err != nil {
		s.Fatal("Failed to get expected free disk space: ", err)
	}

	if freeDiskSpace <= 0 || freeDiskSpace > expectedFreeDiskSpace+spaceMarginBytes {
		s.Fatalf("Invalid free disk space;  got %d, want: 0 < size < %d", freeDiskSpace, expectedFreeDiskSpace+spaceMarginBytes)
	}

	totalDiskSpace, err := spaced.TotalDiskSpace(ctx, statefulMount)
	if err != nil {
		s.Fatal("Failed to query total disk space: ", err)
	}

	expectedTotalDiskSpace, err := statTotalDiskSpace(ctx, statefulMount)
	if err != nil {
		s.Fatal("Failed to get expected total disk space: ", err)
	}

	if totalDiskSpace <= 0 || totalDiskSpace > expectedTotalDiskSpace+spaceMarginBytes {
		s.Fatalf("Invalid total disk space;  got %d, want: 0 < size < %d", totalDiskSpace, expectedTotalDiskSpace+spaceMarginBytes)
	}

	response, err := spaced.DiskIOStatsForPathsPrettyPrint(ctx, "/")
	if err != nil {
		s.Fatal("Failed to get disk I/O stats for /: ", err)
	}
	err = parseDiskIOStatsForPathsPrettyPrintResponse("/", response)
	if err != nil {
		s.Fatal("Error parsing response from spaced.DiskIOStatsForPathsPrettyPrint: ", err)
	}

	response, err = spaced.DiskIOStats(ctx)
	if err != nil {
		s.Fatal("Failed to get disk I/O stats for all block devices: ", err)
	}
	err = parseDiskIOStats(response)
	if err != nil {
		s.Fatal("Error parsing response from spaced.DiskIOStats: ", err)
	}
}
