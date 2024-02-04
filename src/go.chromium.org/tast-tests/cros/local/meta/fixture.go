// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package meta contains support code for Tast meta tests.
package meta

import (
	"context"
	"fmt"
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
		Name:     "metaLocalDataFilesFixture",
		Desc:     "Demonstrate how to use data files in fixtures",
		Contacts: []string{"tast-owner@google.com", "seewaifu@google.com"},
		Data: []string{
			"fixture_data_internal.txt",
			"fixture_data_external.txt",
		},
		Impl: dataFileFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaLocalFixtureDUTFeature",
		Desc:     "Demonstrate how to access DUT Features in fixtures",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org"},
		Data:     []string{},
		Impl:     &dutFeatureFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaLocalFixtureWithStringVal",
		Parent:   "metaRemoteFixtureWithStringVal",
		Desc:     "Used for verification of tests accessing string value from local fixture",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org", "yichiyan@chromium.org"},
		Data:     []string{},
		Impl:     &fixtSerializedStringFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaLocalFixtureWithStructVal",
		Parent:   "metaRemoteFixtureWithStructVal",
		Desc:     "Used for verification of tests accessing string value from local fixture",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org", "yichiyan@chromium.org"},
		Data:     []string{},
		Impl:     &fixtSerializedStructFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaLocalFixtureWithSameBundle",
		Parent:   "metaLocalFixtureWithStringVal",
		Desc:     "Used for verification of fixtures accessing string value from parent in the same bundle",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org", "yichiyan@chromium.org"},
		Data:     []string{},
		Impl:     &fixtSerializedSameBundleFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaLocalSetupFailureFixture",
		Desc:     "Test tast fixture failures",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org"},
		Impl:     localSetupFailureFixture{},
	})
	testing.AddFixture(&testing.Fixture{
		Name:     "metaLocalParamFixture",
		Desc:     "Test tast parameterized fixture",
		Contacts: []string{"tast-owner@google.com", "seewaifu@chromium.org"},
		Impl:     localParamFixture{},
		Params:   genParams(),
	})
}

type fixtSerializedStringFixture struct{}

func (fixtSerializedStringFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	s.AttachErrorHandlers(
		func(errMsg string) {
			testing.ContextLog(ctx, "Got error: ", errMsg)
		},
		func(errMsg string) {
			testing.ContextLog(ctx, "Got fatal error: ", errMsg)
		})
	v := ""
	if err := s.ParentFillValue(&v); err != nil {
		s.Fatal("Failed to get remote parent string value in Setup: ", err)
	}
	if v != meta.RemoteFixtureExpectedStringVal {
		s.Errorf("Failed to get expected fixture value with FixtFillValue; got %q, want %q", v, meta.RemoteFixtureExpectedStringVal)
	}
	return v
}
func (fixtSerializedStringFixture) Reset(ctx context.Context) error {
	return nil
}
func (fixtSerializedStringFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {

}
func (fixtSerializedStringFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (fixtSerializedStringFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}

type fixtSerializedSameBundleFixture struct{}

func (fixtSerializedSameBundleFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	v := ""
	if err := s.ParentFillValue(&v); err != nil {
		s.Fatal("Failed to get remote parent string value in Setup: ", err)
	}
	parentValue, ok := s.ParentValue().(string)
	if !ok {
		s.Fatal("Parent is not avaiable")
	}
	if v != parentValue {
		s.Errorf("Failed to get expected fixture value with FixtFillValue; got %q, want %q", v, parentValue)
	}
	return v
}
func (fixtSerializedSameBundleFixture) Reset(ctx context.Context) error {
	return nil
}
func (fixtSerializedSameBundleFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {

}
func (fixtSerializedSameBundleFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (fixtSerializedSameBundleFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}

type fixtSerializedStructFixture struct{}

func (fixtSerializedStructFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	structValue := meta.TestStruct{}
	if err := s.ParentFillValue(&structValue); err != nil {
		s.Fatal("Failed to deserialize struct data with FixtFillValue: ", err)
	}
	if diff := cmp.Diff(&structValue, meta.RemoteFixtureExpectedStructVal); diff != "" {
		s.Errorf("Failed to get expected fixture value with FixtFillValue; (-got +want): %s", diff)
	}
	return structValue
}

func (fixtSerializedStructFixture) Reset(ctx context.Context) error {
	return nil
}
func (fixtSerializedStructFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (fixtSerializedStructFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (fixtSerializedStructFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}

type dataFileFixture struct{}

// DataFiles contains fixture data files information.
type DataFiles struct {
	Files []string // Files contains location for all fixture data files.
}

func (dataFileFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var files []string
	for _, fn := range []string{
		"fixture_data_internal.txt",
		"fixture_data_external.txt",
	} {
		s.Log("Copying ", fn)
		dst := filepath.Join(s.OutDir(), fn)
		if err := fsutil.CopyFile(s.DataPath(fn), dst); err != nil {
			s.Errorf("Failed copying %s: %s", fn, err)
		}
		files = append(files, dst)
	}
	return &DataFiles{Files: files}
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

type localSetupFailureFixture struct{}

func (localSetupFailureFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	s.Error("Failed intentionally ")
	return nil
}
func (localSetupFailureFixture) Reset(ctx context.Context) error {
	return nil
}
func (localSetupFailureFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {

}
func (localSetupFailureFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (localSetupFailureFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}

type localParamVal struct {
	featureA bool
	featureB bool
}
type remoteParamFixture struct{}

type localParamFixture struct{}

func (localParamFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var v []string
	// Since the parent fixture is from a different bundle, ParentFillValue is used.
	// If the fixture is from the same bundle, ParentValue can be used, too.
	if err := s.ParentFillValue(&v); err != nil {
		s.Fatal("Failed to get remote parent string value in Setup: ", err)
	}
	val := s.Param().(localParamVal)
	features := []string{meta.LocalFeature}
	if val.featureA {
		features = append(features, meta.LocalFeatureA)
	}
	if val.featureB {
		features = append(features, meta.LocalFeatureB)
	}
	features = append(features, v...)
	return features
}
func (localParamFixture) Reset(ctx context.Context) error {
	return nil
}
func (localParamFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (localParamFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
func (localParamFixture) TearDown(ctx context.Context, s *testing.FixtState)     {}

const (
	// BaseFeature is a feature mask for a parameterized factory fixture example.
	BaseFeature = iota
	// FeatureA is a feature mask for a parameterized factory fixture example.
	FeatureA = 1 << iota
	// FeatureB is a feature mask for a parameterized factory fixture example.
	FeatureB = 1 << iota
)

// supportedFeatures provide a mapping between the supported feature combinations
// and the parent fixtures.
var supportedFeatures = map[uint32]string{
	BaseFeature:         "metaRemoteParamFixture",
	FeatureA:            "metaRemoteParamFixture.a",
	FeatureB:            "metaRemoteParamFixture.b",
	FeatureA | FeatureB: "metaRemoteParamFixture.ab",
}

// genParams generates all supported parameters of localParamFixture.
func genParams() (params []testing.FixtureParam) {
	for features := range supportedFeatures {
		params = append(params, paramFixtureFactory(features))
	}
	return params
}

// paramFixtureFactory returns a parameterized fixture of localParamFixture.
func paramFixtureFactory(features uint32) testing.FixtureParam {
	val := localParamVal{
		featureA: (features & FeatureA) != 0,
		featureB: (features & FeatureB) != 0,
	}

	parent, ok := supportedFeatures[features]
	if !ok {
		panic(fmt.Sprintf("unsupported feature mask: %x", features))
	}

	return testing.FixtureParam{
		Name:   paramName(features),
		Val:    val,
		Parent: parent,
	}
}

// ParamFixtureName returns a parameterized fixture of localParamFixture.
func ParamFixtureName(features uint32) string {
	_, ok := supportedFeatures[features]
	if !ok {
		panic(fmt.Sprintf("unsupported feature mask: %x", features))
	}
	return fmt.Sprintf("metaLocalParamFixture.%s", paramName(features))
}

func paramName(features uint32) string {
	return fmt.Sprintf("f_%x", features)
}
