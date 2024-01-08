// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/meta"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SerializedFixtVal,
		Params: []testing.Param{{
			Name:    "access_string_value_from_remote_fixture",
			Val:     verifyDeserializedStringVal,
			Fixture: "metaRemoteFixtureWithStringVal",
		}, {
			Name:    "access_string_value_from_local_fixture",
			Val:     verifyDeserializedStringVal,
			Fixture: "metaLocalFixtureWithStringVal",
		}, {
			Name:    "access_struct_value_from_remote_fixture",
			Val:     verifyDeserializedStructVal,
			Fixture: "metaRemoteFixtureWithStructVal",
		}, {
			Name:    "access_struct_value_from_local_fixture",
			Val:     verifyDeserializedStructVal,
			Fixture: "metaLocalFixtureWithStructVal",
		}, {
			Name:    "access_string_value_from_same_bundle",
			Val:     verifyDeserializedStringVal,
			Fixture: "metaLocalFixtureWithSameBundle",
		},
		},
		Desc:         "Ensure remote fixture values can be accessed local fixtures ",
		Contacts:     []string{"tast-core@google.com", "seewaifu@chromium.org", "yichiyan@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		BugComponent: "b:1034522",
	})
}

func SerializedFixtVal(ctx context.Context, s *testing.State) {
	if err := s.Param().(func(*testing.State) error)(s); err != nil {
		s.Fatal("Failed to verify deserialized value: ", err)
	}
}

func verifyDeserializedStringVal(s *testing.State) error {
	strValue := ""
	if err := s.FixtFillValue(&strValue); err != nil {
		return errors.Wrap(err, "failed to deserialize string data with FixtFillValue")
	}
	if strValue != meta.RemoteFixtureExpectedStringVal {
		return errors.Errorf("failed to get expected fixture value with FixtFillValue; got %q, want %q", strValue, meta.RemoteFixtureExpectedStringVal)
	}
	return nil
}

func verifyDeserializedStructVal(s *testing.State) error {
	structValue := meta.TestStruct{}
	if err := s.FixtFillValue(&structValue); err != nil {
		return errors.Wrap(err, "failed to deserialize struct data with FixtFillValue")
	}
	if diff := cmp.Diff(&structValue, meta.RemoteFixtureExpectedStructVal); diff != "" {
		return errors.Errorf("failed to get expected fixture value with FixtFillValue; (-got +want): %s", diff)
	}
	return nil
}
