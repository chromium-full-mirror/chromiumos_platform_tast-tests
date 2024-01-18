// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"fmt"
	"os"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Stdio,
		Desc:         "Ensures that accessing stdin/stdout does not harm test execution",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "group:meta", "group:hw_agnostic"},
	})
}

func Stdio(ctx context.Context, s *testing.State) {
	var r string
	if _, err := fmt.Scan(&r); err == nil {
		s.Error("fmt.Scan succeeded unexpectedly")
	}
	if _, err := fmt.Print("foo"); err == nil {
		s.Error("fmt.Print succeeded unexpectedly")
	}
	if _, err := fmt.Fprint(os.Stderr, "foo"); err == nil {
		s.Error("fmt.Fprint succeeded unexpectedly")
	}
}
