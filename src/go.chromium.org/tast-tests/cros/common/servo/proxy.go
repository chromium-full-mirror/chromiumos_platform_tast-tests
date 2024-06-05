// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const proxyTimeout = 10 * time.Second // max time for establishing SSH connection

// Proxy wraps a Servo object and forwards connections to the servod instance
// over SSH if needed.
type Proxy struct {
	svo             *Servo
	hst             *ssh.Conn      // Initialized lazily.
	fwd             *ssh.Forwarder // nil if servod is running locally or inside a docker container
	servoHostname   string
	port            int
	sshPort         int
	keyFile, keyDir string
	dcl             *client.Client // nil if servod is not running inside a docker container
	sdc             string         // empty if servod is not running inside a docker container
}

func createDockerClient(ctx context.Context, dockerHost string) (*client.Client, error) {
	// Create Docker Client.
	// If no user-provided docker host exists but a unix domain socket does, then use that.
	if dockerHost == "" {
		_, err := os.Stat("/var/run/docker.sock")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			testing.ContextLog(ctx, "Docker client connecting over docker.sock")
			return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		}
		// Default to satlab docker settings.
		dockerHost = "tcp://192.168.231.1:2375"
	}
	// Otherwise connect over TCP
	testing.ContextLogf(ctx, "Docker client connecting over TCP to %q", dockerHost)

	// b/207133139, default HTTPClient inside the Docker Client object fails to
	// connects to docker daemon. Create the transport with DialContext and use
	// this while initializing new docker client object.
	timeout := time.Duration(1 * time.Second)
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: timeout,
		}).DialContext,
	}
	c := http.Client{Transport: transport}

	return client.NewClientWithOpts(client.WithHost(dockerHost), client.WithHTTPClient(&c), client.WithAPIVersionNegotiation())
}

type connectInfo struct {
	Hostname        string
	ServoPort       int
	ServoSSHPort    int
	DockerContainer string
	DockerHost      string
}

func splitHostPort(servoHostPort string) (*connectInfo, error) {
	var result connectInfo
	result.Hostname = "localhost"
	result.ServoPort = 9999
	result.ServoSSHPort = 22

	hostport := servoHostPort

	dockerParts := strings.SplitN(hostport, ":docker:", 2)
	if len(dockerParts) > 1 {
		hostport = dockerParts[0]
		dockerArgs := strings.SplitN(dockerParts[1], ":", 2)
		if len(dockerArgs) == 0 {
			return nil, errors.New("servo arg must be of the form hostname:9999:docker:container_name or hostname:9999:docker:container_name:tcp://docker_host:2375")
		}
		result.DockerContainer = dockerArgs[0]
		if len(dockerArgs) > 1 {
			result.DockerHost = dockerArgs[1]
		}
		result.ServoSSHPort = 0
	}

	if strings.HasSuffix(hostport, ":nossh") {
		result.ServoSSHPort = 0
		hostport = strings.TrimSuffix(hostport, ":nossh")
	}
	sshParts := strings.SplitN(hostport, ":ssh:", 2)
	if len(sshParts) > 0 {
		hostport = sshParts[0]
	}

	// The port starts after the last colon.
	i := strings.LastIndexByte(hostport, ':')
	if i >= 0 {
		if hostport[0] == '[' {
			// Expect the first ']' just before the last ':'.
			end := strings.IndexByte(hostport, ']')
			if end < 0 {
				return nil, errors.New("missing ']' in address")
			}
			switch end + 1 {
			case len(hostport): // No port
				if hostport[1:end] != "" {
					result.Hostname = hostport[1:end]
				}
				i = -1
			case i: // ] before :
				if hostport[1:end] != "" {
					result.Hostname = hostport[1:end]
				}
			default:
				return nil, errors.New("servo arg must be of the form hostname:9999 or hostname:9999:ssh:22 or [::1]:9999")
			}
		} else {
			if hostport[:i] != "" {
				result.Hostname = hostport[:i]
			}
			if strings.IndexByte(result.Hostname, ':') >= 0 {
				return nil, errors.New("unexpected colon in hostname")
			}
		}
		if i >= 0 {
			var err error
			if result.ServoPort, err = strconv.Atoi(hostport[i+1:]); err != nil {
				return nil, errors.Wrap(err, "parsing servo port")
			}
			if result.ServoPort <= 0 {
				return nil, errors.New("invalid servo port")
			}
		}
	} else if hostport != "" {
		result.Hostname = hostport
	}
	// If hostname looks like docker, then it is.
	if strings.HasSuffix(result.Hostname, "docker_servod") && result.DockerContainer == "" {
		result.DockerContainer = result.Hostname
		result.ServoSSHPort = 0
		result.Hostname = ""
		return &result, nil
	}
	// If localhost, default to no ssh.
	if result.Hostname == "localhost" || result.Hostname == "127.0.0.1" || result.Hostname == "::1" {
		result.ServoSSHPort = 0
	}
	if len(sshParts) > 1 {
		var err error
		if result.ServoSSHPort, err = strconv.Atoi(sshParts[1]); err != nil {
			return nil, errors.Wrap(err, "parsing servo host ssh port")
		}
		if result.ServoSSHPort <= 0 {
			return nil, errors.New("invalid servo host ssh port")
		}
	}
	return &result, nil
}

// NewProxy returns a Proxy object for communicating with the servod instance at spec,
// which can be blank (defaults to localhost:9999) or a hostname (defaults to hostname:9999:ssh:22)
// or a host:port (ssh port defaults to 22) or to fully qualify everything host:port:ssh:sshport.
//
// Use hostname:9999:nossh to prevent the use of ssh at all. You probably don't ever want to use this.
//
// You can also use IPv4 addresses as the hostnames, or IPv6 addresses in square brackets [::1].
//
// If you are using ssh port forwarding, please note that the host and ssh port will be evaluated locally,
// but the servo port should be the real servo port on the servo host.
// So if you used the ssh command `ssh -L 2223:localhost:22 -L 2222:${DUT_HOSTNAME?}:22 root@${SERVO_HOSTNAME?}`
// then you would start tast with `tast run --var=servo=localhost:${SERVO_PORT?}:ssh:2223 localhost:2222 firmware.Config*`
//
// If the instance is not running on the local system, an SSH connection will be opened
// to the host running servod and servod connections will be forwarded through it.
// keyFile and keyDir are used for establishing the SSH connection and should
// typically come from dut.DUT's KeyFile and KeyDir methods.
//
// If the servod is running in a docker container, the serverHostPort expected to be in form "${CONTAINER_NAME}:9999:docker:".
// The port of the servod host is defaulted to 9999, user only needs to provide the container name.
// CONTAINER_NAME must end with docker_servod.
func NewProxy(ctx context.Context, servoHostPort, keyFile, keyDir string) (newProxy *Proxy, retErr error) {
	var pxy Proxy
	defer func() {
		if retErr != nil {
			pxy.Close(ctx)
		}
	}()

	connectInfo, err := splitHostPort(servoHostPort)
	if err != nil {
		return nil, err
	}
	pxy.port = connectInfo.ServoPort
	pxy.servoHostname = connectInfo.Hostname
	pxy.sshPort = connectInfo.ServoSSHPort
	pxy.keyFile = keyFile
	pxy.keyDir = keyDir

	if err := pxy.connectSSH(ctx); err != nil {
		return nil, err
	}
	if pxy.hst != nil {
		return &pxy, nil
	}
	if connectInfo.DockerContainer != "" {
		pxy.dcl, err = createDockerClient(ctx, connectInfo.DockerHost)
		if err != nil {
			return nil, err
		}
		// b/227630721: For servod containers, create the XLMRpc connection using IP address instead of hostname.
		if pxy.servoHostname == "" {
			ip, err := getServodContainerIP(ctx, pxy.dcl, connectInfo.DockerContainer)
			if err != nil {
				return nil, err
			}
			pxy.servoHostname = ip
		}
		pxy.sdc = connectInfo.DockerContainer
	}

	testing.ContextLogf(ctx, "Connecting to servod directly at %s:%d", pxy.servoHostname, pxy.port)
	pxy.svo, err = New(ctx, pxy.servoHostname, pxy.port)
	if err != nil {
		return nil, err
	}
	return &pxy, nil
}

// getServodContainerIP returns the IP address of the given docker container name if it found,
// otherwise it results a empty string and error.
func getServodContainerIP(ctx context.Context, dcl *client.Client, name string) (ipaddr string, err error) {
	// Check if docker client object is created, otherwise return an error.
	if dcl == nil {
		return "", errors.New("Docker client object is empty")
	}
	// Create filter to get container with given hostname and in running state.
	f := filters.NewArgs()
	f.Add("name", name)
	f.Add("status", "running")
	// Get the list of containers based on the filter above.
	containers, err := dcl.ContainerList(ctx, types.ContainerListOptions{Filters: f})
	if err != nil {
		testing.ContextLog(ctx, "Error occurred while getting the docker containers list")
		return "", err
	}
	// Return error if the container is not found or is not in running state.
	if len(containers) != 1 {
		return "", errors.Errorf("%d number of container(s) with name %s found", len(containers), name)
	}
	// Get the Docker network set to the container, this is set in the drone env variables.
	// If not found then fall back to default network name.
	cnet := os.Getenv("DOCKER_DEFAULT_NETWORK")
	if cnet == "" {
		cnet = "default_satlab"
	}
	if containers[0].NetworkSettings != nil {
		satNet := containers[0].NetworkSettings.Networks[cnet]
		if satNet != nil {
			return satNet.IPAddress, nil
		}
		return "", errors.Errorf("could not find the %q network for the container %q. Found networks: [%v]", cnet, name, containers[0].NetworkSettings.Networks)
	}
	return "", errors.Errorf("could not find IP address for the container %q", name)
}

func (p *Proxy) connectSSH(ctx context.Context) (retErr error) {
	// If the servod instance isn't running locally, assume that we need to connect to it via SSH.
	if p.sshPort <= 0 || p.hst != nil {
		return nil
	}
	// First, create an SSH connection to the remote system running servod.
	sopt := ssh.Options{
		KeyFile:        p.keyFile,
		KeyDir:         p.keyDir,
		ConnectTimeout: proxyTimeout,
		WarnFunc:       func(msg string) { testing.ContextLog(ctx, msg) },
		Hostname:       net.JoinHostPort(p.servoHostname, fmt.Sprint(p.sshPort)),
		User:           "root",
	}
	testing.ContextLogf(ctx, "Opening Servo SSH connection to %s", sopt.Hostname)
	hst, err := ssh.New(ctx, &sopt)
	if err != nil {
		logServoStatus(ctx, hst, p.sshPort)
		return err
	}
	defer func() {
		if retErr != nil {
			hst.Close(ctx)
		}
	}()

	testing.ContextLog(ctx, "Creating forwarded connection to port ", p.port)
	p.fwd, err = hst.NewForwarder("localhost:0", fmt.Sprintf("localhost:%d", p.port),
		func(err error) { testing.ContextLog(ctx, "Got servo forwarding error: ", err) })
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			p.fwd.Close()
			p.fwd = nil
		}
	}()
	var portstr, host string
	var port int
	if host, portstr, err = net.SplitHostPort(p.fwd.ListenAddr().String()); err != nil {
		return err
	}
	if port, err = strconv.Atoi(portstr); err != nil {
		return errors.Wrap(err, "parsing forwarded servo port")
	}

	if p.svo == nil {
		testing.ContextLogf(ctx, "Connecting to servod via ssh at %s:%d", host, port)
		p.svo, err = New(ctx, host, port)
		if err != nil {
			return err
		}
	} else {
		testing.ContextLogf(ctx, "Reconnecting to servod via ssh at %s:%d", host, port)
		err = p.svo.reconnect(ctx, host, port)
		if err != nil {
			return err
		}
	}

	p.hst = hst
	return nil
}

// logServoStatus logs the current servo status from the servo host.
func logServoStatus(ctx context.Context, hst *ssh.Conn, port int) {
	// Check if servod is running of the servo host.
	out, err := hst.CommandContext(ctx, "servodtool", "instance", "show", "-p", fmt.Sprint(port)).CombinedOutput()
	if err != nil {
		testing.ContextLogf(ctx, "Servod process is not initialized on the servo-host: %v: %v", err, string(out))
		return
	}
	testing.ContextLogf(ctx, "Servod instance is running on port %v of the servo host", port)
	// Check if servod is busy.
	if out, err = hst.CommandContext(ctx, "dut-control", "-p", fmt.Sprint(port), "serialname").CombinedOutput(); err != nil {
		testing.ContextLogf(ctx, "The servod is not responsive or busy: %v: %v", err, string(out))
		return
	}
	testing.ContextLog(ctx, "Servod is responsive on the host and can provide information about serialname: ", string(out))
}

// Close closes the proxy's SSH connection if present.
func (p *Proxy) Close(ctx context.Context) {
	testing.ContextLog(ctx, "Closing Servo Proxy")
	if p.svo != nil {
		p.svo.Close(ctx)
		p.svo = nil
	}
	if p.fwd != nil {
		p.fwd.Close()
		p.fwd = nil
	}
	if p.hst != nil {
		p.hst.Close(ctx)
		p.hst = nil
	}
	if p.dcl != nil {
		p.dcl.Close()
		p.dcl = nil
	}
}

// Reconnect closes the ssh connection, and reconnects.
func (p *Proxy) Reconnect(ctx context.Context) error {
	testing.ContextLog(ctx, "Closing Servo SSH connection")
	if p.fwd != nil {
		p.fwd.Close()
		p.fwd = nil
	}
	if p.hst != nil {
		p.hst.Close(ctx)
		p.hst = nil
	}
	return p.connectSSH(ctx)
}

func (p *Proxy) isLocal() bool {
	return p.sshPort <= 0 || p.isDockerized()
}

func (p *Proxy) isDockerized() bool {
	return p.sdc != ""
}

// Servo returns the proxy's encapsulated Servo object.
func (p *Proxy) Servo() *Servo { return p.svo }

func (p *Proxy) runCommandImpl(ctx context.Context, dumpLogOnError, asRoot bool, name string, args ...string) error {
	var execOpts []testexec.RunOption
	if dumpLogOnError {
		execOpts = append(execOpts, testexec.DumpLogOnError)
	}
	if p.isLocal() {
		if p.isDockerized() {
			_, _, err := p.dockerExec(ctx, nil, name, args...)
			return err
		}
		if asRoot {
			sudoargs := append([]string{name}, args...)
			testing.ContextLog(ctx, "Running sudo ", sudoargs)
			return testexec.CommandContext(ctx, "sudo", sudoargs...).Run(execOpts...)
		}
		return testexec.CommandContext(ctx, name, args...).Run(execOpts...)
	}
	if err := p.connectSSH(ctx); err != nil {
		return err
	}
	return p.hst.CommandContext(ctx, name, args...).Run(execOpts...)
}

// RunCommand execs a command on the servo host, optionally as root.
func (p *Proxy) RunCommand(ctx context.Context, asRoot bool, name string, args ...string) error {
	return p.runCommandImpl(ctx /*dumpLogOnError=*/, true, asRoot, name, args...)
}

// RunCommandQuiet execs a command on the servo host, optionally as root, does not log output.
func (p *Proxy) RunCommandQuiet(ctx context.Context, asRoot bool, name string, args ...string) error {
	return p.runCommandImpl(ctx /*dumpLogOnError=*/, false, asRoot, name, args...)
}

// OutputCommand execs a command as the root user and returns stdout.
func (p *Proxy) OutputCommand(ctx context.Context, asRoot bool, name string, args ...string) ([]byte, error) {
	if p.isLocal() {
		if p.isDockerized() {
			out, _, err := p.dockerExec(ctx, nil, name, args...)
			return out, err
		}
		if asRoot {
			sudoargs := append([]string{name}, args...)
			testing.ContextLog(ctx, "Running sudo ", sudoargs)
			return testexec.CommandContext(ctx, "sudo", sudoargs...).Output(testexec.DumpLogOnError)
		}
		return testexec.CommandContext(ctx, name, args...).Output(testexec.DumpLogOnError)
	}
	if err := p.connectSSH(ctx); err != nil {
		return nil, err
	}
	return p.hst.CommandContext(ctx, name, args...).Output(ssh.DumpLogOnError)
}

// SeparatedOutputCommand execs a command as the root user,
// and returns stdout, stderr and err.
func (p *Proxy) SeparatedOutputCommand(ctx context.Context, asRoot bool, name string, args ...string) ([]byte, []byte, error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	if p.isLocal() {
		if p.isDockerized() {
			return p.dockerExec(ctx, nil, name, args...)
		}
		cmd := testexec.CommandContext(ctx, name, args...)
		if asRoot {
			sudoargs := append([]string{name}, args...)
			testing.ContextLog(ctx, "Running sudo ", sudoargs)
			cmd = testexec.CommandContext(ctx, "sudo", sudoargs...)
		}
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		err := cmd.Run()
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
	}
	if err := p.connectSSH(ctx); err != nil {
		return nil, nil, err
	}
	cmd := p.hst.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

// InputCommand execs a command and redirects stdin.
func (p *Proxy) InputCommand(ctx context.Context, asRoot bool, stdin io.Reader, name string, args ...string) error {
	if p.isLocal() {
		if p.isDockerized() {
			stdout, stderr, err := p.dockerExec(ctx, stdin, name, args...)
			testing.ContextLog(ctx, "Stdout: ", string(stdout))
			testing.ContextLog(ctx, "Stderr: ", string(stderr))
			return err
		}
		if asRoot {
			sudoargs := append([]string{name}, args...)
			testing.ContextLog(ctx, "Running sudo ", sudoargs)
			cmd := testexec.CommandContext(ctx, "sudo", sudoargs...)
			cmd.Stdin = stdin
			return cmd.Run(testexec.DumpLogOnError)
		}
		cmd := testexec.CommandContext(ctx, name, args...)
		cmd.Stdin = stdin
		return cmd.Run(testexec.DumpLogOnError)
	}
	if err := p.connectSSH(ctx); err != nil {
		return err
	}
	cmd := p.hst.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	return cmd.Run(ssh.DumpLogOnError)
}

// GetFile copies a servo host file to a local file.
func (p *Proxy) GetFile(ctx context.Context, asRoot bool, remoteFile, localFile string) error {
	if p.isLocal() {
		if p.isDockerized() {
			outFile, err := os.OpenFile(localFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
			if err != nil {
				return errors.Wrap(err, "could not create local file")
			}
			r, _, err := p.dcl.CopyFromContainer(ctx, p.sdc, remoteFile)
			if err != nil {
				return errors.Wrap(err, "could not copy remote file")
			}
			tr := tar.NewReader(r)
			if _, err := tr.Next(); err != nil {
				return errors.Wrap(err, "could not read remote tar file")
			}
			_, err = io.Copy(outFile, tr)
			if err != nil {
				return errors.Wrap(err, "could not write to local file")
			}
			return outFile.Close()
		}
		if asRoot {
			// This is effectively copying the file from root to the user running the test.
			cmd := testexec.CommandContext(ctx, "sudo", "cat", remoteFile)
			outFile, err := os.OpenFile(localFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
			if err != nil {
				return errors.Wrap(err, "could not create local file")
			}
			cmd.Stdout = outFile
			if err := cmd.Run(testexec.DumpLogOnError); err != nil {
				outFile.Close()
				return err
			}
			return outFile.Close()
		}
		return testexec.CommandContext(ctx, "cp", remoteFile, localFile).Run(testexec.DumpLogOnError)
	}
	if err := p.connectSSH(ctx); err != nil {
		return err
	}
	return linuxssh.GetFile(ctx, p.hst, remoteFile, localFile, linuxssh.DereferenceSymlinks)
}

// PutFiles copies a local file to a servo host file.
func (p *Proxy) PutFiles(ctx context.Context, asRoot bool, fileMap map[string]string) error {
	if p.isLocal() {
		for l, r := range fileMap {
			if p.isDockerized() {
				f, err := os.ReadFile(l)
				if err != nil {
					return errors.Wrap(err, "could not read local file")
				}
				hdr := &tar.Header{
					Name: filepath.Base(r),
					Size: int64(len(f)),
				}
				var buf bytes.Buffer
				tw := tar.NewWriter(&buf)
				if err := tw.WriteHeader(hdr); err != nil {
					return errors.Wrap(err, "failed to write tar header")
				}
				if _, err := tw.Write(f); err != nil {
					return errors.Wrap(err, "failed to write tar body")
				}
				if err := tw.Close(); err != nil {
					return errors.Wrap(err, "failed to close tar")
				}
				return p.dcl.CopyToContainer(ctx, p.sdc, filepath.Dir(r), &buf, types.CopyToContainerOptions{AllowOverwriteDirWithFile: true})
			}
			if asRoot {
				testing.ContextLogf(ctx, "Running sudo cp %s %s", l, r)
				if err := testexec.CommandContext(ctx, "sudo", "cp", l, r).Run(testexec.DumpLogOnError); err != nil {
					return err
				}
			} else {
				if err := testexec.CommandContext(ctx, "cp", l, r).Run(testexec.DumpLogOnError); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := p.connectSSH(ctx); err != nil {
		return err
	}
	_, err := linuxssh.PutFiles(ctx, p.hst, fileMap, linuxssh.DereferenceSymlinks)
	return err
}

// GetPort returns the port where servod is running on the server.
func (p *Proxy) GetPort() int {
	if p.isDockerized() {
		return 9999
	}
	return p.port
}

// dockerExec execs a command with Docker SDK.
func (p *Proxy) dockerExec(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, []byte, error) {
	// prepare exec
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Privileged:   true,
	}
	if stdin != nil {
		execConfig.AttachStdin = true
	}
	// The only user within servod container is root, no sudo needed.
	execConfig.Cmd = append([]string{name}, args...)
	testing.ContextLog(ctx, "Running docker command ", execConfig.Cmd)
	createResp, err := p.dcl.ContainerExecCreate(ctx, p.sdc, execConfig)
	if err != nil {
		return nil, nil, err
	}
	execID := createResp.ID

	// run it, with stdout/stderr attached
	attachResp, err := p.dcl.ContainerExecAttach(ctx, execID, types.ExecStartCheck{})
	if err != nil {
		return nil, nil, err
	}
	if stdin != nil {
		go func() {
			// Note there is no synchronization between this and the reading from attachResp
			// this assumes the program will not exit until stdin sends EOF, if the program
			// exits before stdin is finished processing it the `defer attachResp.Close()`
			// will cause io.Copy in this goroutine to error and this goroutine will exit
			n, err := io.Copy(attachResp.Conn, stdin)
			if err != nil {
				testing.ContextLogf(ctx, "Error piping stdin to %q process: %v", name, err)
			} else {
				testing.ContextLogf(ctx, "Sent %d bytes from stdin to %q process", n, name)
			}
			attachResp.CloseWrite()
		}()
	}
	defer attachResp.Close()

	// read the output
	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error, 1)

	go func() {
		// StdCopy demultiplexes the stream into two buffers
		_, err = stdcopy.StdCopy(&outBuf, &errBuf, attachResp.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return nil, nil, err
		}
		break

	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	// get the exit code
	inspectResp, err := p.dcl.ContainerExecInspect(ctx, execID)
	if err != nil {
		return nil, nil, err
	}

	stdout := toValidUTF8(outBuf.Bytes())
	stderr := toValidUTF8(errBuf.Bytes())
	if inspectResp.ExitCode != 0 {
		exitCodeErr := errors.Errorf(
			"docker exec command exited with non-zero exit code: %v", inspectResp.ExitCode,
		)
		return stdout, stderr, exitCodeErr
	}
	return stdout, stderr, nil
}

// toValidUTF8 converts replaces invalid utf8 byte segments with empty strings
func toValidUTF8(data []byte) []byte {
	if data == nil {
		return nil
	}
	return []byte(strings.ToValidUTF8(string(data), ""))
}

// Proxied returns true if the servo host is connected via ssh proxy.
func (p *Proxy) Proxied() bool {
	return p.sshPort > 0
}

// NewForwarder forwards a local port to a remote port on the servo host.
func (p *Proxy) NewForwarder(ctx context.Context, hostPort string) (*ssh.Forwarder, error) {
	if !p.Proxied() {
		return nil, errors.New("servo host is not connected via ssh")
	}
	if err := p.connectSSH(ctx); err != nil {
		return nil, err
	}
	fwd, err := p.hst.NewForwarder("localhost:0", hostPort,
		func(err error) { testing.ContextLog(ctx, "Got forwarding error: ", err) })
	if err != nil {
		return nil, errors.Wrap(err, "creating ssh forwarder")
	}
	return fwd, nil
}
