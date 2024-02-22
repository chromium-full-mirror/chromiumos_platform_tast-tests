// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arcvideoplayback

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
)

// VideoApp contains user's operation in video application.
type VideoApp interface {
	Install(ctx context.Context) error
	Uninstall(ctx context.Context) error
	Launch(ctx context.Context) error
	CopyFileToFolder(ctx context.Context, videoPath string) (cleanup func() error, retErr error)
	PlayVideoInFullScreen(video string) uiauto.Action
	Close(ctx context.Context) error
}
