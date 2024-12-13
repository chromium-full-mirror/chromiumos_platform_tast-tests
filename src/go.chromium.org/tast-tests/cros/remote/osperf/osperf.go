// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package osperf

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
)

// OpenBenchPage opens a new tab with the provided url and creates a new connection for it.
func OpenBenchPage(ctx context.Context, benchURL string, conn ui.ConnServiceClient) (*ui.NewConnResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.NewConn(ctx, &ui.NewConnRequest{Url: benchURL})
}

// DumpCommandOutput runs the provided command on the dut and dumps the result to a file with the given path.
func DumpCommandOutput(ctx context.Context, dut *dut.DUT, path, cmd string) error {
	output, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(output), 0644)
}
