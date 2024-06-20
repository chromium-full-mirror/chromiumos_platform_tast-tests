// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EhideCommandSupport,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test rsync, scp, and sftp commands when ehide is enabled",
		Timeout:      1 * time.Minute,
		Attr:         []string{"group:mainline", "informational"},
		Contacts:     []string{"cros-networking@google.com", "chenzikai@google.com"},
		BugComponent: "b:1493959", // ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		Fixture:      "ehide",
		Params: []testing.Param{{
			Name: "rsync",
			Val:  "rsync",
		}, {
			Name: "scp",
			Val:  "scp",
		}, {
			Name: "sftp",
			Val:  "sftp",
		}},
	})
}

func EhideCommandSupport(ctx context.Context, s *testing.State) {
	// Create a local test file to upload later to DUT.
	localFile, err := os.CreateTemp(os.TempDir(), "ehide_test_file_")
	if err != nil {
		s.Fatal("Failed to create test file: ", err)
	}
	localPath := localFile.Name()
	defer os.Remove(localPath)
	defer localFile.Close()

	// Write data to the test file.
	const testFileData = "Ehide test data"
	if _, err := localFile.Write([]byte(testFileData)); err != nil {
		s.Fatal("Failed to write test file: ", err)
	}

	dutDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get DUT directory")
	}
	d := s.DUT()
	if err := d.Conn().CommandContext(ctx, "mkdir", "-p", dutDir).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to create DUT directory: ", err)
	}
	_, testFileName := path.Split(localPath)
	// The destination to upload the test file to.
	dutPath := filepath.Join(dutDir, testFileName)

	// Parse hostname and port.
	hostnameSplit := strings.Split(d.HostName(), ":")
	if len(hostnameSplit) != 2 {
		s.Fatal("Failed to parse hostname from ", d.HostName())
	}
	hostname := hostnameSplit[0]
	port := hostnameSplit[1]
	remotePath := fmt.Sprintf("root@%s:%s", hostname, dutDir)

	// Defer the file removal function.
	defer func() {
		if err := removeDUTFile(ctx, d, dutPath); err != nil {
			s.Error("Failed to remove the test file on DUT: ", err)
		}
	}()

	// Start testing!
	testingCmd := s.Param().(string)
	s.Logf("Testing %s", testingCmd)
	switch testingCmd {
	case "rsync":
		if err := testexec.CommandContext(ctx, "rsync", "-e", fmt.Sprintf("ssh -p %s", port), localPath, remotePath).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to run rsync: ", err)
		}
	case "scp":
		if err := testexec.CommandContext(ctx, "scp", "-P", port, localPath, remotePath).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to run scp: ", err)
		}
	case "sftp":
		if err := runSFTP(ctx, localPath, remotePath); err != nil {
			s.Fatal("Failed to run sftp: ", err)
		}
	}
	if err := verifyDUTFile(ctx, d, dutPath, testFileData); err != nil {
		s.Error("Failed to verify DUT file: ", err)
	}
}

// verifyDUTFile verifies the content of the uploaded test file.
func verifyDUTFile(ctx context.Context, d *dut.DUT, dutPath, testFileData string) error {
	out, err := linuxssh.ReadFile(ctx, d.Conn(), dutPath)
	if err != nil {
		return errors.Wrap(err, "failed to read uploaded DUT file")
	}
	if string(out) != testFileData {
		return errors.Errorf("DUT file contents do not match, got %s, want %s", out, testFileData)
	}
	return nil
}

func removeDUTFile(ctx context.Context, d *dut.DUT, dutPath string) error {
	if err := d.Conn().CommandContext(ctx, "rm", dutPath).Run(exec.DumpLogOnError); err != nil {
		// OK if the file does not exist.
		if strings.Contains(err.Error(), "No such file or directory") {
			return nil
		}
		return err
	}
	return nil
}

func runSFTP(ctx context.Context, localPath, remotePath string) error {
	// Set a short timeout for sftp to avoid sftp blocking the test.
	sftpCtx, sftpCtxCancel := context.WithTimeout(ctx, 15*time.Second)
	defer sftpCtxCancel()

	cmd := testexec.CommandContext(sftpCtx, "sftp", remotePath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return errors.Wrap(err, "failed to get stdin pipe")
	}

	go func() {
		defer stdin.Close()
		if _, err := io.WriteString(stdin, fmt.Sprintf("put %s", localPath)); err != nil {
			testing.ContextLog(ctx, "Failed to write to stdin: ", err)
		}
	}()

	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to run sftp")
	}
	return nil
}
