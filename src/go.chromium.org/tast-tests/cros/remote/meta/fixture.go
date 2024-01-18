// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package meta contains support code for Tast meta tests.
package meta

import (
	"context"
	"path/filepath"
	"reflect"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast-tests/cros/common/meta"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/framework/protocol"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "metaRemoteDataFilesFixture",
		Desc:     "Demonstrate how to use data files in fixtures",
		Contacts: []string{"tast-owner@google.com", "oka@chromium.org"},
		Data: []string{
			"fixture_data_internal.txt",
			"fixture_data_external.txt",
		},
		Impl: dataFileFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaRemoteFixtureDUTFeature",
		Desc:     "Demonstrate how to access DUT Features in fixtures",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org"},
		Data:     []string{},
		Impl:     &dutFeatureFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaRemoteFixtureWithStringVal",
		Desc:     "Used for verification of tests accessing string value from local fixture",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org", "yichiyan@chromium.org"},
		Data:     []string{},
		Impl:     &fixtSerializedStringFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaRemoteFixtureWithStructVal",
		Desc:     "Used for verification of tests accessing string value from local fixture",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org", "yichiyan@chromium.org"},
		Data:     []string{},
		Impl:     &fixtSerializedStructFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaRemoteSetupFailureFixture",
		Desc:     "Test tast fixture failures",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org"},
		Impl:     remoteSetupFailureFixture{},
	})
}

type fixtSerializedStringFixture struct{}

func (fixtSerializedStringFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return meta.RemoteFixtureExpectedStringVal
}
func (fixtSerializedStringFixture) Reset(ctx context.Context) error {
	return nil
}
func (fixtSerializedStringFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (fixtSerializedStringFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (fixtSerializedStringFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}

type fixtSerializedStructFixture struct{}

func (fixtSerializedStructFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return meta.RemoteFixtureExpectedStructVal
}
func (fixtSerializedStructFixture) Reset(ctx context.Context) error {
	return nil
}
func (fixtSerializedStructFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (fixtSerializedStructFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (fixtSerializedStructFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}

type dataFileFixture struct{}

func (dataFileFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	for _, fn := range []string{
		"fixture_data_internal.txt",
		"fixture_data_external.txt",
	} {
		s.Log("Copying ", fn)
		if err := fsutil.CopyFile(s.DataPath(fn), filepath.Join(s.OutDir(), fn)); err != nil {
			s.Errorf("Failed copying %s: %s", fn, err)
		}
	}
	return nil
}
func (dataFileFixture) Reset(ctx context.Context) error {
	return nil
}
func (dataFileFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (dataFileFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (dataFileFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}

type dutFeatureFixture struct {
	feature *protocol.DUTFeatures
}

func (dff *dutFeatureFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	dff.feature = s.Features("")
	return dff.feature
}
func (dff *dutFeatureFixture) Reset(ctx context.Context) error {
	return nil
}
func (dff *dutFeatureFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	feature := s.Features("")
	allowUnexported := func(reflect.Type) bool { return true }
	if diff := cmp.Diff(feature, dff.feature, cmpopts.EquateEmpty(), cmp.Exporter(allowUnexported)); diff != "" {
		s.Logf("Got unexpected feature in PreTest (-got +want): %s", diff)
		s.Fatal("Got unexpected feature in PreTest")
	}
}
func (dff *dutFeatureFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	feature := s.Features("")
	allowUnexported := func(reflect.Type) bool { return true }
	if diff := cmp.Diff(feature, dff.feature, cmpopts.EquateEmpty(), cmp.Exporter(allowUnexported)); diff != "" {
		s.Logf("Got unexpected feature in PostTest (-got +want): %s", diff)
		s.Fatal("Got unexpected feature in PostTest")
	}
}
func (dff *dutFeatureFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	feature := s.Features("")
	allowUnexported := func(reflect.Type) bool { return true }
	if diff := cmp.Diff(feature, dff.feature, cmpopts.EquateEmpty(), cmp.Exporter(allowUnexported)); diff != "" {
		s.Logf("Got unexpected feature in TearDown (-got +want): %s", diff)
		s.Fatal("Got unexpected feature in TearDown")
	}
}

type remoteSetupFailureFixture struct{}

func (remoteSetupFailureFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	s.Error("Failed intentionally ")
	return nil
}
func (remoteSetupFailureFixture) Reset(ctx context.Context) error {
	return nil
}
func (remoteSetupFailureFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {

}
func (remoteSetupFailureFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (remoteSetupFailureFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}
