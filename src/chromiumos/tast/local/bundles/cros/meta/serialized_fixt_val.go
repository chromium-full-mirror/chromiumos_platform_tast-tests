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
	if err := s.Param().(func(*testing.State) error)(s); err != nil {
		s.Fatal("Failed to verify deserialized value: ", err)
	}
}

func verifyDeserializedStringVal(s *testing.State) error {
	serializedData, err := s.FixtSerializedValue()
	if err != nil {
		errors.Wrap(err, "failed to get serialized fixture value")
	}
	strVal, err := meta.DeserializedTestStringVal(serializedData)
	if err != nil {
		return errors.Wrap(err, "failed to deserialize string data")
	}
	if strVal != meta.RemoteFixtureExpectedStringVal {
		return errors.Errorf("failed to get expected fixture value; got %q, want %q", strVal, meta.RemoteFixtureExpectedStringVal)
	}
	strValue := ""
	if err := s.FixtFillValue(&strValue); err != nil {
		return errors.Wrap(err, "failed to deserialize string data with FixtDeserializedValue")
	}
	if strValue != meta.RemoteFixtureExpectedStringVal {
		return errors.Errorf("failed to get expected fixture value with FixtDeserializedValue; got %q, want %q", strVal, meta.RemoteFixtureExpectedStringVal)
	}
	return nil
}

func verifyDeserializedStructVal(s *testing.State) error {
	// TODO: b/264292451: Remove the use of FixtSerializedValue().
	serializedData, err := s.FixtSerializedValue()
	if err != nil {
		errors.Wrap(err, "failed to get serialized fixture value")
	}
	structVal, err := meta.DeserializedTestStructVal(serializedData)
	if err != nil {
		return errors.Wrap(err, "failed to deserialize string data")
	}
	if diff := cmp.Diff(structVal, meta.RemoteFixtureExpectedStructVal); diff != "" {
		return errors.Errorf("failed to get expected fixture value; (-got +want): %s", diff)
	}
	structValue := meta.TestStruct{}
	if err := s.FixtFillValue(&structValue); err != nil {
		return errors.Wrap(err, "failed to deserialize struct data with FixtDeserializedValue")
	}
	if diff := cmp.Diff(structVal, meta.RemoteFixtureExpectedStructVal); diff != "" {
		return errors.Errorf("failed to get expected fixture value with FixtDeserializedValue; (-got +want): %s", diff)
	}
	return nil
}
