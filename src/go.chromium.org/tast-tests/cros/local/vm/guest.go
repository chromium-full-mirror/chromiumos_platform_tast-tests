// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

// Guest is an interface to a generic guest OS, be it a container or VM.
// It can be used by tests that do not use the implementation details of any
// specific type of guest OS.
type Guest interface {
	Command(ctx context.Context, vshArgs ...string) *testexec.Cmd
}
