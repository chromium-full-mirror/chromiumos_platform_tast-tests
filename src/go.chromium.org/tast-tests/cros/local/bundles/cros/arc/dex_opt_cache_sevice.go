// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/cache"
	arcpb "go.chromium.org/tast-tests/cros/services/cros/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			arcpb.RegisterDexOptCacheServiceServer(srv, &DexOptCacheService{s})
		},
	})
}

// DexOptCacheService implements tast.cros.arc.DexOptCacheService.
type DexOptCacheService struct {
	s *testing.ServiceState
}

// Generate generates TTS cache.
func (c *DexOptCacheService) Generate(ctx context.Context, request *empty.Empty) (res *arcpb.DexOptCacheResponse, retErr error) {
	testing.ContextLog(ctx, "Generating DexOpt cache")

	targetDir, err := ioutil.TempDir("", "dex_opt_cache")
	if err != nil {
		return nil, errors.Wrap(err, "failed to created target dir for DexOpt cache")
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(targetDir)
		}
	}()

	testing.ContextLog(ctx, "Starting ARC")
	cr, a, err := cache.OpenSession(ctx, []string{}, targetDir)
	if err != nil {
		os.RemoveAll(targetDir)
		return nil, errors.Wrap(err, "failed to start ARC")
	}

	defer cr.Close(ctx)
	defer a.Close(ctx)

	if err := cache.CopyDexOptCache(ctx, targetDir); err != nil {
		os.RemoveAll(targetDir)
		return nil, errors.Wrap(err, "failed to generate DexOpt cache")
	}

	return &arcpb.DexOptCacheResponse{
		TargetDir:       targetDir,
		DexOptCacheName: cache.DexOptCacheArchive,
	}, nil
}
