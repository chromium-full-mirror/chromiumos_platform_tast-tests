// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/meta"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RemoteFixtParam,
		Params: []testing.Param{{
			Fixture: "metaRemoteParamFixture",
			Val: []string{
				meta.RemoteFeature,
			},
		}, {
			Name:    "a",
			Fixture: "metaRemoteParamFixture.a",
			Val: []string{
				meta.RemoteFeature,
				meta.RemoteFeatureA,
			},
		}, {
			Name:    "b",
			Fixture: "metaRemoteParamFixture.b",
			Val: []string{
				meta.RemoteFeature,
				meta.RemoteFeatureB,
			},
		}, {
			Name:    "ab",
			Fixture: "metaRemoteParamFixture.ab",
			Val: []string{
				meta.RemoteFeature,
				meta.RemoteFeatureA,
				meta.RemoteFeatureB,
			},
		},
		},
		Desc:         "Ensure remote tests can use remote fixtures",
		Contacts:     []string{"tast-core@google.com", "seewaifu@chromium.org", "yichiyan@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		BugComponent: "b:1034522",
	})
}

func RemoteFixtParam(ctx context.Context, s *testing.State) {
	fixtFeatures := s.FixtValue().([]string)
	expectedFeatures := s.Param().([]string)

	if diff := cmp.Diff(fixtFeatures, expectedFeatures); diff != "" {
		// The variable diff is a multi-lines string which is not allowed with s.Fatal.
		// Use format string instead.
		s.Fatalf("Failed to get correct value got %v wanted %v", fixtFeatures, expectedFeatures)
	}
}
