// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package unigraf provides support for interacting with a unigraf utc274 tester.
package unigraf

import (
	"context"
	"strings"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"
	grpc "google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/usbutils/usbswitch"
	"go.chromium.org/tast/core/errors"
)

// PowerRole is a wrapper around passport.PowerRole
type PowerRole passport.PowerRole

// DataRole is a wrapper around passport.DataRole
type DataRole passport.DataRole

// UsbChannel is a wrapper around passport.UsbChannel
type UsbChannel passport.UsbChannel

// InitPdState is a wrapper around passport.InitPdState
type InitPdState passport.InitPdState

// ActiveCc is a wrapper around passport.ActiveCc
type ActiveCc passport.ActiveCc

// CableMode is a wrapper around passport.CableMode
type CableMode passport.CableMode

// Constants for PowerRole
const (
	PowerRoleNotSet PowerRole = PowerRole(passport.PowerRole_POWER_ROLE_NOT_SET)
	PowerRoleSnk    PowerRole = PowerRole(passport.PowerRole_SNK)
	PowerRoleSrc    PowerRole = PowerRole(passport.PowerRole_SRC)
)

func (x PowerRole) String() string {
	return passport.PowerRole(x).String()
}

// Constants for DataRole
const (
	DataRoleNotSet DataRole = DataRole(passport.DataRole_DATA_ROLE_NOT_SET)
	DataRoleUfp    DataRole = DataRole(passport.DataRole_DATA_UFP)
	DataRoleDfp    DataRole = DataRole(passport.DataRole_DATA_DFP)
)

func (x DataRole) String() string {
	switch x {
	case DataRoleUfp:
		return "UFP"
	case DataRoleDfp:
		return "DFP"
	default:
		return "Data role not set"
	}
}

// Constants for UsbChannel
const (
	UsbChannelNotSet   UsbChannel = UsbChannel(passport.UsbChannel_USB_CHANNEL_NOT_SET)
	UsbChannelUSB2     UsbChannel = UsbChannel(passport.UsbChannel_USB_2_HS)
	UsbChannelUSB3And2 UsbChannel = UsbChannel(passport.UsbChannel_USB_3_AND_2_HS)
)

func (x UsbChannel) String() string {
	switch x {
	case UsbChannelUSB2:
		return "USB2"
	case UsbChannelUSB3And2:
		return "USB3"
	default:
		return "USB channel not set"
	}
}

// Constants for InitPdState
const (
	InitPdStateNotSet InitPdState = InitPdState(passport.InitPdState_INIT_PD_STATE_NOT_SET)
	InitPdStateUfp    InitPdState = InitPdState(passport.InitPdState_PD_UFP)
	InitPdStateDfp    InitPdState = InitPdState(passport.InitPdState_PD_DFP)
	InitPdStateDrp    InitPdState = InitPdState(passport.InitPdState_PD_DRP)
)

func (x InitPdState) String() string {
	switch x {
	case InitPdStateUfp:
		return "UFP"
	case InitPdStateDfp:
		return "DFP"
	case InitPdStateDrp:
		return "DRP"
	default:
		return "Init PD state not set"
	}
}

// Constants for ActiveCc
const (
	ActiveCcNotSet ActiveCc = ActiveCc(passport.ActiveCc_ACTIVE_CC_NOT_SET)
	ActiveCc1      ActiveCc = ActiveCc(passport.ActiveCc_CC1)
	ActiveCc2      ActiveCc = ActiveCc(passport.ActiveCc_CC2)
)

func (x ActiveCc) String() string {
	switch x {
	case ActiveCc1:
		return "CC1"
	case ActiveCc2:
		return "CC2"
	default:
		return "Active CC not set"
	}
}

// Constants for CableMode
const (
	CableModeNotSet   CableMode = CableMode(passport.CableMode_CABLE_MODE_NOT_SET)
	CableModeNormal   CableMode = CableMode(passport.CableMode_NORMAL)
	CableModeElecTest CableMode = CableMode(passport.CableMode_ELEC_TEST)
)

// UsbTester is data type to model a unigraf utc274 usb tester.
type UsbTester struct {
	conn          *grpc.ClientConn
	client        passport.UsbTesterServiceClient
	tester        string
	uri           string
	switchPortNum int
}

// New Unigraf tester. It will connect to the the remote grcp server passed as
// an argument.
func New(ctx context.Context, uri, serial string, pasitTopology *labapi.PasitHost) (*UsbTester, error) {

	conn, err := grpc.Dial(uri, grpc.WithInsecure())
	if err != nil {
		return nil, errors.Errorf("failed to dial server uri=%s", uri)
	}

	client := passport.NewUsbTesterServiceClient(conn)
	if client == nil {
		return nil, errors.Errorf("failed create unigraf client but dial was OK uri=%s", uri)
	}

	ctl := &UsbTester{
		conn:          conn,
		client:        client,
		uri:           uri,
		switchPortNum: 0,
	}
	openctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	testers, err := ctl.client.GetTesters(openctx, &passport.GetTestersRequest{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get testers uri=%s", uri)
	}

	if pasitTopology != nil {
		for _, tester := range pasitTopology.GetDevices() {
			if tester.GetType() != labapi.PasitHost_Device_USB_TESTER {
				continue
			}
			ctl.tester = strings.ToUpper(tester.GetId())
			break
		}
	} else if serial != "" {
		ctl.tester = strings.ToUpper(serial)
	} else {
		if len(testers.Testers) != 1 {
			return nil, errors.Errorf("the tester selection is ambiguous, there are %d testers",
				len(testers.Testers))
		}
		// Default to the first tester so that we make local runs easier.
		ctl.tester = testers.Testers[0].Id
	}

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

	if _, err := s.client.CloseTester(ctx, &passport.CloseTesterRequest{Id: s.tester}); err != nil {
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
	reqctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req.Id = s.tester

	reply, err := s.client.SetTesterCapability(reqctx, req)

	if err != nil || (reply.GetErrCode() != 0) {
		return errors.Wrapf(err,
			"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg())

	}

	return nil
}

// SetPowerRole will set the data role to the role passed in the argument.
func (s *UsbTester) SetPowerRole(ctx context.Context, role PowerRole) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Id:         s.tester,
			Capability: passport.Capability_POWER_ROLE,
			Value: &passport.SetUsbTesterCapabilityRequest_PowerRole{
				PowerRole: passport.PowerRole(role),
			},
		},
	)
}

// SetDataRole will set the data role to ufp.
func (s *UsbTester) SetDataRole(ctx context.Context, role DataRole) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_DATA_ROLE,
			Value: &passport.SetUsbTesterCapabilityRequest_DataRole{
				DataRole: passport.DataRole(role),
			},
		},
	)
}

// SetUsbChannel will set the usb channel, either 2.0 or 3.0.
func (s *UsbTester) SetUsbChannel(ctx context.Context, channel UsbChannel) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_USB_CHANNEL,
			Value: &passport.SetUsbTesterCapabilityRequest_UsbChannel{
				UsbChannel: passport.UsbChannel(channel),
			},
		},
	)
}

// SetInitPdState will set the initial PD state.
func (s *UsbTester) SetInitPdState(ctx context.Context, state InitPdState) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_INIT_PD_STATE,
			Value: &passport.SetUsbTesterCapabilityRequest_InitPdState{
				InitPdState: passport.InitPdState(state),
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

// SetActiveCc will set the active CC.
func (s *UsbTester) SetActiveCc(ctx context.Context, cc ActiveCc) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_ACTIVE_CC,
			Value: &passport.SetUsbTesterCapabilityRequest_ActiveCc{
				ActiveCc: passport.ActiveCc(cc),
			},
		},
	)
}

// SetCableMode will set the cable mode.
func (s *UsbTester) SetCableMode(ctx context.Context, mode CableMode) error {
	return s.doCapabilitySetRequest(
		ctx,
		&passport.SetUsbTesterCapabilityRequest{
			Capability: passport.Capability_CABLE_MODE,
			Value: &passport.SetUsbTesterCapabilityRequest_CableMode{
				CableMode: passport.CableMode(mode),
			},
		},
	)
}

// doCapabilityGetRequest is a internal helper method that does the actual grpc request.
func (s *UsbTester) doCapabilityGetRequest(
	ctx context.Context,
	req *passport.GetUsbTesterCapabilityRequest,
) (*passport.GetUsbTesterCapabilityReply, error) {
	reqctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req.Id = s.tester

	reply, err := s.client.GetTesterCapability(reqctx, req)

	if err != nil {
		return nil, errors.Wrap(err, "failed to get capability member")
	}

	return reply, nil
}

// DataRole will return the current data role on the testers.
func (s *UsbTester) DataRole(ctx context.Context) (DataRole, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_DATA_ROLE},
	)

	return DataRole(reply.GetDataRole()), err
}

// PowerRole will return the current power role on the testers.
func (s *UsbTester) PowerRole(ctx context.Context) (PowerRole, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_POWER_ROLE},
	)

	return PowerRole(reply.GetPowerRole()), err
}

// UsbChannel will return the usb channel, either 2.0 or 3.0.
func (s *UsbTester) UsbChannel(ctx context.Context) (UsbChannel, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_USB_CHANNEL},
	)

	return UsbChannel(reply.GetUsbChannel()), err
}

// InitPdState will return the starting state of power delivery.
func (s *UsbTester) InitPdState(ctx context.Context) (InitPdState, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_DATA_ROLE},
	)

	return InitPdState(reply.GetInitPdState()), err
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

// VbusVoltage will return the Vbus voltage.
func (s *UsbTester) VbusVoltage(ctx context.Context) (int, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_VBUS_VOLTAGE},
	)

	return int(reply.GetNonDescrete()), err
}

// VbusCurrent will return the Vbus current.
func (s *UsbTester) VbusCurrent(ctx context.Context) (int, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_VBUS_CURRENT},
	)

	return int(reply.GetNonDescrete()), err
}

// ActiveCc will return the active Cc.
func (s *UsbTester) ActiveCc(ctx context.Context) (ActiveCc, error) {
	reply, err := s.doCapabilityGetRequest(
		ctx,
		&passport.GetUsbTesterCapabilityRequest{Capability: passport.Capability_ACTIVE_CC},
	)

	return ActiveCc(reply.GetActiveCc()), err
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
		return errors.Wrapf(err,
			"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg())

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
		return errors.Wrapf(err,
			"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg())

	}

	return nil
}

// SetTestPort will set the active test port.
func (s *UsbTester) SetTestPort(ctx context.Context, portID int) error {

	if currentPort, err := s.TestPort(ctx); err != nil {
		return errors.Wrap(err, "failed to get port")
	} else if currentPort == portID {
		return nil
	} else {
		req := &passport.SetActivePortRequest{
			Id:     s.tester,
			PortId: uint32(currentPort),
			State:  passport.PortState_PORT_STATE_OFF,
		}
		if reply, err := s.client.SetActivePort(ctx, req); err != nil || reply.GetErrCode() != 0 {
			return errors.Wrapf(
				err,
				"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
				reply.GetErrCode(),
				reply.GetErrorMsg(),
			)
		}
	}

	reply, err := s.client.SetActivePort(
		ctx,
		&passport.SetActivePortRequest{
			Id:     s.tester,
			PortId: uint32(portID),
			State:  passport.PortState_PORT_STATE_ON,
		},
	)
	if err != nil || reply.GetErrCode() != 0 {
		return errors.Wrapf(err,
			"failed to do set request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg())
	}

	return nil
}

// TestPort will get the active test port.
func (s *UsbTester) TestPort(ctx context.Context) (int, error) {
	reply, err := s.client.GetActivePort(
		ctx,
		&passport.GetActivePortRequest{
			Id: s.tester,
		},
	)
	if err != nil || reply.GetErrCode() != 0 || reply.GetMaxNumPorts() == 0 {
		return 0, errors.Wrapf(err,
			"failed to do get request, internal sdk error code was %d, internal sdk error message was %s",
			reply.GetErrCode(),
			reply.GetErrorMsg())
	}

	return int(reply.GetPortId()), nil
}

// USB switch interface implementation

// DisablePorts disables the device testing ports.
func (s *UsbTester) DisablePorts(ctx context.Context) error {
	//TODO(b:416456393): Implement this properly when turning off ports is supported.
	if s.switchPortNum == 1 {
		return s.SetTestPort(ctx, 0)
	}
	return s.SetTestPort(ctx, 1)
}

// EnablePort enables the used port.
func (s *UsbTester) EnablePort(ctx context.Context) error {
	return s.SetTestPort(ctx, s.switchPortNum)
}

// SetActiveSwitchPort sets the port affected by Enable/DisablePort actions.
func (s *UsbTester) SetActiveSwitchPort(ctx context.Context, portNum int) error {
	if portNum != 0 && portNum != 1 {
		return errors.Errorf("port number must be 0 or 1, got %d", portNum)
	}
	s.switchPortNum = portNum
	return nil
}

// GetType returns the type of the switch.
func (s *UsbTester) GetType() usbswitch.SwitchType {
	return usbswitch.UTC274
}

// EnterMode sets the mode of the device.
func (s *UsbTester) EnterMode(ctx context.Context, mode usbswitch.ConnectionMode) error {
	switch mode {
	case usbswitch.Usb2Mode:
		return s.SetUsbChannel(ctx, UsbChannelUSB2)
	case usbswitch.Usb3Mode:
		return s.SetUsbChannel(ctx, UsbChannelUSB3And2)
	case usbswitch.DpMode:
		return s.SetUsbChannel(ctx, UsbChannelUSB3And2)
	default:
		return errors.New("unsupported mode")
	}
}

// FlipOrientation sets the orientation of USB plug.
func (s *UsbTester) FlipOrientation(ctx context.Context, flipped bool) error {
	if err := s.SetCableMode(ctx, CableModeElecTest); err != nil {
		return errors.Wrap(err, "failed to set cable mode to elec test before flipping orientation")
	}

	if flipped {
		return s.SetActiveCc(ctx, ActiveCc2)
	}

	return s.SetActiveCc(ctx, ActiveCc1)
}
