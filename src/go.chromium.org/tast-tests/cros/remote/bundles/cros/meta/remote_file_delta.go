// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteFileDelta,
		Desc:         "Demonstrates how to save a delta of a file with Tast utility RemoteFileDelta",
		Contacts:     []string{"tast-core@google.com", "yichiyan@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
	})
}

func RemoteFileDelta(ctx context.Context, s *testing.State) {
	const (
		cp1 = "RemoteFileDelta Checkpoint1"
		cp2 = "RemoteFileDelta Checkpoint2"
	)

	dst1 := filepath.Join(s.OutDir(), "message_delta_1.txt")
	saveDelta(ctx, s, dst1, cp1)

	dst2 := filepath.Join(s.OutDir(), "message_delta_2.txt")
	saveDelta(ctx, s, dst2, cp2)

	// Make sure dst1 has its message.
	d1, err := os.ReadFile(dst1)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", dst1, err)
	}
	s1 := string(d1)
	if !strings.Contains(s1, cp1) {
		s.Fatalf("Failed to find %q in %s", cp1, s1)
	}

	// Make sure dst2 has its message.
	d2, err := os.ReadFile(dst2)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", dst2, err)
	}
	s2 := string(d2)
	if !strings.Contains(s2, cp2) {
		s.Fatalf("Failed to find %q in %s", cp2, s2)
	}

	// Make sure dst2 does not have cp1.
	if strings.Contains(s2, cp1) {
		s.Fatalf("Failed to find unexpected %q in %s", cp1, s2)
	}
}

func saveDelta(ctx context.Context, s *testing.State, dst, msg string) {
	const maxLogSize = 20 * 1024 * 1024 // default max system message log size 20MB

	conn := s.DUT().Conn()
	rtd, err := linuxssh.NewRemoteFileDelta(ctx, conn, "/var/log/messages", dst, maxLogSize)
	if err != nil {
		s.Fatal("Failed to create object for saving message_delta_1.txt: ", err)
	}

	if _, err := conn.CommandContext(ctx, "logger", msg).CombinedOutput(); err != nil {
		s.Fatalf("Failed to log %q: %v", msg, err)
	}
	// GoBigSleepLint: waiting 5 second for additional /var/log/messages to be populated.
	testing.Sleep(ctx, time.Second*5)
	rtd.Save(ctx)

}
