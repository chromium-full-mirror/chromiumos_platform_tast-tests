// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crospts

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/crospts/ptsworld"
	"go.chromium.org/tast/core/testing"
)

const (
	// ptsWorldBaseImagex86 is the tast external data name of x86 base image.
	ptsWorldBaseImagex86 = "base-x86_64.img.tar.xz"
	// ptsWorldDataImagex86 is the tast external data name of x86 data image.
	ptsWorldDataImagex86 = "pts-data-x86_64.img.tar.xz"
	// ptsWorldBaseImagearm64 is the tast external data name of arm64 base image.
	ptsWorldBaseImagearm64 = "base-arm64.img.tar.xz"
	// ptsWorldDataImagearm64 is the tast external data name of arm64 data image.
	ptsWorldDataImagearm64 = "pts-data-arm64.img.tar.xz"
)

const (
	// setUpTimeout is the time to set up PTSWorld.
	setUpTimeout = 3 * time.Minute
	// resetTimeout is the time to reset PTSWorld.
	resetTimeout = 5 * time.Second
	// preTestTimeout is the time to run pre-test steps.
	preTestTimeout = 5 * time.Second
	// postTestTimeout is the time to run post-test steps.
	postTestTimeout = 5 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "mountPtsWorldForCrOSx86",
		Desc: "The fixture to mount PTSWorld for CrOS host on x86",
		Contacts: []string{
			"cros-core-systems-perf@google.com",
			"darrenwu@google.com",
		},
		Data: []string{ptsWorldBaseImagex86, ptsWorldDataImagex86},
		Impl: &PtsWorldFixture{
			fixture: ptsworld.NewCrosFixture(ptsWorldBaseImagex86, ptsWorldDataImagex86),
			mount:   true,
			unmount: false,
		},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "unmountPtsWorldForCrOSx86",
		Desc: "The fixture to unmount PTSWorld for CrOS host on x86",
		Contacts: []string{
			"cros-core-systems-perf@google.com",
			"darrenwu@google.com",
		},
		Data: []string{ptsWorldBaseImagex86, ptsWorldDataImagex86},
		Impl: &PtsWorldFixture{
			fixture: ptsworld.NewCrosFixture(ptsWorldBaseImagex86, ptsWorldDataImagex86),
			mount:   false,
			unmount: true,
		},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "mountPtsWorldForCrOSarm64",
		Desc: "The fixture to mount PTSWorld for CrOS host on arm64",
		Contacts: []string{
			"cros-core-systems-perf@google.com",
			"darrenwu@google.com",
		},
		Data: []string{ptsWorldBaseImagearm64, ptsWorldDataImagearm64},
		Impl: &PtsWorldFixture{
			fixture: ptsworld.NewCrosFixture(ptsWorldBaseImagearm64, ptsWorldDataImagearm64),
			mount:   true,
			unmount: false,
		},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "unmountPtsWorldForCrOSarm64",
		Desc: "The fixture to unmount PTSWorld for CrOS host on arm64",
		Contacts: []string{
			"darrenwu@google.com",
			"cros-core-systems-perf@google.com",
		},
		Data: []string{ptsWorldBaseImagearm64, ptsWorldDataImagearm64},
		Impl: &PtsWorldFixture{
			fixture: ptsworld.NewCrosFixture(ptsWorldBaseImagearm64, ptsWorldDataImagearm64),
			mount:   false,
			unmount: true,
		},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
}

type PtsWorldFixture struct {
	fixture ptsworld.Fixture
	mount   bool
	unmount bool
}

func (f *PtsWorldFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := f.fixture.Prepare(ctx, s); err != nil {
		s.Fatal("Failed to prepare PTSWorld: ", err)
		return err
	}

	if f.mount {
		if err := f.fixture.Mount(ctx, s); err != nil {
			s.Fatal("Failed to mount PTSWorld: ", err)
			return err
		}
	}
	return nil
}

func (f *PtsWorldFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.unmount {
		s.Log("Unmounting PTSWorld")
		if err := f.fixture.Unmount(ctx, s); err != nil {
			s.Fatal("Failed to unmount PTSWorld: ", err)
		}
	}
}

func (f *PtsWorldFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *PtsWorldFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *PtsWorldFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
