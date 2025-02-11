// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package unigraf provides support for interacting with a unigraf utc274 tester.
package unigraf

import (
	"context"
	"time"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast/core/errors"

	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"
	grpc "google.golang.org/grpc"
)

// UsbTester is data type to model a unigraf utc274 usb tester.
type UsbTester struct {
	conn   *grpc.ClientConn
	client passport.UsbTesterServiceClient
	tester string
	uri    string
}

// New Unigraf tester. It will connect to the the remote grcp server passed as
// an argument.
func New(ctx context.Context, uri, testerID string) (*UsbTester, error) {

	conn, err := grpc.Dial(uri, grpc.WithInsecure())
	if err != nil {
		return nil, errors.Errorf("failed to dial server uri=%s", uri)
	}

	client := passport.NewUsbTesterServiceClient(conn)
	if client == nil {
		return nil, errors.Errorf("failed create unigraf client but dial was OK uri=%s", uri)
	}

	ctl := &UsbTester{
		conn:   conn,
		client: client,
		tester: testerID,
		uri:    uri,
	}
	openctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	testers, err := ctl.client.GetTesters(openctx, &passport.GetTestersRequest{})
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"failed to get testers uri=%s",
			uri,
		)
	}

	if !slices.ContainsFunc(testers.Testers, func(x *passport.UsbTester) bool {
		return x.Id == testerID
	}) {
		return nil, errors.Errorf("the given serial=%s is not present on the tester", testerID)
	}

	if _, err := ctl.client.OpenTester(openctx, &passport.OpenTesterRequest{Id: testerID}); err != nil {
		return nil, errors.Wrapf(
			err,
			"failed to open serial=%s for uri=%s",
			uri,
			testerID,
		)
	}

	return ctl, nil
}

// Close is used to close the serial of the tester. Defer it's execution.
func (s *UsbTester) Close(ctx context.Context) error {
	if s == nil || s.conn == nil || s.client == nil {
		// There was a problem opening the device, so we there is nothing to cleanup.
		return nil
	}

	delctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if _, err := s.client.CloseTester(delctx, &passport.CloseTesterRequest{Id: s.tester}); err != nil {
		return errors.Errorf("failed to close unigraf tester serial=%s, uri=%s", s.tester, s.uri)
	}

	if err := s.conn.Close(); err != nil {
		return errors.Wrap(err, "failed to close passport unigraf testing connection")
	}

	return nil
}

// PowerRole will return the current power role on the testers.
func (s *UsbTester) PowerRole(ctx context.Context) (passport.PowerRole, error) {
	reqctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	reply, err := s.client.GetTesterCapability(
		reqctx,
		&passport.GetUsbTesterCapabilityRequest{
			Id:         s.tester,
			Capability: passport.Capability_POWER_ROLE,
		},
	)

	if err != nil {
		return passport.PowerRole_POWER_ROLE_NOT_SET, errors.Wrap(err, "failed to get power role")
	}

	return reply.GetPowerRole(), nil
}

// PowerRoleSRC will set the power role to source.
func (s *UsbTester) PowerRoleSRC(ctx context.Context) error {
	return setPowerRole(ctx, s, passport.PowerRole_SRC)
}

// PowerRoleSNK will set the power role to source.
func (s *UsbTester) PowerRoleSNK(ctx context.Context) error {
	return setPowerRole(ctx, s, passport.PowerRole_SNK)
}

func setPowerRole(ctx context.Context, s *UsbTester, role passport.PowerRole) error {
	reqctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := s.client.SetTesterCapability(
		reqctx,
		&passport.SetUsbTesterCapabilityRequest{
			Id:         s.tester,
			Capability: passport.Capability_POWER_ROLE,
			Value: &passport.SetUsbTesterCapabilityRequest_PowerRole{
				PowerRole: role,
			},
		},
	)

	return err
}

// TogglePowerRole this will toggle the power role.
func (s *UsbTester) TogglePowerRole(ctx context.Context) error {

	val, err := s.PowerRole(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get data role while togglinh")
	}

	if val == passport.PowerRole_SNK {
		return s.PowerRoleSRC(ctx)
	}

	return s.PowerRoleSNK(ctx)
}

// DataRole will return the current data role on the testers.
func (s *UsbTester) DataRole(ctx context.Context) (passport.DataRole, error) {
	reqctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	reply, err := s.client.GetTesterCapability(
		reqctx,
		&passport.GetUsbTesterCapabilityRequest{
			Id:         s.tester,
			Capability: passport.Capability_DATA_ROLE,
		},
	)

	if err != nil {
		return passport.DataRole_DATA_ROLE_NOT_SET, errors.Wrap(err, "failed to get data role")
	}

	return reply.GetDataRole(), nil
}

// DataRoleUFP will set the data role to ufp.
func (s *UsbTester) DataRoleUFP(ctx context.Context) error {
	return setDataRole(ctx, s, passport.DataRole_DATA_UFP)
}

// DataRoleDFP will set the data role to dfp.
func (s *UsbTester) DataRoleDFP(ctx context.Context) error {
	return setDataRole(ctx, s, passport.DataRole_DATA_DFP)
}

func setDataRole(ctx context.Context, s *UsbTester, role passport.DataRole) error {
	reqctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := s.client.SetTesterCapability(
		reqctx,
		&passport.SetUsbTesterCapabilityRequest{
			Id:         s.tester,
			Capability: passport.Capability_DATA_ROLE,
			Value: &passport.SetUsbTesterCapabilityRequest_DataRole{
				DataRole: role,
			},
		},
	)

	return err
}
