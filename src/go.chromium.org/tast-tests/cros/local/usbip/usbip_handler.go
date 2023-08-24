// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"bytes"
	"fmt"
	"net"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/usbdevice"
	"go.chromium.org/tast/core/errors"
	"golang.org/x/sys/unix"
)

const (
	busPrefix  = "1-1."
	pathPrefix = "/sys/devices/pci0000:00/0000:00:1d.1/usb1/"
)

// Handler represents a connection handler that implements USBIP protocol.
type Handler struct {
	devices  []usbdevice.Device
	attached []bool
}

// NewHandler returns a new instance of UsbIpHandler.
func NewHandler() *Handler {
	usbIPHandler := &Handler{
		devices:  []usbdevice.Device{},
		attached: []bool{},
	}
	return usbIPHandler
}

// AddDevice adds a virtual device to the USBIP handler and returns its bus and path.
func (h *Handler) AddDevice(device usbdevice.Device) (string, string, uint32, uint32) {
	h.devices = append(h.devices, device)
	h.attached = append(h.attached, false)
	index := len(h.devices) - 1
	return getDeviceMetaByIndex(index)
}

// RemoveDevices removes all the devices from the USBIP handler.
func (h *Handler) RemoveDevices() {
	// HandleConnection is run in a separate go routine.
	// It could still be handling connections.
	h.devices = []usbdevice.Device{}
	h.attached = []bool{}
}

// HandleConnection handles USBIP communication.
func (h *Handler) HandleConnection(conn *net.Conn) error {
	header, err := usbdevice.ReadBE[opCommandHeader](*conn)
	if err != nil {
		return errors.Wrap(err, "unable to read UsbIpOpCommandHeader")
	}
	if header.CommandCode == commandOpReqDevlist {
		reply := newOpRepDevlist(h.devices)
		if err := usbdevice.WriteBE(*conn, reply); err != nil {
			return errors.Wrap(err, "unable to write newOpRepDevlist")
		}
	} else if header.CommandCode == commandOpReqImport {
		busID := make([]byte, 32)
		_, err := (*conn).Read(busID)
		if err != nil {
			return errors.Wrap(err, "unable to read bus id")
		}
		index, err := busToIndex(busID)
		if err != nil {
			return errors.Wrap(err, "unable to get device index")
		}
		device := h.devices[index]
		reply := newOpRepImport(device, index)
		if err := usbdevice.WriteBE(*conn, reply); err != nil {
			return errors.Wrap(err, "unable to write newOpRepImport")
		}
		h.attached[index] = true
		if err := handleCommands(conn, device.GetHandler()); err != nil {
			return errors.Wrap(err, "unable to handle device-level commands")
		}
		h.attached[index] = false
	}
	return nil
}

// handleCommands runs an infinite loop to handle device-level commands.
func handleCommands(conn *net.Conn, deviceHandler usbdevice.DeviceHandler) error {
	for {
		header, err := usbdevice.ReadBE[messageHeader](*conn)
		if err != nil {
			return errors.Wrap(err, "unable to read UsbIpMessageHeader")
		}
		if header.Command == commandSubmit {
			if err := handleCommandSubmit(conn, header, deviceHandler); err != nil {
				return errors.Wrap(err, "unable to handle USBIP_COMMAND_SUBMIT")
			}
		} else if header.Command == commandUnlink {
			if err := handleCommandUnlink(conn, header); err != nil {
				return errors.Wrap(err, "unable to handle USBIP_COMMAND_UNLINK")
			}
		} else {
			return errors.Errorf("unexpected command: %#v", header)
		}
	}
}

func busToIndex(bus []byte) (int, error) {
	index, err := strconv.Atoi(string(bytes.Trim(bus, "\x00"))[len(busPrefix):])
	return index, err
}

func getDeviceMetaByIndex(index int) (bus, path string, busnum, devnum uint32) {
	bus = fmt.Sprintf("%s%d", busPrefix, index)
	path = fmt.Sprintf("%s%s", pathPrefix, bus)
	// busnum:devnum of the shared device on the host.
	// As we emulate devices, these could be arbitrary numbers.
	// Pretend to have a single bus 1 on the host with device numbers matching their indices.
	busnum = 1
	devnum = uint32(index)
	return bus, path, busnum, devnum
}

func newOpRepDevlist(devices []usbdevice.Device) opRepDevlist {
	summaries := make([]DeviceSummary, len(devices))
	for index, device := range devices {
		bus, path, busnum, devnum := getDeviceMetaByIndex(index)
		summary := usbipSummary(device)
		summary.Header.Path = [256]byte{}
		copy(summary.Header.Path[:], []byte(path))
		summary.Header.BusID = [32]byte{}
		copy(summary.Header.BusID[:], []byte(bus))
		summary.Header.Busnum = busnum
		summary.Header.Devnum = devnum
		summaries[index] = summary
	}

	return opRepDevlist{
		Header: opCommandHeader{
			Version:     version,
			CommandCode: commandOpRepDevlist,
			Status:      statusOk,
		},
		NumDevices: uint32(len(devices)),
		Devices:    summaries,
	}
}

func newOpRepImport(device usbdevice.Device, index int) opRepImport {
	bus, path, busnum, devnum := getDeviceMetaByIndex(index)
	result := opRepImport{
		header: opCommandHeader{
			Version:     version,
			CommandCode: commandOpRepImport,
			Status:      statusOk,
		},
		device: usbipSummaryHeader(device),
	}

	result.device.Path = [256]byte{}
	copy(result.device.Path[:], []byte(path))
	result.device.BusID = [32]byte{}
	copy(result.device.BusID[:], []byte(bus))
	result.device.Busnum = busnum
	result.device.Devnum = devnum
	return result
}

func handleCommandSubmit(conn *net.Conn, header messageHeader, deviceHandler usbdevice.DeviceHandler) error {
	command, err := usbdevice.ReadBE[commandSubmitBody](*conn)
	if err != nil {
		return errors.Wrap(err, "unable to read UsbIpCommandSubmitBody")
	}

	transferBuffer := make([]byte, command.TransferBufferLength)

	// Reading transfer buffer in case of a not ISO packets.
	if header.Direction == dirOut && command.TransferBufferLength > 0 {
		if _, err := (*conn).Read(transferBuffer); err != nil {
			return errors.Wrap(err, "could not read transfer buffer")
		}
	}

	if err := deviceHandler.HandleUSBMessage(header.Endpoint, command.SetupBytes, transferBuffer); err != nil {
		return errors.Wrapf(err, "unable to handle command %#v", command)
	}

	replyHeader := messageHeader{
		Command:        commandRetSubmit,
		SequenceNumber: header.SequenceNumber,
		DeviceID:       header.DeviceID,
		Direction:      dirOut,
		Endpoint:       header.Endpoint,
	}
	replyBody := returnSubmitBody{
		Status:          0,
		ActualLength:    uint32(len(transferBuffer)),
		StartFrame:      0,
		NumberOfPackets: 0,
		ErrorCount:      0,
		Padding:         0,
	}
	if err := usbdevice.WriteBE(*conn, replyHeader); err != nil {
		return errors.Wrap(err, "unable to write UsbIpMessageHeader")
	}
	if err := usbdevice.WriteBE(*conn, replyBody); err != nil {
		return errors.Wrap(err, "unable to write UsbIpReturnSubmitBody")
	}
	if header.Direction == dirIn {
		if err := usbdevice.Write(*conn, transferBuffer); err != nil {
			return errors.Wrap(err, "unable to write transferBuffer")
		}
	}
	return nil
}

func handleCommandUnlink(conn *net.Conn, header messageHeader) error {
	if _, err := usbdevice.ReadBE[commandUnlinkBody](*conn); err != nil {
		return errors.Wrap(err, "unable to read UsbIpCommandUnlinkBody")
	}
	replyHeader := messageHeader{
		Command:        commandRetUnlink,
		SequenceNumber: header.SequenceNumber,
		DeviceID:       header.DeviceID,
		Direction:      dirOut,
		Endpoint:       header.Endpoint,
	}
	replyBody := returnUnlinkBody{
		Status:  -int32(unix.ECONNRESET),
		Padding: [24]byte{},
	}
	if err := usbdevice.WriteBE(*conn, replyHeader); err != nil {
		return errors.Wrap(err, "unable to write UsbIpMessageHeader")
	}
	if err := usbdevice.WriteBE(*conn, replyBody); err != nil {
		return errors.Wrap(err, "unable to write UsbIpReturnUnlinkBody")
	}
	return nil
}

func usbipSummaryHeader(d usbdevice.Device) DeviceSummaryHeader {
	result := DeviceSummaryHeader{
		Speed:               usbdevice.SpeedFull,
		IDVendor:            d.DeviceDescriptor().IDVendor,
		IDProduct:           d.DeviceDescriptor().IDProduct,
		BcdDevice:           d.DeviceDescriptor().BcdDevice,
		BDeviceClass:        d.DeviceDescriptor().BDeviceClass,
		BDeviceSubclass:     d.DeviceDescriptor().BDeviceSubclass,
		BDeviceProtocol:     d.DeviceDescriptor().BDeviceProtocol,
		BConfigurationValue: d.ConfigurationDescriptor().BConfigurationValue,
		BNumConfigurations:  d.DeviceDescriptor().BNumConfigurations,
		BNumInterfaces:      d.ConfigurationDescriptor().BNumInterfaces,
	}

	return result
}

func usbipSummary(d usbdevice.Device) DeviceSummary {
	return DeviceSummary{
		Header: usbipSummaryHeader(d),
		DeviceInterface: DeviceInterface{
			BInterfaceClass:    d.InterfaceDescriptor().BInterfaceClass,
			BInterfaceSubclass: d.InterfaceDescriptor().BInterfaceSubclass,
			BInterfaceProtocol: d.InterfaceDescriptor().BInterfaceProtocol,
			Padding:            0,
		},
	}
}
