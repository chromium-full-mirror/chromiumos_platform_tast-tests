// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Crosvm holds info about a running instance of the crosvm command.
type Crosvm struct {
	cmd        *testexec.Cmd // crosvm process
	socketPath string        // crosvm control socket
	stdin      io.Writer     // stdin for cmd
	stdout     *os.File      // stdout for cmd; uses os.File to set a read dealine
}

// SharedDirParam holds parameters for a shared directory.
type SharedDirParam struct {
	Src       string
	Tag       string
	FsType    string
	Cache     string
	Timeout   uint
	Writeback bool
	DAX       bool
	CaseFold  bool
}

func (p *SharedDirParam) toArg() string {
	return fmt.Sprintf("%s:%s:type=%s:cache=%s:timeout=%d:writeback=%t:dax=%t:ascii_casefold=%t", p.Src, p.Tag, p.FsType, p.Cache, p.Timeout, p.Writeback, p.DAX, p.CaseFold)
}

// SerialIOPath contains file names used for serial input and output.
type SerialIOPath struct {
	ToGuest string
	ToHost  string
}

// CrosvmParams - Parameters for starting a crosvm instance.
type CrosvmParams struct {
	vmKernel       string           // path to the VM kernel image
	vmBios         string           // path to a BIOS file for the VM
	numCpus        uint             // Number of CPUs to expose to the VM
	memSize        uint             // Amount of guest memory in MiB
	useBiosFile    bool             // Set to true to boot CrosVM with BIOS
	rootfsPath     string           // optional path to the VM rootfs
	diskPaths      []string         // paths that will be mounted read only
	rwDiskPaths    []string         // paths that will be mounted read/write
	socketPath     string           // path to the VM control socket
	kernelArgs     []string         // string arguments to be passed to the VM kernel
	sharedDirs     []SharedDirParam // array of configuration of a directory to be shared with the VM
	serialIO       []SerialIOPath   // paths to files used for serial input and output
	vhostUserNet   []string         // paths to sockets that vhost-user-net devices will use
	disableSandbox bool             // whether or not the sandbox is disabled
	tapFds         []uint           // file descriptors for configured tap device
}

// Option configures a CrosvmParams
type Option func(s *CrosvmParams)

// Rootfs sets a path to the VM rootfs.
func Rootfs(path string) Option {
	return func(p *CrosvmParams) {
		p.rootfsPath = path
	}
}

// Disks adds paths to disks that will be mounted read only.
func Disks(paths ...string) Option {
	return func(p *CrosvmParams) {
		p.diskPaths = append(p.diskPaths, paths...)
	}
}

// RWDisks adds paths to disks that will be mounted read/write.
func RWDisks(paths ...string) Option {
	return func(p *CrosvmParams) {
		p.rwDiskPaths = append(p.rwDiskPaths, paths...)
	}
}

// Socket sets a path to the control socket.
func Socket(path string) Option {
	return func(p *CrosvmParams) {
		p.socketPath = path
	}
}

// KernelArgs sets extra kernel command line arguments.
func KernelArgs(args ...string) Option {
	return func(p *CrosvmParams) {
		p.kernelArgs = append(p.kernelArgs, args...)
	}
}

// SharedDir sets a config for directory to be shared with the VM.
func SharedDir(param SharedDirParam) Option {
	return func(p *CrosvmParams) {
		p.sharedDirs = append(p.sharedDirs, param)
	}
}

// SerialIO sets files used for serial input and output.
func SerialIO(toGuest, toHost string) Option {
	return func(p *CrosvmParams) {
		p.serialIO = append(p.serialIO, SerialIOPath{ToGuest: toGuest, ToHost: toHost})
	}
}

// SerialOutput sets a file that serial log will be written.
func SerialOutput(path string) Option {
	return SerialIO("", path)
}

// VhostUserNet sets a socket to be used by a vhost-user net device.
func VhostUserNet(socket string) Option {
	return func(p *CrosvmParams) {
		p.vhostUserNet = append(p.vhostUserNet, socket)
	}
}

// DisableSandbox disables the sandbox (sandbox is enabled by default without this option)
func DisableSandbox() Option {
	return func(p *CrosvmParams) {
		p.disableSandbox = true
	}
}

// NumCpus allows one to set the number of CPUs to expose to the VM
func NumCpus(numbercpus uint) Option {
	return func(p *CrosvmParams) {
		p.numCpus = numbercpus
	}
}

// MemSize allows one to set the amount of the guest memory in MiB.
func MemSize(memsize uint) Option {
	return func(p *CrosvmParams) {
		p.memSize = memsize
	}
}

// Net sets virtio device by tap device file descriptor
func Net(tapFd uint) Option {
	return func(p *CrosvmParams) {
		p.tapFds = append(p.tapFds, tapFd)
	}
}

// NewCrosvmParams constructs a set of crosvm parameters.
func NewCrosvmParams(kernel string, opts ...Option) *CrosvmParams {
	p := &CrosvmParams{
		vmKernel:    kernel,
		useBiosFile: false,
		numCpus:     1,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// NewCrosvmParamsBIOS constructs a set of crosvm parameters for use with a BIOS.
func NewCrosvmParamsBIOS(bios string, opts ...Option) *CrosvmParams {
	p := &CrosvmParams{
		vmBios:      bios,
		useBiosFile: true,
		numCpus:     1,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// ToArgs converts CrosvmParams to an array of strings that can be used as crosvm's command line flags.
func (p *CrosvmParams) ToArgs() []string {
	args := []string{"run"}

	if p.numCpus > 1 {
		args = append(args, "--cpus", strconv.FormatUint((uint64(p.numCpus)), 10))
	}

	if p.memSize > 0 {
		args = append(args, "--mem", strconv.FormatUint(uint64(p.memSize), 10))
	}

	if p.socketPath != "" {
		args = append(args, "--socket", p.socketPath)
	}

	if p.rootfsPath != "" {
		args = append(args, "--root", p.rootfsPath)
	}

	for _, path := range p.rwDiskPaths {
		args = append(args, "--rwdisk", path)
	}

	for _, path := range p.diskPaths {
		args = append(args, "-d", path)
	}

	for _, param := range p.sharedDirs {
		args = append(args, "--shared-dir", param.toArg())
	}

	for idx, pipes := range p.serialIO {
		input := ""
		if pipes.ToGuest != "" {
			input = fmt.Sprintf(",input=%s", pipes.ToGuest)
		}
		output := ""
		if pipes.ToHost != "" {
			output = fmt.Sprintf(",path=%s", pipes.ToHost)
		}

		args = append(args, "--serial", fmt.Sprintf("type=file,num=%d%s%s", idx+1, input, output))
	}

	if p.disableSandbox {
		args = append(args, "--disable-sandbox")
	}

	for _, sock := range p.vhostUserNet {
		args = append(args, "--vhost-user-net", sock)
	}

	if p.useBiosFile {
		args = append(args, "--bios", p.vmBios)
	} else {
		args = append(args, "-p", strings.Join(p.kernelArgs, " "))
		// The kernel path must come at the end.
		args = append(args, p.vmKernel)
	}

	for _, tapFd := range p.tapFds {
		args = append(args, "--net", fmt.Sprintf("tap-fd=%d", tapFd))
	}

	return args
}

// NewCrosvm starts a crosvm instance with the optional disk path as an additional disk.
func NewCrosvm(ctx context.Context, params *CrosvmParams) (*Crosvm, error) {
	// A client of this must use a kernel or a BIOS
	// Check that the one chosen by the client has a valid path
	if params.useBiosFile {
		if _, err := os.Stat(params.vmBios); err != nil {
			return nil, errors.Wrap(err, "failed to find VM bios")
		}
	} else {
		if _, err := os.Stat(params.vmKernel); err != nil {
			return nil, errors.Wrap(err, "failed to find VM kernel")
		}
	}

	vm := &Crosvm{}
	vm.cmd = testexec.CommandContext(ctx, "crosvm", params.ToArgs()...)
	vm.socketPath = params.socketPath

	var err error

	if vm.stdin, err = vm.cmd.StdinPipe(); err != nil {
		return nil, err
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	vm.cmd.Stdout = pw
	vm.stdout = pr

	defer pw.Close()

	if err = vm.cmd.Start(); err != nil {
		pr.Close()
		return nil, err
	}
	return vm, nil
}

// Close stops the crosvm process (and underlying VM) started by NewCrosvm.
func (vm *Crosvm) Close(ctx context.Context) error {
	defer vm.stdout.Close()
	cmd := testexec.CommandContext(ctx, "crosvm", "stop", vm.socketPath)
	if err := cmd.Run(); err != nil {
		testing.ContextLog(ctx, "Failed to exec stop: ", err)
		cmd.DumpLog(ctx)
		return err
	}
	if err := vm.cmd.Wait(); err != nil {
		testing.ContextLog(ctx, "Failed waiting for crosvm to exit: ", err)
		vm.cmd.DumpLog(ctx)
		return err
	}

	return nil
}

// Stdin is attached to the crosvm process's stdin. It can be used to run commands.
func (vm *Crosvm) Stdin() io.Writer {
	return vm.stdin
}

// Stdout is attached to the crosvm process's stdout. It receives all console output.
func (vm *Crosvm) Stdout() io.Reader {
	return vm.stdout
}

// WaitForCompletion waits for the crosvm process to terminate.
func (vm *Crosvm) WaitForCompletion() error {
	return vm.cmd.Wait()
}

// WaitForOutput waits until a line matched by re has been written to stdout,
// crosvm's stdout is closed, or the deadline is reached. It returns the full
// line that was matched. This function will consume output from stdout until it
// returns.
func (vm *Crosvm) WaitForOutput(ctx context.Context, re *regexp.Regexp) (string, error) {
	// Start a goroutine that reads bytes from crosvm and buffers them in a
	// string builder. We can't do this with lines because then we will miss the
	// initial prompt that comes up that doesn't have a line terminator. If a
	// matching line is found, send it through the channel.
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	// Allow the blocking read call to stop when the deadline has been exceeded.
	// Defer removing the deadline until this function has exited.
	deadline, ok := ctx.Deadline()
	// If no deadline is set, default to no timeout.
	if !ok {
		deadline = time.Time{}
	}
	vm.stdout.SetReadDeadline(deadline)
	defer vm.stdout.SetReadDeadline(time.Time{})
	go func() {
		defer close(ch)
		var line strings.Builder
		var b [1]byte
		for {
			_, err := vm.stdout.Read(b[:])
			if err != nil {
				ch <- result{"", err}
				return
			}
			if b[0] == '\n' {
				line.Reset()
				continue
			}
			line.WriteByte(b[0])
			if re.MatchString(line.String()) {
				ch <- result{line.String(), nil}
				return
			}
		}
	}()

	select {
	case r := <-ch:
		if os.IsTimeout(r.err) {
			// If the read times out, this means the deadline has passed
			select {
			case <-ctx.Done():
				return "", errors.Wrap(ctx.Err(), "timeout out waiting for output")
			}
		}
		return r.line, r.err
	}
}
