// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/bluetooth/facade"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterBluetoothServiceServer(srv, &BtService{
				s: s,
			})
		},
		// GuaranteeCompatibility allows non-Tast test harness clients to call this service.
		GuaranteeCompatibility: true,
	})
}

// BtService implements tast.cros.bluetooth.BluetoothService.
type BtService struct {
	s      *testing.ServiceState
	facade common.BluetoothFacade
}

// SetBluetoothStack configures the DUT to use the specified bluetooth stack
// and initializes the BluetoothFacade as needed.
//
// Enable should be called after this if to ensure the adapter is powered on
// if it is to be used beyond calling IsPoweredOn.
func (b *BtService) SetBluetoothStack(ctx context.Context, request *pb.SetBluetoothStackRequest) (*emptypb.Empty, error) {
	var stackType common.BluetoothStackType
	switch request.StackType {
	case pb.BluetoothStackType_BLUETOOTH_STACK_TYPE_BLUEZ:
		stackType = common.BluetoothStackTypeBluez
	case pb.BluetoothStackType_BLUETOOTH_STACK_TYPE_FLOSS:
		stackType = common.BluetoothStackTypeFloss
	default:
		return nil, errors.Errorf("invalid request stackType %d", request.StackType)
	}
	newFacade, err := facade.NewBluetoothFacade(ctx, stackType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s bluetooth facade", stackType)
	}
	b.facade = newFacade
	return &emptypb.Empty{}, nil
}

func (b *BtService) assertHasFacade() error {
	if b.facade == nil {
		return errors.New("facade not configured; call SetBluetoothStack prior to calling this method")
	}
	return nil
}

// StackType returns the BluetoothStackType the facade uses.
func (b *BtService) StackType(ctx context.Context, request *emptypb.Empty) (*pb.StackTypeResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	response := &pb.StackTypeResponse{}
	switch b.facade.StackType() {
	case common.BluetoothStackTypeBluez:
		response.StackType = pb.BluetoothStackType_BLUETOOTH_STACK_TYPE_BLUEZ
	case common.BluetoothStackTypeFloss:
		response.StackType = pb.BluetoothStackType_BLUETOOTH_STACK_TYPE_FLOSS
	default:
		return nil, errors.Errorf("bluetooth facade has unknown stack type %q", b.facade.StackType())
	}
	return response, nil
}

// Enable will turn on the bluetooth daemons and power on adapter.
func (b *BtService) Enable(ctx context.Context, request *emptypb.Empty) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.Enable(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Disable will turn off the bluetooth daemons and power off adapter.
func (b *BtService) Disable(ctx context.Context, request *emptypb.Empty) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.Disable(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Reset resets the default adapter and turns it back on if powerOn is true.
func (b *BtService) Reset(ctx context.Context, request *pb.ResetRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.Reset(ctx, request.PowerOn); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// AdapterProperties collects and returns the default adapter's properties.
func (b *BtService) AdapterProperties(ctx context.Context, request *emptypb.Empty) (*pb.AdapterPropertiesResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	props, err := b.facade.AdapterProperties(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.AdapterPropertiesResponse{
		AdapterProperties: &pb.BluetoothAdapterProperties{
			Address:      props.Address,
			Name:         props.Name,
			Powered:      props.Powered,
			Discoverable: props.Discoverable,
			Discovering:  props.Discovering,
			Pairable:     props.Pairable,
		},
	}, nil
}

// IsPoweredOn returns true if the default adapter is enabled.
func (b *BtService) IsPoweredOn(ctx context.Context, request *emptypb.Empty) (*pb.IsPoweredOnResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	isPoweredOn, err := b.facade.IsPoweredOn(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.IsPoweredOnResponse{
		IsPoweredOn: isPoweredOn,
	}, nil
}

// SetPowered sets the default adapter's enabled state.
func (b *BtService) SetPowered(ctx context.Context, request *pb.SetPoweredRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.SetPowered(ctx, request.Powered); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// StartDiscovery starts the discovery of remote devices.
func (b *BtService) StartDiscovery(ctx context.Context, request *emptypb.Empty) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.StartDiscovery(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// StopDiscovery stops the discovery of remote devices.
func (b *BtService) StopDiscovery(ctx context.Context, request *emptypb.Empty) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.StopDiscovery(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// IsDiscovering checks if the adapter is discovering.
func (b *BtService) IsDiscovering(ctx context.Context, request *emptypb.Empty) (*pb.IsDiscoveringResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	isDiscovering, err := b.facade.IsDiscovering(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.IsDiscoveringResponse{
		IsDiscovering: isDiscovering,
	}, nil
}

// IsDiscoverable returns true if the default adapter is discoverable.
func (b *BtService) IsDiscoverable(ctx context.Context, request *emptypb.Empty) (*pb.IsDiscoverableResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	isDiscoverable, err := b.facade.IsDiscoverable(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.IsDiscoverableResponse{
		IsDiscoverable: isDiscoverable,
	}, nil
}

// SetDiscoverable sets whether the default adapter is discoverable.
func (b *BtService) SetDiscoverable(ctx context.Context, request *pb.SetDiscoverableRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.SetDiscoverable(ctx, request.Discoverable); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// SetDiscoverableWithTimeout sets whether the default adapter is discoverable.
// If discoverable is true, it will be set as discoverable for the provided
// timeout in seconds.
func (b *BtService) SetDiscoverableWithTimeout(ctx context.Context, request *pb.SetDiscoverableWithTimeoutRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.SetDiscoverableWithTimeout(ctx, request.Discoverable, request.TimeoutSeconds); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// IsPairable returns true if the default adapter is pairable.
func (b *BtService) IsPairable(ctx context.Context, request *emptypb.Empty) (*pb.IsPairableResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	isPairable, err := b.facade.IsPairable(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.IsPairableResponse{
		IsPairable: isPairable,
	}, nil
}

// SetPairable sets whether the default adapter is pairable.
func (b *BtService) SetPairable(ctx context.Context, request *pb.SetPairableRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.SetPairable(ctx, request.Pairable); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Address returns the address of the default adapter.
func (b *BtService) Address(ctx context.Context, request *emptypb.Empty) (*pb.AddressResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	adapterAddress, err := b.facade.Address(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.AddressResponse{
		AdapterAddress: adapterAddress,
	}, nil
}

// Name returns the name of the default adapter.
func (b *BtService) Name(ctx context.Context, request *emptypb.Empty) (*pb.NameResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	adapterName, err := b.facade.Name(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.NameResponse{
		AdapterName: adapterName,
	}, nil
}

// UUIDs returns the UUIDs of the default adapter.
func (b *BtService) UUIDs(ctx context.Context, request *emptypb.Empty) (*pb.UUIDsResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	adapterUUIDs, err := b.facade.UUIDs(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.UUIDsResponse{
		AdapterUUIDs: adapterUUIDs,
	}, nil
}

// Devices returns the addresses of all known devices.
func (b *BtService) Devices(ctx context.Context, request *emptypb.Empty) (*pb.DevicesResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	knownDeviceAddresses, err := b.facade.Devices(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.DevicesResponse{
		KnownDeviceAddresses: knownDeviceAddresses,
	}, nil
}

// HasDevice checks if the adapter knows the device with the given address.
// A device is known if is paired or if it has been discovered by the adapter.
func (b *BtService) HasDevice(ctx context.Context, request *pb.HasDeviceRequest) (*pb.HasDeviceResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	hasDevice, err := b.facade.HasDevice(ctx, request.DeviceAddress)
	if err != nil {
		return nil, err
	}
	return &pb.HasDeviceResponse{
		HasDevice: hasDevice,
	}, nil
}

// RemoveDevice removes/forgets a known device with the given address.
func (b *BtService) RemoveDevice(ctx context.Context, request *pb.RemoveDeviceRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.RemoveDevice(ctx, request.DeviceAddress); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ConnectDevice connects to a device with the given address.
func (b *BtService) ConnectDevice(ctx context.Context, request *pb.ConnectDeviceRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.ConnectDevice(ctx, request.DeviceAddress); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DisconnectDevice disconnects the device with the given address.
func (b *BtService) DisconnectDevice(ctx context.Context, request *pb.DisconnectDeviceRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.DisconnectDevice(ctx, request.DeviceAddress); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DeviceProperties collects and returns a known device's properties.
func (b *BtService) DeviceProperties(ctx context.Context, request *pb.DevicePropertiesRequest) (*pb.DevicePropertiesResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	props, err := b.facade.DeviceProperties(ctx, request.DeviceAddress)
	if err != nil {
		return nil, err
	}
	return &pb.DevicePropertiesResponse{
		DeviceProperties: &pb.BluetoothDeviceProperties{
			Address:   props.Address,
			Name:      props.Name,
			Alias:     props.Alias,
			Connected: props.Connected,
			Paired:    props.Paired,
		},
	}, nil
}

// DeviceIsConnected checks whether the device with the given address is
// connected.
func (b *BtService) DeviceIsConnected(ctx context.Context, request *pb.DeviceIsConnectedRequest) (*pb.DeviceIsConnectedResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	deviceIsConnected, err := b.facade.DeviceIsConnected(ctx, request.DeviceAddress)
	if err != nil {
		return nil, err
	}
	return &pb.DeviceIsConnectedResponse{
		DeviceIsConnected: deviceIsConnected,
	}, nil
}

// DeviceIsPaired checks whether the device with the given address is paired.
func (b *BtService) DeviceIsPaired(ctx context.Context, request *pb.DeviceIsPairedRequest) (*pb.DeviceIsPairedResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	deviceIsPaired, err := b.facade.DeviceIsPaired(ctx, request.DeviceAddress)
	if err != nil {
		return nil, err
	}
	return &pb.DeviceIsPairedResponse{
		DeviceIsPaired: deviceIsPaired,
	}, nil
}

// DeviceAddress returns the address of a known device with the given name.
func (b *BtService) DeviceAddress(ctx context.Context, request *pb.DeviceAddressRequest) (*pb.DeviceAddressResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	deviceAddress, err := b.facade.DeviceAddress(ctx, request.DeviceName)
	if err != nil {
		return nil, err
	}
	return &pb.DeviceAddressResponse{
		DeviceAddress: deviceAddress,
	}, nil
}

// DeviceName returns the name of a known device with the given address.
func (b *BtService) DeviceName(ctx context.Context, request *pb.DeviceNameRequest) (*pb.DeviceNameResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	deviceName, err := b.facade.DeviceName(ctx, request.DeviceAddress)
	if err != nil {
		return nil, err
	}
	return &pb.DeviceNameResponse{
		DeviceName: deviceName,
	}, nil
}

// DeviceAlias returns the alias of a known device with the given address.
func (b *BtService) DeviceAlias(ctx context.Context, request *pb.DeviceAliasRequest) (*pb.DeviceAliasResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	deviceAlias, err := b.facade.DeviceAlias(ctx, request.DeviceAddress)
	if err != nil {
		return nil, err
	}
	return &pb.DeviceAliasResponse{
		DeviceAlias: deviceAlias,
	}, nil
}

// DiscoverDevice will start discovery, wait until a device is found, and then
// stop discovery. A non-nil error return indicates that the adapter was able
// to successfully discover the device before the timeout was reached.
func (b *BtService) DiscoverDevice(ctx context.Context, request *pb.DiscoverDeviceRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.DiscoverDevice(ctx, request.DeviceAddress, request.DiscoveryTimeout.AsDuration()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// PairDevice pairs a peer device with the given address and authentication
// pin.
//
// DiscoverDevice should be called prior to PairDevice to ensure that the
// adapter knows of the device for pairing.
func (b *BtService) PairDevice(ctx context.Context, request *pb.PairDeviceRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.PairDevice(ctx, request.DeviceAddress, request.Pin); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// EnabledOnBoot returns the value of the system setting that determines if
// the bluetooth adapter is enabled on boot.
// Note: This requires a chrome user to be currently logged in.
func (b *BtService) EnabledOnBoot(ctx context.Context, empty *emptypb.Empty) (*pb.EnabledOnBootResponse, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	enabledOnBoot, err := b.facade.EnabledOnBoot(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.EnabledOnBootResponse{
		AdapterEnabledOnBoot: enabledOnBoot,
	}, nil
}

// SetEnabledOnBoot sets the value of the system setting that determines if
// the bluetooth adapter is enabled on boot.
// Note: This requires a chrome user to be currently logged in.
func (b *BtService) SetEnabledOnBoot(ctx context.Context, request *pb.SetEnabledOnBootRequest) (*emptypb.Empty, error) {
	if err := b.assertHasFacade(); err != nil {
		return nil, err
	}
	if err := b.facade.SetEnabledOnBoot(ctx, request.AdapterEnabledOnBoot); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// SetDebugLogLevels sets the debug log level.
// Note: Setting log level is only supported for bluez at this time.
func (b *BtService) SetDebugLogLevels(ctx context.Context, request *pb.SetDebugLogLevelsRequest) (*emptypb.Empty, error) {
	if err := b.facade.SetDebugLogLevels(ctx, request.GetLevel()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
