// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/vm/dlc"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const runVhostUserNetTest string = "run-vhost-user-net-test.sh"

func init() {
	testing.AddTest(&testing.Test{
		Func:         VhostUserNet,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests crosvm's vhost-user net device",
		Contacts:     []string{"cros-virt-devices-guests@google.com", "keiichiw@google.com"},
		BugComponent: "b:1248538",
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{runVhostUserNetTest},
		SoftwareDeps: []string{"vm_host", "chrome", "dlc"},
		Fixture:      "vmDLC",
	})
}

type ifreq struct {
	name  [unix.IFNAMSIZ]byte
	flags int16
}

func openTapDevice(ifname string) (int, error) {
	const path = "/dev/net/tun"

	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to open Tap device: %v", ifname)
	}

	if len(ifname) > unix.IFNAMSIZ-1 {
		unix.Close(fd)
		return 0, errors.Wrapf(err, "too long Ifname: %s", ifname)
	}

	ifr := ifreq{}
	copy(ifr.name[:], ifname)
	ifr.flags = unix.IFF_TAP | unix.IFF_NO_PI | unix.IFF_VNET_HDR
	if _, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		unix.TUNSETIFF,
		uintptr(unsafe.Pointer(&ifr)),
	); errno != 0 {
		unix.Close(fd)
		return 0, errors.Errorf("failed to set network interface: %s", errno.Error())
	}

	return fd, nil
}

func getCrosvmCmd(ctx context.Context, kernel, serialLog, sock, script string, scriptArgs []string) *testexec.Cmd {
	kernParams := []string{
		"root=/dev/root",
		"rootfstype=virtiofs",
		"rw",
		fmt.Sprintf("init=%s", script),
		"--",
	}
	kernParams = append(kernParams, scriptArgs...)

	ps := vm.NewCrosvmParams(
		kernel,
		vm.SharedDir(
			vm.SharedDirParam{
				Src:       "/",
				Tag:       "/dev/root",
				FsType:    "fs",
				Cache:     "always",
				Timeout:   5,
				Writeback: false,
				DAX:       false,
			}),
		vm.VhostUserNet(sock),
		vm.KernelArgs(kernParams...),
		vm.SerialOutput(serialLog),
	)
	args := ps.ToArgs()
	return testexec.CommandContext(ctx, "crosvm", args...)
}

type tapDevice struct {
	fd      int
	addr    net.IP
	gateway net.IP
}

func getTap(ctx context.Context, pc *patchpanel.Client, cid uint32) (device tapDevice, cleanup func(), err error) {
	resp, err := pc.NotifyTerminaVMStartup(ctx, cid)
	if err != nil {
		err = errors.Wrap(err, "failed to send NotifyTerminaVMStartup request to patchpanel")
		return
	}
	shutdown := func() {
		if err := pc.NotifyTerminaVMShutdown(ctx, cid); err != nil {
			testing.ContextLog(ctx, "Failed to notify termina shutdown: ", err)
		}
	}

	fd, err := openTapDevice(resp.TapDeviceIfname)
	if err != nil {
		shutdown()
		err = errors.Wrap(err, "failed to open Tap device")
		return
	}

	device = tapDevice{
		fd:      fd,
		addr:    resp.Ipv4Address,
		gateway: resp.GatewayIpv4Address,
	}

	cleanup = func() {
		shutdown()
		unix.Close(fd)
	}

	return
}

func waitUntilFileAvailable(ctx context.Context, path string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(path); err != nil {
			return errors.Wrapf(err, "%s is unavailable", path)
		}
		return nil
	}, nil)
}

func VhostUserNet(ctx context.Context, s *testing.State) {
	td, err := ioutil.TempDir("", "tast.vm.VhostUserNet.")
	if err != nil {
		s.Fatal("Failed to create temporary directory: ", err)
	}
	defer os.RemoveAll(td)

	// Get Tap Fds
	pc, err := patchpanel.New(ctx)
	if err != nil {
		s.Fatal("Failed to create patchpanel client: ", err)
	}

	// cid will be used by patchpanel to identify VMs.
	// Use values larger than 8192 to guarantee no overlap with crostini/arcvm.
	serverCid := uint32(8193)
	serverTap, serverCleanup, err := getTap(ctx, pc, serverCid)
	if err != nil {
		s.Fatal("Failed to get Tap FD for server: ", err)
	}
	defer serverCleanup()

	clientCid := uint32(8194)
	clientTap, clientCleanup, err := getTap(ctx, pc, clientCid)
	if err != nil {
		s.Fatal("Failed to get Tap FD for client: ", err)
	}
	defer clientCleanup()

	// Start vhost-user-net-device.
	devlog, err := os.Create(filepath.Join(s.OutDir(), "device.log"))
	if err != nil {
		s.Fatal("Failed to create device log file: ", err)
	}
	defer devlog.Close()

	serverSock := filepath.Join(td, "vhost-user-net-server.sock")
	clientSock := filepath.Join(td, "vhost-user-net-client.sock")

	cmdArgs := []string{
		"device",
		"net",
		"--tap-fd", fmt.Sprintf("%s,%d", serverSock, serverTap.fd),
		"--tap-fd", fmt.Sprintf("%s,%d", clientSock, clientTap.fd),
	}
	devCmd := testexec.CommandContext(ctx, "crosvm", cmdArgs...)
	devCmd.Stdout = devlog
	devCmd.Stderr = devlog
	if err := devCmd.Start(); err != nil {
		s.Fatal("Failed to start vhost-user net device: ", err)
	}

	data := s.FixtValue().(dlc.FixtData)
	script := s.DataPath(runVhostUserNetTest)

	// Wait until the device starts and sockets are created
	if err := waitUntilFileAvailable(ctx, serverSock); err != nil {
		s.Fatalf("%s didn't become available: %v", serverSock, err)
	}
	if err := waitUntilFileAvailable(ctx, clientSock); err != nil {
		s.Fatalf("%s didn't become available: %v", clientSock, err)
	}

	// Start server VM
	serverLog := filepath.Join(s.OutDir(), "serial-server.log")
	serverArgs := []string{
		"server",
		serverTap.addr.String(),
		serverTap.gateway.String(),
	}
	serverCmd := getCrosvmCmd(ctx, data.Kernel, serverLog, serverSock, script, serverArgs)
	serverOut, err := os.Create(filepath.Join(s.OutDir(), "crosvm-server.log"))
	if err != nil {
		s.Fatal("Failed to create crosvm server log file: ", err)
	}
	defer serverOut.Close()
	serverCmd.Stdout = serverOut
	serverCmd.Stderr = serverOut

	if err := serverCmd.Start(); err != nil {
		s.Fatal("Failed to run server crosvm: ", err)
	}

	// Start client VM
	clientLog := filepath.Join(s.OutDir(), "serial-client.log")
	clientArgs := []string{
		"client",
		clientTap.addr.String(),
		clientTap.gateway.String(),
		serverTap.addr.String(), // destination address
	}
	clientCmd := getCrosvmCmd(ctx, data.Kernel, clientLog, clientSock, script, clientArgs)
	clientOut, err := os.Create(filepath.Join(s.OutDir(), "crosvm-client.log"))
	if err != nil {
		s.Fatal("Failed to create crosvm client log file: ", err)
	}
	defer clientOut.Close()
	clientCmd.Stdout = clientOut
	clientCmd.Stderr = clientOut

	if err := clientCmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to run client crosvm: ", err)
	}

	// Wait for server VM being completed.
	if err := serverCmd.Wait(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to complete server VM: ", err)
	}

	// vhost-user-net device must stop right after all of VMs stopped.
	if err := devCmd.Wait(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to complete vhost-user-net-device: ", err)
	}

	// Check client log
	log, err := ioutil.ReadFile(clientLog)
	if err != nil {
		s.Fatalf("Failed to read %s: %v", clientLog, err)
	}

	if !bytes.Contains(log, []byte("iperf Done.")) {
		s.Fatal("iperf3 didn't run successfully")
	}
}
