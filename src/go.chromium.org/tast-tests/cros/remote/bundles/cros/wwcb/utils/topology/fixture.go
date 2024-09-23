// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package topology

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/api"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

const (
	setupTimeout    = 2 * time.Minute
	preTestTimeout  = 2 * time.Minute
	postTestTimeout = 2 * time.Minute
)

type topologyParamVal struct {
	defaultTopology func(*testing.FixtState, string) *labapi.PasitHost
}

var topologyFileVar = testing.RegisterVarString(
	"topology.file",
	"",
	"A textproto file containing the PASIT testbed topology when running the test manually.",
)

var rpcServicePortVar = testing.RegisterVarString(
	"topology.apiPort",
	"8300",
	"A string containing the port that the passport API uses when running in a docker image.",
)

var rpcServiceHostVar = testing.RegisterVarString(
	"topology.apiHost",
	"",
	"A string containing the host that the passport API is running on.",
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "wwcb",
		Desc:            "PASIT fixtures that manage testbed topology",
		Contacts:        []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent:    "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Impl:            &TestFixture{},
		SetUpTimeout:    setupTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"USBID", "ExtCameraID", "DockingID", "ExtDispID1", "ExtDispID2", "EthernetID", "USBTypeAIDArray"},
		Params: []testing.FixtureParam{
			{
				Name: "storage",
				Val:  topologyParamVal{defaultTopology: defaultStorageTopology},
			},
			{
				Name:   "storageEnableServoAndDisableTabletMode",
				Parent: "enableServoAndDisableTabletMode",
				Val:    topologyParamVal{defaultTopology: defaultStorageTopology},
			},
			{
				Name:   "storageEnableServoAndTabletMode",
				Parent: "enableServoAndTabletMode",
				Val:    topologyParamVal{defaultTopology: defaultStorageTopology},
			},
			{
				Name: "camera",
				Val:  topologyParamVal{defaultTopology: defaultCameraTopology},
			},
			{
				Name: "display",
				Val:  topologyParamVal{defaultTopology: defaultDisplayTopology},
			},
			{
				Name: "dock",
				Val:  topologyParamVal{defaultTopology: defaultFullTopology},
			},
		},
	})
}

var ipPowerPorts = []int{1}

var marshaller = prototext.MarshalOptions{
	Multiline: true,
	Indent:    " ",
}

var unmarshaller = prototext.UnmarshalOptions{
	DiscardUnknown: true,
}

func varOrDefault(s *testing.FixtState, varName, defaultValue string) string {
	if val, ok := s.Var(varName); ok {
		return val
	}
	return defaultValue
}

func defaultStorageTopology(s *testing.FixtState, hostname string) *labapi.PasitHost {
	usbID := varOrDefault(s, "USBID", "2001902")
	return DefaultStorageTopology(hostname, usbID)
}

func defaultCameraTopology(s *testing.FixtState, hostname string) *labapi.PasitHost {
	cameraID := varOrDefault(s, "ExtCameraID", "2001903")
	return DefaultCameraTopology(hostname, cameraID)
}

func defaultDisplayTopology(s *testing.FixtState, hostname string) *labapi.PasitHost {
	disp1ID := varOrDefault(s, "ExtDispID1", "2109001")
	disp2ID := varOrDefault(s, "ExtDispID2", "2109002")
	return DefaultDisplayTopology(hostname, disp1ID, disp2ID)
}

func defaultFullTopology(s *testing.FixtState, hostname string) *labapi.PasitHost {
	dockingID := varOrDefault(s, "DockingID", "1912901")
	disp1ID := varOrDefault(s, "ExtDispID1", "2007901")
	disp2ID := varOrDefault(s, "ExtDispID2", "2007902")
	ethID := varOrDefault(s, "EthernetID", "j45sw01")
	usbID := varOrDefault(s, "USBTypeAIDArray", "2001901")

	var usbs []string
	if usbID != "" {
		usbs = strings.Split(usbID, ",")
	}

	return DefaultFullTopology(hostname, dockingID, disp1ID, disp2ID, ethID, usbs...)
}

// TestFixture is the PASIT test fixture.
type TestFixture struct {
	Helper          *Helper
	defaultTopology func(*testing.FixtState, string) *labapi.PasitHost
	hostConn        *ssh.Conn
	hostForwarder   *ssh.Forwarder
	grpcConn        *grpc.ClientConn
	switchService   api.SwitchService
	cameraService   api.CameraService
	CameraHelper    *api.CameraServiceHelper
}

// SetUp configures the fixture.
func (tf *TestFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	hostname := s.DUT().HostName()
	if host, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = host
	}

	var pasitTopology *labapi.PasitHost
	if topologyFileVar.Value() != "" {
		rawText, err := os.ReadFile(topologyFileVar.Value())
		if err != nil {
			s.Fatal("Failed to read topology file: ", err)
		}

		pasitTopology = &labapi.PasitHost{}
		if err := unmarshaller.Unmarshal(rawText, pasitTopology); err != nil {
			s.Fatal("Failed to unmarshal topology file: ", err)
		}
		s.Log("Loaded DUT info from textproto file")
	} else if dutConfig, err := s.ChromeOSDUTLabConfig(""); err == nil {
		if dutConfig.GetChromeos().GetPasitHost() != nil {
			pasitTopology = dutConfig.GetChromeos().GetPasitHost()
			s.Log("Loaded DUT info from lab config")
		}
	}

	// No dut topology defined, use default.
	if pasitTopology == nil {
		params := s.Param().(topologyParamVal)
		pasitTopology = params.defaultTopology(s, hostname)
		s.Log("Loaded DUT info from CLI args")
	}

	s.Log("Saving topology to topology.textproto")
	if err := os.WriteFile(filepath.Join(s.OutDir(), "topology.textproto"), []byte(marshaller.Format(pasitTopology)), 0644); err != nil {
		// Non fatal error.
		s.Log("Failed to save topology.textproto: ", err)
	}

	s.Log("Using topology: ", marshaller.Format(pasitTopology))

	// Connect to PASIT Host service.
	apiHostname := rpcServiceHostVar.Value()
	if apiHostname == "" {
		apiHostname = pasitTopology.GetHostname()
	}
	if apiHostname != "" {
		s.Logf("Connecting to pasit host: %q", apiHostname)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return tf.connectToGrpcServices(ctx, s, apiHostname)
		}, &testing.PollOptions{Timeout: 60 * time.Second}); err != nil {
			s.Fatal("Failed to connect to passport host: ", err)
		}

		// Create the service clients.
		tf.switchService = passport.NewSwitchServiceClient(tf.grpcConn)
		tf.cameraService = passport.NewCameraServiceClient(tf.grpcConn)
	} else {
		// No host info provided, assume that USB devices are connected to the local host.
		s.Log("Using local pasit host")
		switchService, err := api.NewLocalSwitchService(ctx)
		if err != nil {
			s.Fatal("Failed to create local switch service: ", err)
		}
		tf.switchService = switchService

		cameraService, err := api.NewLocalCameraService(ctx)
		if err != nil {
			s.Fatal("Failed to create local camera service: ", err)
		}
		tf.cameraService = cameraService
	}

	tf.Helper = NewHelper(pasitTopology, hostname, tf.switchService)
	tf.CameraHelper = api.NewCameraServiceHelper(tf.cameraService)
	return tf
}

// Reset does nothing currently, but is required for the test fixture.
func (tf *TestFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest initializes the test fixture before each test run.
func (tf *TestFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Initialize fixtures to find the connected devices.
	if err := tf.Helper.InitializeFixtures(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	if err := tf.CameraHelper.InitializeCameras(ctx); err != nil {
		s.Fatal("Failed to initialize cameras: ", err)
	}

	// Try to power cycle IP power for DUTs that have it.
	if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
		// Just log errors here since this may not always be provided.
		// Later this should be moved into the proto.
		s.Log("Failed to power on the docking station: ", err)
	}
}

// PostTest cleans up the test fixture after each test run.
func (tf TestFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	tf.Helper.ResetAll(ctx)
	utils.CloseIppower(ctx, ipPowerPorts)
}

// TearDown releases resources held open by the test fixture.
func (tf *TestFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

// ConnectPeripheralsViaDock connects the peripherals via the dock, verifies each connection and returns the list of USB devices.
// Peripherals devices such as external display, ethernet, USB (audio) devices.
func (tf *TestFixture) ConnectPeripheralsViaDock(ctx context.Context, dut *dut.DUT) (string, []string, error) {
	docks := tf.Helper.DevicesByType(DeviceTypeDockingStation)
	if len(docks) != 1 {
		return "", nil, errors.Errorf("failed to find dock, expected 1 docks got %d", len(docks))
	}

	dockID := docks[0]
	if _, err := tf.Helper.ActivateDeviceByTypeViaId(ctx, DeviceTypeMonitor, dockID); err != nil {
		return "", nil, errors.Wrap(err, "failed to connect monitor")
	}

	if _, err := tf.Helper.ActivateDeviceByTypeViaId(ctx, DeviceTypeNetwork, dockID); err != nil {
		return "", nil, errors.Wrap(err, "failed to connect ethernet")
	}

	usbTypeADeviceIDs := tf.Helper.DevicesByTypeViaId(DeviceTypeHID, dockID)
	for _, deviceID := range usbTypeADeviceIDs {
		if err := tf.Helper.ActivateDeviceByID(ctx, deviceID); err != nil {
			return "", nil, errors.Wrapf(err, "failed to connect to the USB Type-A device: %s", deviceID)
		}
	}

	usbDevices, err := utils.GetStableUSBDevices(ctx, dut)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to get a list of USB devices")
	}

	if err := utils.VerifyPeripheralsConnection(ctx, dut, true, usbDevices); err != nil {
		return "", nil, errors.Wrap(err, "failed to verify connections to the peripherals")
	}

	return dockID, usbDevices, nil
}

// VerifyDockingInterface verifies the docking interface is the same as the one in the capabilities.json file for meta tests.
func (tf *TestFixture) VerifyDockingInterface(ctx context.Context, dut *dut.DUT, capFile string) error {
	dockIDs := tf.Helper.DevicesByType(DeviceTypeDockingStation)
	if len(dockIDs) != 1 {
		return errors.Errorf("failed to determine dock ID, expected 1 docks, got %d", len(dockIDs))
	}

	dockingID := dockIDs[0]
	disable := func(ctx context.Context) error {
		if err := tf.Helper.DeactivateDeviceByID(ctx, dockingID); err != nil {
			return errors.Wrap(err, "failed to connect to the external storage")
		}
		return nil
	}

	enable := func(ctx context.Context) error {
		if err := tf.Helper.ActivateDeviceByID(ctx, dockingID); err != nil {
			return errors.Wrap(err, "failed to connect to the external storage")
		}
		return nil
	}

	if err := utils.VerifyDockingInterface(ctx, dut, capFile, disable, enable); err != nil {
		return errors.Wrap(err, "failed to verify the docking station interface")
	}
	return nil
}

// connectToGrpcServices connects to a passport API gRPC connection at the provided address.
//
// Address can be of the form:
//
//	<host>:<port> - The service is running at host:port so we should connect directly there
//		e.g. localhost:8300
//
//	<host>:<port>:docker:<container_name> - The service is running on the host in a docker container
//	so we should connect to the machine using the host:port and forward the docker container port locally.
//		e.g. chromeos1-row1-rack1-host1-apihost:22:docker:pasit-dev
//		e.g. localhost:2202:docker:pasit-dev
func (tf *TestFixture) connectToGrpcServices(ctx context.Context, s *testing.FixtState, address string) error {
	s.Logf("Connecting to PASIT host :%q", address)

	containerName := ""
	dockerParts := strings.SplitN(address, ":docker:", 2)
	if len(dockerParts) > 1 {
		address = dockerParts[0]
		containerName = dockerParts[1]
	}

	// If the container name is empty, then the address we're being given is
	// directly connectable, e.g. we forwarded the port locally or it is directly
	// available on the same network.
	if containerName == "" {
		grpcConn, err := grpc.Dial(address, grpc.WithInsecure())
		if err != nil {
			return errors.Wrapf(err, "failed to connect to btpeerd gRPC server %s", address)
		}
		tf.grpcConn = grpcConn
		return nil
	}

	// Open a ssh connection to the passport host.
	sshOptions := &ssh.Options{
		KeyDir:       s.DUT().KeyDir(),
		KeyFile:      s.DUT().KeyFile(),
		ProxyCommand: s.DUT().ProxyCommand(),
		Hostname:     address,
	}
	conn, err := ssh.New(ctx, sshOptions)
	if err != nil {
		return errors.Wrapf(err, "failed to connect to %s", sshOptions.Hostname)
	}
	tf.hostConn = conn

	// Get ip of container on the host.
	output, err := conn.CommandContext(ctx, "docker", "inspect", "-f", "{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}", containerName).Output()
	if err != nil {
		return errors.Wrap(err, "failed to find address of docker image")
	}

	// Forward the gRPC port to the local machine using the created ssh connection.
	onFwdError := func(err error) {
		testing.ContextLogf(ctx, "ERROR: passport host ssh error %s: %v", address, err)
	}
	remoteAddr := fmt.Sprintf("%s:%s", strings.TrimSpace(string(output)), rpcServicePortVar.Value())

	testing.ContextLogf(ctx, "Connecting to service at: %q", remoteAddr)
	portForwarder, err := conn.ForwardLocalToRemote("tcp", "localhost:0", remoteAddr, onFwdError)
	if err != nil {
		return errors.Wrapf(err, "failed to port forward PASIT host for port at %s", address)
	}
	tf.hostForwarder = portForwarder

	// Dial gRPC service.
	grpcConn, err := grpc.Dial(portForwarder.ListenAddr().String(), grpc.WithInsecure())
	if err != nil {
		return errors.Wrapf(err, "failed to connect to btpeerd gRPC server on %s through forwarded btpeerd port at %q", address, portForwarder.ListenAddr().String())
	}
	tf.grpcConn = grpcConn
	return nil
}
