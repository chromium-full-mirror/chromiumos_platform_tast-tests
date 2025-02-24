// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package unigraf provides support for interacting with a unigraf utc274 tester.
package unigraf

import (
	"context"
	"time"

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
func New(ctx context.Context, uri string) (*UsbTester, error) {

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
		uri:    uri,
	}
	openctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	testers, err := ctl.client.GetTesters(openctx, &passport.GetTestersRequest{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get testers uri=%s", uri)
	}

	// For the moment there is a maximum of 1 usb tester per setup.
	if len(testers.Testers) != 1 {
		return nil, errors.Errorf(
			"the tester selection is ambiguous, there are %d testers",
			len(testers.Testers),
		)
	}
	ctl.tester = testers.Testers[0].Id

	if _, err := ctl.client.OpenTester(openctx, &passport.OpenTesterRequest{Id: ctl.tester}); err != nil {
		return nil, errors.Wrapf(err, "failed to open serial=%s for uri=%s", uri, ctl.tester)
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

// doCapabilitySetRequest is a internal helper method that does the actual grpc request.
func (s *UsbTester) doCapabilitySetRequest(
	ctx context.Context,
	req *passport.SetUsbTesterCapabilityRequest,
) error {
	reqctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req.Id = s.tester

	reply, err := s.client.SetTesterCapability(reqctx, req)

	if err != nil || (reply.GetErrCode() != 0) {
		return errors.Wrapf(
			err,
			"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg(),
		)
	}

	return nil
}

// SetPowerRole will set the data role to the role passed in the argument.
func (s *UsbTester) SetPowerRole(ctx context.Context, role passport.PowerRole) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Id:         s.tester,
			Capability: passport.Capability_POWER_ROLE,
			Value: &passport.SetUsbTesterCapabilityRequest_PowerRole{
				PowerRole: role,
			},
		},
	)
}

// SetDataRole will set the data role to ufp.
func (s *UsbTester) SetDataRole(ctx context.Context, role passport.DataRole) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_DATA_ROLE,
			Value: &passport.SetUsbTesterCapabilityRequest_DataRole{
				DataRole: role,
			},
		},
	)
}

// SetUsbChannel will set the usb channel, either 2.0 or 3.0.
func (s *UsbTester) SetUsbChannel(ctx context.Context, channel passport.UsbChannel) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_USB_CHANNEL,
			Value: &passport.SetUsbTesterCapabilityRequest_UsbChannel{
				UsbChannel: channel,
			},
		},
	)
}

// SetPinAssignment will set the display port alternate mode pin assignment.
func (s *UsbTester) SetPinAssignment(ctx context.Context, pinMode passport.PinAassignment) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_PIN_ASSIGMENT,
			Value: &passport.SetUsbTesterCapabilityRequest_PinMode{
				PinMode: pinMode,
			},
		},
	)
}

// SetInitPdState will set the initial PD state.
func (s *UsbTester) SetInitPdState(ctx context.Context, state passport.InitPdState) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_INIT_PD_STATE,
			Value: &passport.SetUsbTesterCapabilityRequest_InitPdState{
				InitPdState: state,
			},
		},
	)
}

// SetSnkPdoCount will set the number of snk pdos.
func (s *UsbTester) SetSnkPdoCount(ctx context.Context, cnt int64) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_SNK_PDO_COUNT,
			Value: &passport.SetUsbTesterCapabilityRequest_NonDescrete{
				NonDescrete: cnt,
			},
		},
	)
}

// SetSrcPdoCount will set the number of src pdos.
func (s *UsbTester) SetSrcPdoCount(ctx context.Context, cnt int64) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_SRC_PDO_COUNT,
			Value: &passport.SetUsbTesterCapabilityRequest_NonDescrete{
				NonDescrete: cnt,
			},
		},
	)
}

// doCapabilityGetRequest is a internal helper method that does the actual grpc request.
func (s *UsbTester) doCapabilityGetRequest(
	ctx context.Context,
	req *passport.GetUsbTesterCapabilityRequest,
) (*passport.GetUsbTesterCapabilityReply, error) {
	reqctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req.Id = s.tester

	reply, err := s.client.GetTesterCapability(reqctx, req)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get capability member")
	}

	return reply, nil
}

// DataRole will return the current data role on the testers.
func (s *UsbTester) DataRole(ctx context.Context) (passport.DataRole, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_DATA_ROLE},
	)

	return reply.GetDataRole(), err
}

// PowerRole will return the current power role on the testers.
func (s *UsbTester) PowerRole(ctx context.Context) (passport.PowerRole, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_POWER_ROLE},
	)

	return reply.GetPowerRole(), err
}

// UsbChannel will return the usb channel, either 2.0 or 3.0.
func (s *UsbTester) UsbChannel(ctx context.Context) (passport.UsbChannel, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_USB_CHANNEL},
	)

	return reply.GetUsbChannel(), err
}

// PinAssignment will return the display port alternate mode pin assignment.
func (s *UsbTester) PinAssignment(ctx context.Context) (passport.PinAassignment, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_PIN_ASSIGMENT},
	)

	return reply.GetPinMode(), err
}

// InitPdState will return the starting state of power delivery.
func (s *UsbTester) InitPdState(ctx context.Context) (passport.InitPdState, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_DATA_ROLE},
	)

	return reply.GetInitPdState(), err
}

// SnkPdoCount will return get the number of snk pdos.
func (s *UsbTester) SnkPdoCount(ctx context.Context) (int64, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_SNK_PDO_COUNT},
	)

	return reply.GetNonDescrete(), err
}

// SrcPdoCount will return get the number of src pdos.
func (s *UsbTester) SrcPdoCount(ctx context.Context) (int64, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_SRC_PDO_COUNT},
	)

	return reply.GetNonDescrete(), err
}

// DpInfo will get the display port information.
func (s *UsbTester) DpInfo(ctx context.Context) (*passport.GetDpInfoReply, error) {

	reply, err := s.client.GetDpInfo(
		ctx,
		&passport.GetDpInfoRequest{
			Id: s.tester,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get DP information")
	}

	return reply, nil
}

// Replug the cable. This simulates cable replug, it isn't an actual cable replug.
func (s *UsbTester) Replug(ctx context.Context) error {

	reply, err := s.client.ReplugCable(
		ctx,
		&passport.DoCableReplugRequest{
			Id: s.tester,
		},
	)
	if err != nil || reply.GetErrCode() != 0 {
		return errors.Wrapf(
			err,
			"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg(),
		)
	}

	return nil
}

// HardReset the pd state.
func (s *UsbTester) HardReset(ctx context.Context) error {
	reply, err := s.client.HardResetTester(
		ctx,
		&passport.HardResetTesterRequest{
			Id: s.tester,
		},
	)
	if err != nil || reply.GetErrCode() != 0 {
		return errors.Wrapf(
			err,
			"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg(),
		)
	}

	return nil
}

// SetTestPort will set the active test port.
func (s *UsbTester) SetTestPort(ctx context.Context, portID int64) error {
	reply, err := s.client.SetActivePort(
		ctx,
		&passport.SetActivePortRequest{
			Id:     s.tester,
			PortId: uint32(portID),
		},
	)
	if err != nil || reply.GetErrCode() != 0 {
		return errors.Wrapf(
			err,
			"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg(),
		)
	}

	return nil
}

// TestPort will get the active test port.
func (s *UsbTester) TestPort(ctx context.Context) (int64, error) {
	reply, err := s.client.GetActivePort(
		ctx,
		&passport.GetActivePortRequest{
			Id: s.tester,
		},
	)
	if err != nil || reply.GetErrCode() != 0 || reply.GetMaxNumPorts() == 0 {
		return 0, errors.Wrapf(
			err,
			"failed to do get request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg(),
		)
	}

	return int64(reply.GetPortId()), nil
}
