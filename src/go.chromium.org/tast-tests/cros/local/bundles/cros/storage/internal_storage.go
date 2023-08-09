// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"io/ioutil"
	"strings"

	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {

	testing.AddTest(&testing.Test{
		Func: InternalStorage,
		Desc: "Internal storage tests",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational"},
	})
}

const (
	statefulPartition          string = "/mnt/stateful_partition"
	statefulPartitionEncrypted string = "/mnt/stateful_partition/encrypted"
)

// InternalStorage runs the internal storage checks like
// checking stateful file systems, stateful partition free space etc.
func InternalStorage(ctx context.Context, s *testing.State) {
	// Check if stateful partitions are writable
	checkStatefulPartitionsWritable(ctx, s)

	// Check for known kernel issues
	checkKernelErrors(ctx, s)
}

// checkStatefulPartitionsWritable checks that the stateful partitions are writable
func checkStatefulPartitionsWritable(ctx context.Context, s *testing.State) {
	for _, dir := range []string{
		statefulPartition,
		statefulPartitionEncrypted,
	} {
		checkPathExists(ctx, s, dir)
		fp := filepath.Join(dir, ".tast.check-disk")
		if err := ioutil.WriteFile(fp, nil, 0600); err != nil {
			s.Fatalf("%s is not writable: %s", dir, err)
		}
		if err := os.Remove(fp); err != nil {
			s.Fatalf("%s is not writable: %s", dir, err)
		}
	}
}

// checkKernelErrors checks if any known kernel issues are present
func checkKernelErrors(ctx context.Context, s *testing.State) {
	command := `dmesg | grep -E "EXT4-fs error \(device $(cut -d ' ' -f 5,9 /proc/$$/mountinfo | grep -e '^/mnt/stateful_partition ' | cut -d ' ' -f 2 | cut -d '/' -f 3)\):"`
	cmd := testexec.CommandContext(ctx, "sh", "-c", command)
	out, _ := cmd.CombinedOutput()
	if string(out) != "" {
		sample := strings.Split(string(out), `\n`)[0]
		// Log the first file system error.
		s.Fatalf("first file system error: %q", sample)
	}
	// Check for other critical FS errors.
	command = `dmesg | grep "This should not happen!!  Data will be lost"`
	out, _ = testexec.CommandContext(ctx, "sh", "-c", command).Output()
	if string(out) != "" {
		s.Fatalf("critical kernel error: %s. Saw file system error: Data will be lost", out)
	}
	s.Log("No critical kernel errors found")
}

// checkPathExists checks if a given path exists or not.
// Raise error if the path does not exist.
func checkPathExists(ctx context.Context, s *testing.State, path string) {
	if _, err := os.Stat(path); err != nil {
		s.Fatalf("Failed to stat %s: %s", path, err)
	}
}
