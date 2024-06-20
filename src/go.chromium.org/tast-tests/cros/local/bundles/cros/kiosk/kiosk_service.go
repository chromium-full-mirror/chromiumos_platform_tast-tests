// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"encoding/json"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/syslog"
	ppb "go.chromium.org/tast-tests/cros/services/cros/kiosk"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Vars: []string{"ui.signinProfileTestExtensionManifestKey"},
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			ppb.RegisterKioskServiceServer(srv, &KioskService{s: s})
		},
	})
}

// KioskService implements tast.cros.kiosk.KioskService.
type KioskService struct { // NOLINT
	s *testing.ServiceState

	kiosk        *kioskmode.Kiosk
	chrome       *chrome.Chrome
	fakeDMSDir   string
	fakeDMS      *fakedms.FakeDMS
	syslogReader *syslog.Reader
}

// ConfirmKioskStarted confirms Kiosk mode started.
func (c *KioskService) ConfirmKioskStarted(ctx context.Context, req *ppb.ConfirmKioskStartedRequest) (*empty.Empty, error) {
	if _, err := c.InitSyslogReader(ctx, &empty.Empty{}); err != nil {
		return nil, err
	}
	if _, err := c.ConfirmKioskStartedWithReader(ctx, &empty.Empty{}); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// InitSyslogReader initializes a syslog reader to be used in ConfirmKioskStartedWithReader.
//
// Prefer `ConfirmKioskStarted` if possible. This is useful when you don't control the exact timing
// of Kiosk launch.
func (c *KioskService) InitSyslogReader(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	reader, err := syslog.NewReader(ctx, syslog.Program(syslog.Chrome))
	if err != nil {
		return nil, errors.Wrap(err, "failed to run NewReader")
	}
	c.syslogReader = reader
	return &empty.Empty{}, nil
}

// ConfirmKioskStartedWithReader is the same as ConfirmKioskStarted, but uses the reader previously
// created in InitSyslogReader.
func (c *KioskService) ConfirmKioskStartedWithReader(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	reader := c.syslogReader
	c.syslogReader = nil
	if reader == nil {
		return nil, errors.New("no syslogReader, did you call InitSyslogReader?")
	}
	defer reader.Close()

	if err := kioskmode.WaitLaunchLogsWithReader(ctx, reader); err != nil {
		return nil, errors.Wrap(err, "failed to wait Kiosk launch logs")
	}

	return &empty.Empty{}, nil
}

// StartKiosk starts kiosk in autolaunch mode and local DMServer.
func (c *KioskService) StartKiosk(ctx context.Context, req *empty.Empty) (_ *empty.Empty, retErr error) {
	tmpdir, err := os.MkdirTemp("", "fdms-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp dir")
	}
	c.fakeDMSDir = tmpdir
	defer func() {
		if retErr != nil {
			if err := os.RemoveAll(c.fakeDMSDir); err != nil {
				testing.ContextLogf(ctx, "Failed to delete %s: %v", c.fakeDMSDir, err)
			}
			c.fakeDMSDir = ""
		}
	}()

	signinTestExtensionManifestKey, ok := c.s.Var("ui.signinProfileTestExtensionManifestKey")
	if !ok {
		return nil, errors.New("missing ui.signinProfileTestExtensionManifestKey variable")
	}

	// fakedms.New starts a background process that outlives the current context.
	fdms, err := fakedms.New(c.s.ServiceContext(), c.fakeDMSDir) // NOLINT
	if err != nil {
		return nil, errors.Wrap(err, "failed to start FakeDMS")
	}
	c.fakeDMS = fdms
	defer func() {
		if retErr != nil {
			c.fakeDMS.Stop(ctx)
			c.fakeDMS = nil
		}
	}()
	// Check FakeDMS
	if err := c.fakeDMS.Ping(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to ping FakeDMS")
	}
	emptyPb := policy.NewBlob()
	emptyJSON, err := json.Marshal(emptyPb)
	if err := fdms.WritePolicyBlobRaw(emptyJSON); err != nil {
		return nil, errors.Wrap(err, "failed to write policy blob")
	}

	ctx, cancel := context.WithTimeout(ctx, chrome.EnrollmentAndLoginTimeout+kioskmode.SetupDuration+kioskmode.LaunchDuration+kioskmode.CleanupDuration)
	defer cancel()

	// Enroll the device.
	cr, err := chrome.New(
		ctx,
		chrome.FakeEnterpriseEnroll(chrome.Creds{User: "tast-user@managedchrome.com", Pass: "test0000"}),
		chrome.NoLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		signinTestExtensionManifestKey,
		kioskmode.AutoLaunch(kioskmode.WebKioskAccountID),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome in Kiosk mode")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := kiosk.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to clean up Kiosk: ", err)
			}
			c.kiosk = nil
			c.chrome = nil
		}
	}(ctx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to launch Kiosk")
	}

	c.kiosk = kiosk
	c.chrome = cr

	return &empty.Empty{}, nil
}

func (c *KioskService) UpdatePolicies(ctx context.Context, req *ppb.UpdatePoliciesRequest) (*empty.Empty, error) {
	if c.fakeDMS == nil {
		return nil, errors.New("FakeDMS server not started")
	}

	if c.kiosk == nil {
		return nil, errors.New("kiosk not started")
	}

	if c.chrome == nil {
		return nil, errors.New("chrome not started")
	}

	pb := policy.NewBlob()
	pb.AddPolicy(c.kiosk.DeviceLocalAccounts())
	if err := pb.UnmarshalJSON(req.PolicyJson); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal policy json")
	}

	if err := policyutil.ServeBlobAndRefresh(ctx, c.fakeDMS, c.chrome, pb); err != nil {
		return nil, errors.Wrap(err, "failed to serve and refresh policies")
	}

	return &empty.Empty{}, nil
}

func (c *KioskService) CloseKiosk(ctx context.Context, req *empty.Empty) (_ *empty.Empty, retErr error) {
	if c.kiosk != nil {
		if err := c.kiosk.Close(ctx); err != nil {
			retErr = errors.Wrap(err, "failed to close kiosk")
		}
		c.kiosk = nil
		c.chrome = nil
	}

	if c.fakeDMS != nil {
		c.fakeDMS.Stop(ctx)
		c.fakeDMS = nil
	}

	if c.fakeDMSDir != "" {
		if err := os.RemoveAll(c.fakeDMSDir); err != nil {
			testing.ContextLog(ctx, "Failed to remove temporary directory: ", err)
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to remove temporary directory")
			} else {
				testing.ContextLog(ctx, "Failed to remove temporary directory: ", err)
			}
		}
		c.fakeDMSDir = ""
	}

	return &empty.Empty{}, retErr
}
