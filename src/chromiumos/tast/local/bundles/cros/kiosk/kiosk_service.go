// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/kioskmode"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/syslog"
	ppb "chromiumos/tast/services/cros/kiosk"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			ppb.RegisterKioskServiceServer(srv, &KioskService{s: s})
		},
	})
}

// KioskService implements tast.cros.kiosk.KioskService.
type KioskService struct { // NOLINT
	s *testing.ServiceState

	kiosk      *kioskmode.Kiosk
	chrome     *chrome.Chrome
	fakeDMSDir string
	fakeDMS    *fakedms.FakeDMS
}

// ConfirmKioskStarted confirms kiosk mode started.
func (c *KioskService) ConfirmKioskStarted(ctx context.Context, req *ppb.ConfirmKioskStartedRequest) (*empty.Empty, error) {
	reader, err := syslog.NewReader(ctx, syslog.Program(syslog.Chrome))
	if err != nil {
		return nil, errors.Wrap(err, "failed to run NewReader")
	}
	defer reader.Close()

	if err := kioskmode.ConfirmKioskStarted(ctx, reader); err != nil {
		return nil, errors.Wrap(err, "There was a problem while checking chrome logs for Kiosk related entries")
	}

	return &empty.Empty{}, nil
}

// StartKiosk starts kiosk in autolaunch mode and local DMServer.
func (c *KioskService) StartKiosk(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	ok := false

	tmpdir, err := ioutil.TempDir("", "fdms-")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp dir")
	}
	c.fakeDMSDir = tmpdir
	defer func() {
		if !ok {
			if err := os.RemoveAll(c.fakeDMSDir); err != nil {
				testing.ContextLogf(ctx, "Failed to delete %s: %v", c.fakeDMSDir, err)
			}
			c.fakeDMSDir = ""
		}
	}()

	// fakedms.New starts a background process that outlives the current context.
	fdms, err := fakedms.New(c.s.ServiceContext(), c.fakeDMSDir) // NOLINT
	if err != nil {
		return nil, errors.Wrap(err, "failed to start FakeDMS")
	}
	c.fakeDMS = fdms
	defer func() {
		if !ok {
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

	ctx, cancel := context.WithTimeout(ctx, chrome.EnrollmentAndLoginTimeout)
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
		kioskmode.DefaultLocalAccounts(),
		kioskmode.AutoLaunch(kioskmode.WebKioskAccountID),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome in Kiosk mode")
	}
	c.kiosk = kiosk
	c.chrome = cr
	defer func() {
		if !ok {
			kiosk.Close(ctx)
			c.kiosk = nil
			c.chrome = nil
		}
	}()

	ok = true
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
	pb.AddPolicy(c.kiosk.GetLocalAccounts())
	if err := pb.UnmarshalJSON(req.PolicyJson); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal policy json")
	}

	if err := policyutil.ServeBlobAndRefresh(ctx, c.fakeDMS, c.chrome, pb); err != nil {
		return nil, errors.Wrap(err, "failed to serve and refresh policies")
	}

	return &empty.Empty{}, nil
}

func (c *KioskService) CloseKiosk(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	var lastErr error

	if c.kiosk != nil {
		if err := c.kiosk.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close kiosk: ", err)
			lastErr = errors.Wrap(err, "failed to close kiosk")
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
			lastErr = errors.Wrap(err, "failed to remove temporary directory")
		}
		c.fakeDMSDir = ""
	}

	return &empty.Empty{}, lastErr
}
