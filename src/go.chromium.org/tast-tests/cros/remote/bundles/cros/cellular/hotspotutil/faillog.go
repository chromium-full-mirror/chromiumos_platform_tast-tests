// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hotspotutil

import (
	"context"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/services/cros/ui"
)

// FaillogServiceName is the service needed for capture fail-logs.
var FaillogServiceName = "tast.cros.ui.ChromeUIService"

// DumpUITreeWithScreenshotToFile dumps the UI tree and takes screenshot on error, save as file with specified name.
// It takes a grpc client connection to creates a service to further dumps the UI tree and takes screenshot.
func DumpUITreeWithScreenshotToFile(ctx context.Context, conn *grpc.ClientConn, filePrefix string) error {
	svc := ui.NewChromeUIServiceClient(conn)
	if _, err := svc.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{FilePrefix: filePrefix}); err != nil {
		return err
	}
	return nil
}
