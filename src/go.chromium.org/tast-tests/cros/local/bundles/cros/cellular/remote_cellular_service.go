// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/modemfwd"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/upstart"
	cellular_pb "go.chromium.org/tast-tests/cros/services/cros/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			cellular_pb.RegisterRemoteCellularServiceServer(srv, &RemoteCellularService{state: s})
		},
	})
}

// RemoteCellularService implements tast.cros.cellular.RemoteCellularService.
type RemoteCellularService struct {
	state           *testing.ServiceState
	helper          *cellular.Helper
	modemfwdStopped bool
	netUnlock       func()
}

// SetUp initialize the DUT for cellular testing.
func (s *RemoteCellularService) SetUp(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	// Give some time for cellular daemons to perform any modem operations. Stopping them via upstart might leave the modem in a bad state.
	if err := cellular.EnsureUptime(ctx, 2*time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to wait for system uptime")
	}

	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cellular.Helper")
	}
	s.helper = helper

	if s.modemfwdStopped, err = modemfwd.Stop(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to stop job: %q", modemfwd.JobName)
	}
	if s.modemfwdStopped {
		testing.ContextLogf(ctx, "Stopped %q", modemfwd.JobName)
	} else {
		testing.ContextLogf(ctx, "%q not running", modemfwd.JobName)
	}

	// make sure modem is using the correct SIM
	if _, err = modemmanager.NewModemWithSim(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find valid mm dbus object")
	}

	// wait for hermes to stabilize before leaving
	if upstart.JobExists(ctx, hermes.JobName) {
		if err := hermes.WaitForHermesIdle(ctx, 30*time.Second); err != nil {
			testing.ContextLog(ctx, "Could not confirm if Hermes is idle: ", err)
		}
	}
	return &empty.Empty{}, nil
}

// PreTest runs before every cellular test.
func (s *RemoteCellularService) PreTest(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	// Prevent check_ethernet.hook from interrupting test if network is temporarily
	// disabled. Automatically unlocked after 30 minutes, so unlock and lock it
	// between each test.
	if unlock, err := network.LockCheckNetworkHook(ctx); err != nil {
		s.netUnlock = nil
		// Just report error if fail to acquire lock, since it's not strictly required.
		testing.ContextLog(ctx, "Failed to lock the check network hook: ", err)
	} else {
		s.netUnlock = unlock
	}
	return &empty.Empty{}, nil
}

// PostTest runs after every cellular test.
func (s *RemoteCellularService) PostTest(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if s.netUnlock != nil {
		s.netUnlock()
	}
	return &empty.Empty{}, nil
}

// TearDown releases any held resources and reverts the changes made in SetUp.
func (s *RemoteCellularService) TearDown(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if s.helper == nil {
		return nil, errors.New("Cellular helper not available, SetUp must be called first")
	}

	if s.modemfwdStopped {
		if err := modemfwd.StartAndWaitForQuiescence(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to restart modemfwd")
		}
		testing.ContextLogf(ctx, "Started %q", modemfwd.JobName)
	}

	return &empty.Empty{}, nil
}

// Enable enables the cellular technology on the DUT.
func (s *RemoteCellularService) Enable(ctx context.Context, req *empty.Empty) (*cellular_pb.EnableResponse, error) {
	if s.helper == nil {
		return &cellular_pb.EnableResponse{}, errors.New("Cellular helper not available, SetUp must be called first")
	}

	elapsed, err := s.helper.Enable(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to enable cellular")
	}

	return &cellular_pb.EnableResponse{
		EnableTime: int64(elapsed),
	}, nil
}

// Disable disables the cellular technology on the DUT.
func (s *RemoteCellularService) Disable(ctx context.Context, req *empty.Empty) (*cellular_pb.DisableResponse, error) {
	if s.helper == nil {
		return nil, errors.New("Cellular helper not available, SetUp must be called first")
	}

	// make sure DUT is not connected to callbox before disabling as it can leave the callbox in a bad state
	// and cause issues when setting up the default bearer
	if err := s.helper.IsConnected(ctx); err == nil {
		if _, err := s.helper.Disconnect(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to disable cellular, unable to disconnect from current network")
		}
	}

	elapsed, err := s.helper.Disable(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to disable cellular")
	}

	return &cellular_pb.DisableResponse{
		DisableTime: int64(elapsed),
	}, nil
}

// Connect attempts to connect to the cellular service.
func (s *RemoteCellularService) Connect(ctx context.Context, req *empty.Empty) (*cellular_pb.ConnectResponse, error) {
	if s.helper == nil {
		return nil, errors.New("Cellular helper not available, SetUp must be called first")
	}

	elapsed, err := s.helper.ConnectToDefault(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to cellular service")
	}

	return &cellular_pb.ConnectResponse{
		ConnectTime: int64(elapsed),
	}, nil
}

// Disconnect attempts to disconnect from the cellular service.
func (s *RemoteCellularService) Disconnect(ctx context.Context, req *empty.Empty) (*cellular_pb.DisconnectResponse, error) {
	if s.helper == nil {
		return nil, errors.New("Cellular helper not available, SetUp must be called first")
	}

	elapsed, err := s.helper.Disconnect(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to disconnect from cellular service")
	}

	return &cellular_pb.DisconnectResponse{
		DisconnectTime: int64(elapsed),
	}, nil
}

// QueryService returns information about the available cellular service.
func (s *RemoteCellularService) QueryService(ctx context.Context, _ *empty.Empty) (*cellular_pb.QueryServiceResponse, error) {
	if s.helper == nil {
		return nil, errors.New("Cellular helper not available, SetUp must be called first")
	}

	service, err := s.helper.FindServiceForDevice(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cellular service")
	}

	props, err := service.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service properties")
	}

	name, err := props.GetString(shillconst.ServicePropertyName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service name from properties")
	}
	device, err := props.GetObjectPath(shillconst.ServicePropertyDevice)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service device from properties")
	}
	state, err := props.GetString(shillconst.ServicePropertyState)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service state from properties")
	}
	isConnected, err := props.GetBool(shillconst.ServicePropertyIsConnected)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service connection status from properties")
	}
	strength, err := props.GetUint8(shillconst.ServicePropertyStrength)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service strength from properties")
	}
	networkTechnology, err := props.GetString(shillconst.ServicePropertyCellularNetworkTechnology)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service network technology from properties")
	}

	return &cellular_pb.QueryServiceResponse{
		Name:              name,
		Device:            string(device),
		State:             state,
		IsConnected:       isConnected,
		Strength:          int64(strength),
		NetworkTechnology: networkTechnology,
	}, nil
}

// QueryLTESignal returns information about the attached LTE signal.
func (s *RemoteCellularService) QueryLTESignal(ctx context.Context, _ *empty.Empty) (*cellular_pb.QueryLTESignalResponse, error) {
	// make sure that there is an LTE network available
	if resp, err := s.QueryService(ctx, &empty.Empty{}); err != nil {
		return nil, errors.Wrap(err, "failed to get service properties")
	} else if resp.NetworkTechnology != shillconst.CellularNetworkTechnologyLTE {
		return nil, errors.Errorf("failed to find requested network: want %q, got %q", shillconst.CellularNetworkTechnologyLTE, resp.NetworkTechnology)
	}

	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create modem")
	}

	ph, err := dbusutil.NewPropertyHolder(ctx, modemmanager.DBusModemmanagerService, modemmanager.DBusModemmanagerSignalInterface, modem.ObjectPath())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create modem property holder")
	}

	// get all signal properties
	props, err := ph.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get modem properties")
	}

	props, err = props.GetMap(mmconst.SignalPropertyLte)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get LTE signal properties")
	}

	rsrp, err := props.GetFloat64(mmconst.SignalPropertyLteRsrp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get signal RSRP from properties")
	}
	rsrq, err := props.GetFloat64(mmconst.SignalPropertyLteRsrq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get signal RSRQ from properties")
	}
	snr, err := props.GetFloat64(mmconst.SignalPropertyLteSnr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get signal SNR from properties")
	}

	return &cellular_pb.QueryLTESignalResponse{
		Rsrp: rsrp,
		Rsrq: rsrq,
		Snr:  snr,
	}, nil
}

// QueryInterface returns information about the cellular device interface.
// Note: This method assumes that:
// 1. There is a unique Cellular Device.
// 2. The "interface" field of the Cellular Device corresponds to the data connection.
func (s *RemoteCellularService) QueryInterface(ctx context.Context, _ *empty.Empty) (*cellular_pb.QueryInterfaceResponse, error) {
	if s.helper == nil {
		return nil, errors.New("Cellular helper not available, SetUp must be called first")
	}

	props, err := s.helper.Device.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device properties")
	}

	// TODO: (b/294564933) Remove explicit dependency on interface name.
	iface, err := props.GetString(shillconst.DevicePropertyCellularPrimaryMultiplexedInterface)
	if err != nil || iface == "" {
		testing.ContextLog(ctx, "Failed to get primary multiplexed interface from properties, falling back to default")
		// If no multiplexed interface, default to regular device interface.
		if iface, err = props.GetString(shillconst.DevicePropertyInterface); err != nil {
			return nil, errors.Wrap(err, "failed to get device interface from properties")
		}
	}

	return &cellular_pb.QueryInterfaceResponse{
		Name: iface,
	}, nil
}

// DisableSar disables dynamic SAR on the DUT.
func (s *RemoteCellularService) DisableSar(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	return setSARStatus(ctx, false)
}

// EnableSar enables dynamic SAR on the DUT.
func (s *RemoteCellularService) EnableSar(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	return setSARStatus(ctx, true)
}

func setSARStatus(ctx context.Context, enabled bool) (*empty.Empty, error) {
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create modem")
	}

	sarInterface, err := modem.GetSARInterface(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get modem SAR interface")
	}

	if sarInterface.EnableSAR(ctx, enabled); err != nil {
		return nil, errors.Wrap(err, "failed to enable SAR")
	}

	return &empty.Empty{}, nil
}

// ConfigureSar configures the dynamic SAR power level on the DUT.
func (s *RemoteCellularService) ConfigureSar(ctx context.Context, req *cellular_pb.ConfigureSarRequest) (*empty.Empty, error) {
	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create modem")
	}

	sarInterface, err := modem.GetSARInterface(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get modem SAR interface")
	}

	if sarInterface.SetSARPowerLevel(ctx, uint32(req.PowerLevel)); err != nil {
		return nil, errors.Wrap(err, "failed to enable SAR")
	}

	return &empty.Empty{}, nil
}

// VerifyIPConnectivity verifies that the DUT has network connectivity using the provided addresses.
func (s *RemoteCellularService) VerifyIPConnectivity(ctx context.Context, req *cellular_pb.VerifyIPConnectivityRequest) (*cellular_pb.VerifyIPConnectivityResponse, error) {
	if s.helper == nil {
		return nil, errors.New("Cellular helper not available, SetUp must be called first")
	}

	if req.IPv4Address == "" {
		return nil, errors.New("no IPv4 address provided")
	}

	if req.IPv6Address == "" {
		return nil, errors.New("no IPv6 address provided")
	}

	ipv4, ipv6, err := s.helper.GetNetworkProvisionedCellularIPTypes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read network provisioned IP types")
	}

	testCon := func(ctx context.Context) error {
		if req.Method == cellular_pb.VerifyIPConnectivityRequest_Curl {
			return testConnectionCurl(ctx, ipv4, ipv6, req.IPv4Address, req.IPv6Address)
		}

		return testConnectionPing(ctx, ipv4, ipv6, req.IPv4Address, req.IPv6Address)
	}

	if err := s.helper.RunTestOnCellularInterface(ctx, testCon); err != nil {
		return nil, errors.Wrap(err, "failed to run test on cellular interface")
	}

	return &cellular_pb.VerifyIPConnectivityResponse{
		Ipv4: ipv4,
		Ipv6: ipv6,
	}, nil
}

func testConnectionPing(ctx context.Context, ipv4, ipv6 bool, ipv4Addr, ipv6Addr string) error {
	if !ipv4 && !ipv6 {
		return errors.New("failed to run ping test, neither IPv4 nor IPv6 connection available")
	}

	if ipv4 {
		if out, err := testexec.CommandContext(ctx, "ping", "-c1", "-w1", "-4", ipv4Addr).Output(); err != nil {
			return errors.Wrapf(err, "failed ipv4 ping to %q: %s", ipv4Addr, string(out))
		}
	}
	if ipv6 {
		if out, err := testexec.CommandContext(ctx, "ping", "-c1", "-w1", "-6", ipv6Addr).Output(); err != nil {
			return errors.Wrapf(err, "failed ipv6 ping to %q: %s", ipv6Addr, string(out))
		}
	}
	return nil
}

func testConnectionCurl(ctx context.Context, ipv4, ipv6 bool, ipv4Addr, ipv6Addr string) error {
	if !ipv4 && !ipv6 {
		return errors.New("failed to run curl test, neither IPv4 nor IPv6 connection available")
	}

	if ipv4 {
		if out, err := testexec.CommandContext(ctx, "curl", "-m", "5", "-4", ipv4Addr).Output(); err != nil {
			return errors.Wrapf(err, "failed ipv4 curl to %q: %s", ipv4Addr, string(out))
		}
	}

	if ipv6 {
		if out, err := testexec.CommandContext(ctx, "curl", "-m", "5", "-6", ipv6Addr).Output(); err != nil {
			return errors.Wrapf(err, "failed ipv6 curl to %q: %s", ipv6Addr, string(out))
		}
	}
	return nil
}

// WaitForNextSms waits until a single sms added signal is received.
func (s *RemoteCellularService) WaitForNextSms(ctx context.Context, _ *empty.Empty) (*cellular_pb.WaitForNextSmsResponse, error) {
	match := dbusutil.MatchSpec{
		Type:      "signal",
		Interface: modemmanager.DBusModemmanagerMessageInterface,
		Member:    "Added",
	}

	conn, err := dbusutil.SystemBus()
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to system bus")
	}

	signal, err := dbusutil.GetNextSignal(ctx, conn, match)
	if err != nil {
		return nil, errors.Wrap(err, "failed to receive SMS Added signal")
	}

	message, err := smsFromSignal(ctx, signal)
	if err != nil {
		return nil, errors.Wrap(err, "faild to get SMS message from signal")
	}
	return &cellular_pb.WaitForNextSmsResponse{Message: message}, nil
}

func smsFromSignal(ctx context.Context, signal *dbus.Signal) (*cellular_pb.SmsMessage, error) {
	if len(signal.Body) == 0 {
		return nil, errors.New("SMS signal body empty")
	}

	smsPath, ok := signal.Body[0].(dbus.ObjectPath)
	if !ok {
		return nil, errors.Errorf("failed to get SMS path from signal %v", signal.Body)
	}
	ph, err := dbusutil.NewPropertyHolder(ctx, modemmanager.DBusModemmanagerService, modemmanager.DBusModemmanagerSmsInterface, smsPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create SMS property holder")
	}

	props, err := ph.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get SMS properties")
	}

	text, err := props.GetString("Text")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get SMS text")
	}

	return &cellular_pb.SmsMessage{Text: text}, nil
}

// QueryModemType gets the modem variant on the dut from cros_config.
func (s *RemoteCellularService) QueryModemType(ctx context.Context, _ *empty.Empty) (*cellular_pb.QueryModemTypeResponse, error) {
	modemType, err := cellular.GetModemType(ctx)
	if err != nil && err.Error() != "Process exited with status 1" {
		return nil, errors.Wrap(err, "failed to run command cros_config on client host")
	}
	return &cellular_pb.QueryModemTypeResponse{ModemType: uint32(modemType)}, err
}

// QuerySignalBars gets the signal bars count from row item of UI for actively connected cellular network.
func (s *RemoteCellularService) QuerySignalBars(ctx context.Context, _ *empty.Empty) (*cellular_pb.QuerySignalBarsResponse, error) {
	count, err := cellular.GetSignalBarCount(ctx, s.helper)
	if err != nil && err.Error() != "Process exited with status 1" {
		return nil, errors.Wrap(err, "failed to find signal bars count from ui")
	}
	return &cellular_pb.QuerySignalBarsResponse{Count: int32(count)}, err
}
