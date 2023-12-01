// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixtures

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.FakeDMS,
		Desc: "Fixture for a running FakeDMS",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mohamedaomar@google.com", // Author of fake_dmserver.
			"vsavu@google.com",        // Original fixture author.

		},
		Impl:            &fakeDMSFixture{},
		SetUpTimeout:    15 * time.Second,
		ResetTimeout:    5 * time.Second,
		TearDownTimeout: 5 * time.Second,
		PreTestTimeout:  5 * time.Second,
		PostTestTimeout: 5 * time.Second,
	})

	testing.AddFixture(&testing.Fixture{
		Name: fixture.FakeDMSEnrolled,
		Desc: "Fixture for a running FakeDMS",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mohamedaomar@google.com", // Author of fake_dmserver.
			"vsavu@google.com",        // Original fixture author.

		},
		Impl: &fakeDMSFixture{
			importState: true,
		},
		SetUpTimeout:    15 * time.Second,
		ResetTimeout:    5 * time.Second,
		TearDownTimeout: 5 * time.Second,
		PreTestTimeout:  5 * time.Second,
		PostTestTimeout: 5 * time.Second,
		Parent:          fixture.Enrolled,
	})

	// FakeDMSUpdateEngineEnrolled is identical to FakeDMSEnrolled but inherits
	// from UpdateEngineEnrolled fixture to ensure update engine is reset.
	testing.AddFixture(&testing.Fixture{
		Name: fixture.FakeDMSUpdateEngineEnrolled,
		Desc: "Fixture for a running FakeDMS ensuring to reset update engine",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"crisguerrero@chromium.org",
		},
		Impl: &fakeDMSFixture{
			importState: true,
		},
		SetUpTimeout:    15 * time.Second,
		ResetTimeout:    5 * time.Second,
		TearDownTimeout: 5 * time.Second,
		PreTestTimeout:  5 * time.Second,
		PostTestTimeout: 5 * time.Second,
		Parent:          fixture.UpdateEngineEnrolled,
	})
}

type fakeDMSFixture struct {
	// FakeDMS is the currently running fake DM server.
	fakeDMS *fakedms.FakeDMS
	// fdmsDir is the directory where FakeDMS is currently running.
	fdmsDir string
	// importState determines if state should be imported from the parent fixture.
	importState bool
	// Marker for per-test log.
	logMarker *logsaver.Marker
}

func (f *fakeDMSFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	ctx, st := timing.Start(ctx, "fakeDMS_setup")
	defer st.End()

	// Use a tmpdir to ensure multiple startups don't override logs.
	tmpdir, err := ioutil.TempDir(s.OutDir(), "fdms-")
	if err != nil {
		s.Fatal("Failed to create fdms temp dir: ", err)
	}
	f.fdmsDir = tmpdir

	if f.importState {
		var fixtData policy.EnrolledFixtureData
		if err := s.ParentFillValue(&fixtData); err != nil {
			s.Fatal("Parent fixture did not pass directory: ", err)
		}
		if fixtData.FakeDMSDirectory == "" {
			s.Fatal("Parent fixture passed an empty directory")
		}

		stateTarget := filepath.Join(f.fdmsDir, fakedms.StateFile)
		stateSource := filepath.Join(fixtData.FakeDMSDirectory, fakedms.StateFile)
		if err := fsutil.CopyFile(stateSource, stateTarget); err != nil {
			s.Fatalf("Failed to import the existing state from %q to %q: %v", stateSource, stateTarget, err)
		}
	}

	// Start FakeDMS.
	fdms, err := fakedms.New(s.FixtContext(), f.fdmsDir)
	if err != nil {
		s.Fatal("Failed to start FakeDMS: ", err)
	}

	// Make sure FakeDMS is running.
	if err := fdms.Ping(ctx); err != nil {
		s.Fatal("Failed to ping FakeDMS: ", err)
	}

	pb := policy.NewBlob()

	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	f.fakeDMS = fdms

	return f.fakeDMS
}

func (f *fakeDMSFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	ctx, st := timing.Start(ctx, "fakeDMS_teardown")
	defer st.End()

	if f.fakeDMS != nil {
		f.fakeDMS.Stop(ctx)
	}

	// Copy full FakeDMS log to OutDir.
	src := filepath.Join(f.fdmsDir, fakedms.LogFile)
	dst := filepath.Join(s.OutDir(), fakedms.LogFile)
	if err := fsutil.CopyFile(src, dst); err != nil {
		s.Error("Failed to copy FakeDMS logs: ", err)
	}
}

func (f *fakeDMSFixture) Reset(ctx context.Context) error {
	// Make sure FakeDMS is still running.
	if err := f.fakeDMS.Ping(ctx); err != nil {
		return errors.Wrap(err, "failed to ping FakeDMS")
	}

	// Write policy blob.
	if err := f.fakeDMS.WritePolicyBlob(policy.NewBlob()); err != nil {
		return errors.Wrap(err, "failed to clear policies in FakeDMS")
	}

	return nil
}

func (f *fakeDMSFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if f.logMarker != nil {
		s.Error("A log marker is already created but not cleaned up")
	}
	logMarker, err := logsaver.NewMarker(filepath.Join(f.fdmsDir, fakedms.LogFile))
	if err != nil {
		s.Error("Failed to start the log saver: ", err)
	} else {
		f.logMarker = logMarker
	}
}
func (f *fakeDMSFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.logMarker != nil {
		if err := f.logMarker.Save(filepath.Join(s.OutDir(), "fakedms.log")); err != nil {
			s.Error("Failed to store per-test log data: ", err)
		}
		f.logMarker = nil
	}

	// Copy FakeDMS policies to the current tests OutDir.
	// Add prefix to avoid conflic with the Chrome fixture.
	src := filepath.Join(f.fdmsDir, fakedms.PolicyFile)
	dst := filepath.Join(s.OutDir(), "fakedms_"+fakedms.PolicyFile)
	if err := fsutil.CopyFile(src, dst); err != nil {
		s.Error("Failed to copy FakeDMS policies: ", err)
	}
}
