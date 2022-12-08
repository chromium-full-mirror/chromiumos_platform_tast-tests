// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/common/meta"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
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
		},
		},
		Desc:         "Ensure remote fixture values can be accessed local fixtures ",
		Contacts:     []string{"tast-owner@google.com", "seewaifu@chromium.org", "yichiyan@chromium.org"},
		Attr:         []string{"group:mainline", "informational"},
		BugComponent: "b:207607742",
	})
}

func SerializedFixtVal(ctx context.Context, s *testing.State) {
	serializedVal, err := s.FixtSerializedValue()
	if err != nil {
		s.Fatal("Failed to get serialized fixture value: ", err)
	}
	if err := s.Param().(func([]byte) error)(serializedVal); err != nil {
		s.Fatal("Failed to verify deserialized value: ", err)
	}
}

func verifyDeserializedStringVal(serializedData []byte) error {
	strVal, err := meta.DeserializedTestStringVal(serializedData)
	if err != nil {
		return errors.Wrap(err, "failed to deserialize string data")
	}
	if strVal != meta.RemoteFixtureExpectedStringVal {
		return errors.Errorf("failed to get expected fixture value; got %q, want %q", strVal, meta.RemoteFixtureExpectedStringVal)
	}
	return nil
}

func verifyDeserializedStructVal(serializedData []byte) error {
	structVal, err := meta.DeserializedTestStructVal(serializedData)
	if err != nil {
		return errors.Wrap(err, "failed to deserialize string data")
	}
	if diff := cmp.Diff(structVal, meta.RemoteFixtureExpectedStructVal); diff != "" {
		return errors.Errorf("failed to get expected fixture value; (-got +want): %s", diff)
	}
	return nil
}
