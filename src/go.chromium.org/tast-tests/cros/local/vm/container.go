// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	cpb "go.chromium.org/chromiumos/system_api/vm_cicerone_proto" // protobufs for container management

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/caller"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	liveContainerImageServerFormat    = "https://storage.googleapis.com/cros-containers/%d"         // simplestreams image server being served live
	stagingContainerImageServerFormat = "https://storage.googleapis.com/cros-containers-staging/%d" // simplestreams image server for staging
	tarballRootfsPath                 = "/mnt/shared/MyFiles/Downloads/crostini/container_rootfs.tar.xz"
	tarballMetadataPath               = "/mnt/shared/MyFiles/Downloads/crostini/container_metadata.tar.xz"

	ciceroneName      = "org.chromium.VmCicerone"
	ciceronePath      = dbus.ObjectPath("/org/chromium/VmCicerone")
	ciceroneInterface = "org.chromium.VmCicerone"
)

// ContainerImageType represents the mechanism/bucket that we should use to get the container.
type ContainerImageType int

const (
	// LiveImageServer indicates that the current live container image should be downloaded.
	LiveImageServer ContainerImageType = iota
	// StagingImageServer indicates that the current staging container image should be downloaded.
	StagingImageServer
	// Tarball indicates that the container image is available as tarball shared over 9P.
	Tarball
)

// ContainerDebianVersion represents the OS version of the container's image.
type ContainerDebianVersion struct {
	// Codename of the distribution
	Codename string
	// Version number of the distribution
	Version int
}

var (
	// DebianBuster refers to the "buster" distribution of debian (a.k.a. debian 10).
	DebianBuster = ContainerDebianVersion{
		Codename: "buster",
		Version:  10,
	}
	// DebianBullseye refers to the "bullseye" distribution of debian (a.k.a. debian 11).
	DebianBullseye = ContainerDebianVersion{
		Codename: "bullseye",
		Version:  11,
	}
	// DebianBookworm refers to the "bookworm" distribution of debian (a.k.a. debian 12).
	DebianBookworm = ContainerDebianVersion{
		Codename: "bookworm",
		Version:  12,
	}
)

// ContainerArch represents the architecture of the container
type ContainerArch string

const (
	// Amd64 indicates that the container is built for amd64
	Amd64 ContainerArch = "amd64"
	// Arm indicates that the container is build for arm
	Arm ContainerArch = "arm"
)

// ContainerType defines the type of container.
type ContainerType struct {
	// Image is the image source for this container.
	Image ContainerImageType
	// DebianVersion is the version of debian that the image has.
	DebianVersion ContainerDebianVersion
}

// Container encapsulates a container running in a VM.
type Container struct {
	// VM is the VM in which this container is running.
	VM            *VM
	containerName string // name of the container
	username      string // username of the container's primary user
}

// locked is used to prevent creation of a container while the precondition is being used.
var locked = false

// prePackages lists packages containing preconditions that are allowed to call Lock and Unlock.
var prePackages = []string{
	"go.chromium.org/tast-tests/cros/local/crostini",
	"go.chromium.org/tast-tests/cros/local/multivm",
}

// Lock prevents container creation/destruction until Unlock is called.
// It can only be called by preconditions and is idempotent.
func Lock() {
	caller.Check(2, prePackages)
	locked = true
}

// Unlock allows container creation after an earlier call to Lock.
// It can only be called by preconditions and is idempotent.
func Unlock() {
	caller.Check(2, prePackages)
	locked = false
}

// newContainer returns a Container instance.
// You must call Connect() to connect to the VM and Cicerone.
func newContainer(ctx context.Context, containerName, userName string) *Container {
	if locked {
		panic("Do not create a new Container while a container precondition is active")
	}
	return &Container{
		containerName: containerName,
		username:      userName,
	}
}

// DefaultContainer returns a container object with default settings.
func DefaultContainer(ctx context.Context, userEmail string) (*Container, error) {
	username := strings.SplitN(userEmail, "@", 2)[0]
	c := newContainer(ctx, DefaultContainerName, username)
	return c, c.Connect(ctx, userEmail)
}

// Connect connects the container to the running VM and cicerone instances.
func (c *Container) Connect(ctx context.Context, user string) error {
	vm, err := GetRunningVM(ctx, user, Termina)
	if err != nil {
		return err
	}
	c.VM = vm
	return nil
}

// GetRunningContainer returns a Container struct for a currently running container.
// This is useful when the container was started by some other means, eg the installer.
// Will return an error if no container is currently running.
func GetRunningContainer(ctx context.Context, user string) (*Container, error) {
	_, err := GetRunningVM(ctx, user, Termina)
	if err != nil {
		return nil, err
	}
	return DefaultContainer(ctx, user)
}

// ArchitectureAlias returns the alias subpath of the chosen container
// architecture, i.e. part of the path used to compute the container's
// gsutil URL.
func ArchitectureAlias(t ContainerDebianVersion) string {
	return fmt.Sprintf("debian/%s/test", t.Codename)
}

// Create will create a Linux container in an existing VM. It returns without waiting for the creation to complete.
// One must listen on cicerone D-Bus signals to know the creation is done.
func (c *Container) Create(ctx context.Context, t ContainerType) error {
	req := &cpb.CreateLxdContainerRequest{
		VmName:        c.VM.name,
		ContainerName: DefaultContainerName,
		OwnerId:       c.VM.Concierge.ownerID,
	}

	switch t.Image {
	case LiveImageServer:
		req.ImageServer = liveContainerImageServerFormat
		req.ImageAlias = ArchitectureAlias(t.DebianVersion)
	case StagingImageServer:
		req.ImageServer = stagingContainerImageServerFormat
		req.ImageAlias = ArchitectureAlias(t.DebianVersion)
	case Tarball:
		req.RootfsPath = tarballRootfsPath
		req.MetadataPath = tarballMetadataPath
	}

	resp := &cpb.CreateLxdContainerResponse{}
	if err := dbusutil.CallProtoMethod(ctx, c.VM.Concierge.ciceroneObj, ciceroneInterface+".CreateLxdContainer",
		req, resp); err != nil {
		return err
	}

	switch resp.GetStatus() {
	case cpb.CreateLxdContainerResponse_UNKNOWN, cpb.CreateLxdContainerResponse_FAILED:
		return errors.Errorf("failed to create container: %v", resp.GetFailureReason())
	case cpb.CreateLxdContainerResponse_EXISTS:
		return errors.New("container already exists")
	}
	return nil
}

// start launches a Linux container in an existing VM.
func (c *Container) start(ctx context.Context) error {
	starting, err := dbusutil.NewSignalWatcherForSystemBus(ctx, ciceroneDBusMatchSpec("LxdContainerStarting"))
	if err != nil {
		return err
	}
	// Always close the LxdContainerStarting watcher regardless of success.
	defer starting.Close(ctx)

	resp := &cpb.StartLxdContainerResponse{}
	if err := dbusutil.CallProtoMethod(ctx, c.VM.Concierge.ciceroneObj, ciceroneInterface+".StartLxdContainer",
		&cpb.StartLxdContainerRequest{
			VmName:        c.VM.name,
			ContainerName: c.containerName,
			OwnerId:       c.VM.Concierge.ownerID,
			Async:         true,
		}, resp); err != nil {
		return err
	}

	switch resp.GetStatus() {
	case cpb.StartLxdContainerResponse_RUNNING:
		return errors.New("container is already running")
	case cpb.StartLxdContainerResponse_STARTING, cpb.StartLxdContainerResponse_REMAPPING:
	default:
		return errors.Errorf("failed to start container: %v", resp.GetFailureReason())
	}

	sigResult := &cpb.LxdContainerStartingSignal{}
	for sigResult.VmName != c.VM.name ||
		sigResult.ContainerName != c.containerName ||
		sigResult.OwnerId != c.VM.Concierge.ownerID {
		if err := waitForDBusSignal(ctx, starting, nil, sigResult); err != nil {
			return err
		}
	}

	if sigResult.Status != cpb.LxdContainerStartingSignal_STARTED {
		return errors.Errorf("container failed to start: %v", resp.GetFailureReason())
	}

	testing.ContextLogf(ctx, "Started container %q in VM %q", c.containerName, c.VM.name)
	return nil
}

// Stop shuts down a Linux container in an existing VM.
func (c *Container) Stop(ctx context.Context) error {
	// Use a shortened context for stopping.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	stopping, err := dbusutil.NewSignalWatcherForSystemBus(ctx, ciceroneDBusMatchSpec("LxdContainerStopping"))
	if err != nil {
		return err
	}
	// Always close the LxdContainerStopping watcher regardless of success.
	defer stopping.Close(cleanupCtx)

	resp := &cpb.StopLxdContainerResponse{}
	if err := dbusutil.CallProtoMethod(ctx, c.VM.Concierge.ciceroneObj, ciceroneInterface+".StopLxdContainer",
		&cpb.StopLxdContainerRequest{
			VmName:        c.VM.name,
			ContainerName: c.containerName,
			OwnerId:       c.VM.Concierge.ownerID,
		}, resp); err != nil {
		return err
	}

	switch resp.GetStatus() {
	case cpb.StopLxdContainerResponse_STOPPED:
		// The container has already stopped, just return.
		return nil
	case cpb.StopLxdContainerResponse_STOPPING:
		// In this case, the code moves on to the next part - ContainerShutdownSignal.
		// It does not go to default.
	default:
		return errors.Errorf("failed to stop container: %v and failure reason: %v", resp.GetStatus(), resp.GetFailureReason())
	}

	sigResult := &cpb.ContainerShutdownSignal{}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		for sigResult.VmName != c.VM.name ||
			sigResult.ContainerName != c.containerName ||
			sigResult.OwnerId != c.VM.Concierge.ownerID {
			if err := waitForDBusSignal(ctx, stopping, nil, sigResult); err != nil {
				return err
			}
		}

		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		testing.ContextLog(ctx, "Failed to wait for D-Bus ContainerShutdownSignal: ", err)

		// The container didn't stop, try to stop it with more force.
		// This can fail if the container stopped already.
		if _, err := c.VM.LXCCommand(ctx, "stop", c.containerName, "-f"); err != nil {
			testing.ContextLog(ctx, "Failed to stop container with force: ", err)
		}
	}

	testing.ContextLogf(ctx, "Stopped container %q in VM %q", c.containerName, c.VM.name)
	return nil
}

// StartAndWait starts up an already created container and waits for that startup to complete
// before returning.
func (c *Container) StartAndWait(ctx context.Context) error {
	if err := c.SetUpUser(ctx); err != nil {
		return err
	}

	started, err := dbusutil.NewSignalWatcherForSystemBus(ctx, ciceroneDBusMatchSpec("ContainerStarted"))
	if err != nil {
		return err
	}
	// Always close the ContainerStarted watcher regardless of success.
	defer started.Close(ctx)

	if err = c.start(ctx); err != nil {
		return err
	}

	testing.ContextLog(ctx, "Waiting for ContainerStarted D-Bus signal")
	sigResult := &cpb.ContainerStartedSignal{}
	for sigResult.VmName != c.VM.name ||
		sigResult.ContainerName != c.containerName ||
		sigResult.OwnerId != c.VM.Concierge.ownerID {
		if err := waitForDBusSignal(ctx, started, nil, sigResult); err != nil {
			return err
		}
	}
	return nil
}

// GetUsername returns the default user in a container.
func (c *Container) GetUsername(ctx context.Context) (string, error) {
	resp := &cpb.GetLxdContainerUsernameResponse{}
	if err := dbusutil.CallProtoMethod(ctx, c.VM.Concierge.ciceroneObj, ciceroneInterface+".GetLxdContainerUsername",
		&cpb.GetLxdContainerUsernameRequest{
			VmName:        c.VM.name,
			ContainerName: c.containerName,
			OwnerId:       c.VM.Concierge.ownerID,
		}, resp); err != nil {
		return "", err
	}

	if resp.GetStatus() != cpb.GetLxdContainerUsernameResponse_SUCCESS {
		return "", errors.Errorf("failed to get username: %v", resp.GetFailureReason())
	}

	return resp.GetUsername(), nil
}

// GetHomeDir returns the home directory of the default user as an absolute path.
func (c *Container) GetHomeDir(ctx context.Context) (string, error) {
	username, err := c.GetUsername(ctx)
	if err != nil {
		return "", err
	}
	return path.Join("/home", username), nil
}

// SetUpUser sets up the default user in a container.
func (c *Container) SetUpUser(ctx context.Context) error {
	resp := &cpb.SetUpLxdContainerUserResponse{}
	if err := dbusutil.CallProtoMethod(ctx, c.VM.Concierge.ciceroneObj, ciceroneInterface+".SetUpLxdContainerUser",
		&cpb.SetUpLxdContainerUserRequest{
			VmName:            c.VM.name,
			ContainerName:     c.containerName,
			OwnerId:           c.VM.Concierge.ownerID,
			ContainerUsername: c.username,
		}, resp); err != nil {
		return err
	}

	if resp.GetStatus() != cpb.SetUpLxdContainerUserResponse_SUCCESS &&
		resp.GetStatus() != cpb.SetUpLxdContainerUserResponse_EXISTS {
		return errors.Errorf("failed to set up user: %v", resp.GetFailureReason())
	}

	testing.ContextLogf(ctx, "Set up user %q in container %q", c.username, c.containerName)
	return nil
}

// GetIPv4Address returns the IPv4 address of the container.
func (c *Container) GetIPv4Address(ctx context.Context) (ip string, err error) {
	cmd := c.Command(ctx, "hostname", "-I")
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "failed to run %v", strings.Join(cmd.Args, " "))
	}
	return findIPv4(string(out))
}

// PushFile copies a local file to the container's filesystem.
func (c *Container) PushFile(ctx context.Context, localPath, containerPath string) error {
	return pushFile(ctx, c.containerName, c.VM, localPath, containerPath)
}

// GetFile copies a remote file from the container's filesystem.
func (c *Container) GetFile(ctx context.Context, containerPath, localPath string) error {
	return getFile(ctx, c.containerName, c.VM, containerPath, localPath)
}

// CheckFilesExistInDir checks files exist in the given path in container.
// Returns error if any file does not exist or any other error.
func (c *Container) CheckFilesExistInDir(ctx context.Context, path string, files ...string) error {
	// Get file list in the path in container.
	fileList, err := c.GetFileList(ctx, path)
	if err != nil {
		return errors.Wrap(err, "failed to list the content of home directory in container")
	}

	// Create a map.
	set := make(map[string]struct{}, len(fileList))
	for _, s := range fileList {
		set[s] = struct{}{}
	}

	// Check each file exists in fileList.
	for _, file := range files {
		if _, ok := set[file]; !ok {
			return errors.Errorf("failed to find %s in container", file)
		}
	}
	return nil
}

// CheckFileDoesNotExistInDir checks files do not exist in the given path in container.
// Return error if any file exists or any other error.
func (c *Container) CheckFileDoesNotExistInDir(ctx context.Context, path string, files ...string) error {
	// Get file list in the path in container.
	fileList, err := c.GetFileList(ctx, path)
	if err != nil {
		return errors.Wrapf(err, "failed to list the content of %s in container", path)
	}

	// Create a map.
	set := make(map[string]struct{}, len(fileList))
	for _, s := range fileList {
		set[s] = struct{}{}
	}

	// Check each file does not exist in fileList.
	for _, file := range files {
		if _, ok := set[file]; ok {
			return errors.Errorf("unexpectedly found %s in container", file)
		}
	}
	return nil
}

// GetFileList returns a list of the files in the given path in the container.
func (c *Container) GetFileList(ctx context.Context, path string) (fileList []string, err error) {
	return getFileList(ctx, c, path)
}

// CheckFileContent checks that the content of the specified file equals to the given string.
// Returns error if fail to read content or the contest does not equal to the given string.
func (c *Container) CheckFileContent(ctx context.Context, filePath, testString string) error {
	return checkFileContent(ctx, c, filePath, testString)
}

// WriteFile creates a file in the container using echo.
func (c *Container) WriteFile(ctx context.Context, filePath, fileContent string) error {
	return writeFile(ctx, c, filePath, fileContent)
}

// RemoveAll removes a path from the container's file system using 'rm -rf'.
func (c *Container) RemoveAll(ctx context.Context, path string) error {
	return c.Command(ctx, "sudo", "rm", "-rf", path).Run(testexec.DumpLogOnError)
}

// ReadFile reads the content of file using command cat and returns it as a string.
func (c *Container) ReadFile(ctx context.Context, filePath string) (content string, err error) {
	return readFile(ctx, c, filePath)
}

// Cleanup removes all the files under the specific path.
func (c *Container) Cleanup(ctx context.Context, path string) error {
	return cleanup(ctx, c, path)
}

// LinuxPackageInfo queries the container for information about a Linux package
// file. The packageID returned corresponds to the package ID for an installed
// package based on the PackageKit specification which is of the form
// 'package_id;version;arch;repository'.
func (c *Container) LinuxPackageInfo(ctx context.Context, path string) (packageID string, err error) {
	resp := &cpb.LinuxPackageInfoResponse{}
	if err := dbusutil.CallProtoMethod(ctx, c.VM.Concierge.ciceroneObj, ciceroneInterface+".GetLinuxPackageInfo",
		&cpb.LinuxPackageInfoRequest{
			VmName:        c.VM.name,
			ContainerName: c.containerName,
			OwnerId:       c.VM.Concierge.ownerID,
			FilePath:      path,
		}, resp); err != nil {
		return "", err
	}

	if !resp.GetSuccess() {
		return "", errors.Errorf("failed to get Linux package info: %v", resp.GetFailureReason())
	}

	return resp.GetPackageId(), nil
}

// InstallPackage installs a Linux package file into the container.
func (c *Container) InstallPackage(ctx context.Context, path string) error {
	progress, err := dbusutil.NewSignalWatcherForSystemBus(ctx, ciceroneDBusMatchSpec("InstallLinuxPackageProgress"))
	if err != nil {
		return err
	}
	// Always close the InstallLinuxPackageProgress watcher regardless of success.
	defer progress.Close(ctx)

	resp := &cpb.InstallLinuxPackageResponse{}
	if err = dbusutil.CallProtoMethod(ctx, c.VM.Concierge.ciceroneObj, ciceroneInterface+".InstallLinuxPackage",
		&cpb.LinuxPackageInfoRequest{
			VmName:        c.VM.name,
			ContainerName: c.containerName,
			OwnerId:       c.VM.Concierge.ownerID,
			FilePath:      path,
		}, resp); err != nil {
		return err
	}

	if resp.Status != cpb.InstallLinuxPackageResponse_STARTED {
		return errors.Errorf("failed to start Linux package install: %v", resp.FailureReason)
	}

	// Wait for the signal for install completion which will signify success or
	// failure.
	testing.ContextLog(ctx, "Waiting for InstallLinuxPackageProgress D-Bus signal")
	sigResult := &cpb.InstallLinuxPackageProgressSignal{}
	for {
		if err := waitForDBusSignal(ctx, progress, nil, sigResult); err != nil {
			return err
		}
		if sigResult.VmName == c.VM.name &&
			sigResult.ContainerName == c.containerName &&
			sigResult.OwnerId == c.VM.Concierge.ownerID {
			if sigResult.Status == cpb.InstallLinuxPackageProgressSignal_SUCCEEDED {
				return nil
			}
			if sigResult.Status == cpb.InstallLinuxPackageProgressSignal_FAILED {
				return errors.Errorf("failure with Linux package install: %v", sigResult.FailureDetails)
			}
		}
	}
}

// UninstallPackageOwningFile uninstalls the package owning a particular desktop
// file in the container.
func (c *Container) UninstallPackageOwningFile(ctx context.Context, desktopFileID string) error {
	progress, err := dbusutil.NewSignalWatcherForSystemBus(ctx, ciceroneDBusMatchSpec("UninstallPackageProgress"))
	if err != nil {
		return err
	}
	// Always close the UninstallPackageProgress watcher regardless of success.
	defer progress.Close(ctx)

	resp := &cpb.UninstallPackageOwningFileResponse{}
	if err = dbusutil.CallProtoMethod(ctx, c.VM.Concierge.ciceroneObj, ciceroneInterface+".UninstallPackageOwningFile",
		&cpb.UninstallPackageOwningFileRequest{
			VmName:        c.VM.name,
			ContainerName: c.containerName,
			OwnerId:       c.VM.Concierge.ownerID,
			DesktopFileId: desktopFileID,
		}, resp); err != nil {
		return err
	}

	if resp.Status != cpb.UninstallPackageOwningFileResponse_STARTED {
		return errors.Errorf("failed to start package uninstall: %v", resp.FailureReason)
	}

	// Wait for the signal for uninstall completion which will signify success or
	// failure.
	testing.ContextLog(ctx, "Waiting for UninstallPackageProgress D-Bus signal")
	sigResult := &cpb.UninstallPackageProgressSignal{}
	for {
		if err := waitForDBusSignal(ctx, progress, nil, sigResult); err != nil {
			return err
		}
		if sigResult.VmName == c.VM.name && sigResult.ContainerName == c.containerName &&
			sigResult.OwnerId == c.VM.Concierge.ownerID {
			if sigResult.Status == cpb.UninstallPackageProgressSignal_SUCCEEDED {
				return nil
			}
			if sigResult.Status == cpb.UninstallPackageProgressSignal_FAILED {
				return errors.Errorf("failure with package uninstall: %v", sigResult.FailureDetails)
			}
		}
	}
}

// CreateCopy makes a copy of the container as the given name.
func (c *Container) CreateCopy(ctx context.Context, name string) error {
	// Wait for background operations to complete before stopping the container.
	if err := c.VM.WaitForLXDOperations(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for LXD background operations")
	}

	// Stop the container.
	// It prevents race conditions when the container is using the filesystem.
	if err := c.Stop(ctx); err != nil {
		return errors.Wrap(err, "failed to stop the container before taking snapshot")
	}

	if _, err := c.VM.LXCCommand(ctx, "copy", c.containerName, name); err != nil {
		return errors.Wrap(err, "failed to copy container")
	}

	if err := c.StartAndWait(ctx); err != nil {
		return errors.Wrap(err, "failed to start container after creating snapshot")
	}

	// Wait until a basic command works. Running commands immediately after
	// container restart may result in racing issues.
	if err := testing.Poll(
		ctx,
		func(ctx context.Context) error {
			return c.Command(ctx, "pwd").Run(testexec.DumpLogOnError)
		},
		&testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to run basic commands after restore")
	}
	return nil
}

// RestoreCopy restores the default container with a copy with the given name.
func (c *Container) RestoreCopy(ctx context.Context, name string) error {
	// Wait for background operations to complete before stopping the container.
	if err := c.VM.WaitForLXDOperations(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for LXD background operations")
	}

	// Stop the container.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// First check if the container is already stopped.
		out, err := c.VM.LXCCommand(ctx, "list", "-cs", "-fcsv", c.containerName)
		if err != nil {
			return err
		}
		if string(out) == "STOPPED\n" {
			return nil
		}

		_, err = c.VM.LXCCommand(ctx, "stop", "-f", c.containerName)
		return err
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to stop the container")
	}

	if _, err := c.VM.LXCCommand(ctx, "delete", "-f", c.containerName); err != nil {
		return errors.Wrap(err, "failed to delete container")
	}

	if _, err := c.VM.LXCCommand(ctx, "copy", name, c.containerName); err != nil {
		return errors.Wrap(err, "failed to copy container")
	}

	if err := c.StartAndWait(ctx); err != nil {
		return errors.Wrap(err, "failed to start container after restoring copy")
	}

	return nil
}

// DeleteCopy deletes a container copy with the given name.
func (c *Container) DeleteCopy(ctx context.Context, name string) error {
	if _, err := c.VM.LXCCommand(ctx, "delete", "-f", name); err != nil {
		return errors.Wrap(err, "failed to delete container")
	}
	return nil
}

// CheckCopy checks whether a container copy of the given name exists.
func (c *Container) CheckCopy(ctx context.Context, name string) (bool, error) {
	// List the snapshots.
	result, err := c.VM.LXCCommand(ctx, "list", "--format", "json")
	if err != nil {
		return false, errors.Wrapf(err, "failed to list containers, stdout: %s", result)
	}

	// The first level of the list result is an array.
	var out []map[string]interface{}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		return false, errors.Wrap(err, "failed to parse output into map")
	}
	if len(out) == 0 {
		return false, errors.New("the output of lxc list is empty")
	}

	for _, instance := range out {
		val, ok := instance["name"]
		if !ok {
			continue
		}

		if val.(string) == name {
			return true, nil
		}
	}

	return false, nil
}

// DefaultContainerCommand returns a testexec.Cmd with a vsh command that will run in
// the default termina/penguin container.
func DefaultContainerCommand(ctx context.Context, ownerID string, vshArgs ...string) *testexec.Cmd {
	return containerCommand(ctx, DefaultVMName, DefaultContainerName, ownerID, vshArgs...)
}

// Command returns a testexec.Cmd with a vsh command that will run in this
// container.
func (c *Container) Command(ctx context.Context, vshArgs ...string) *testexec.Cmd {
	return containerCommand(ctx, c.VM.name, c.containerName, c.VM.Concierge.ownerID, vshArgs...)
}

func ciceroneDBusMatchSpec(memberName string) dbusutil.MatchSpec {
	return dbusutil.MatchSpec{
		Type:      "signal",
		Path:      ciceronePath,
		Interface: ciceroneInterface,
		Member:    memberName,
	}
}

// ShrinkContainer deletes a lot of large files in the container to make the
// image size smaller. This makes a big speed difference on slow devices for
// backup and restore and reduces disk overhead.
func ShrinkContainer(ctx context.Context, cont *Container) error {
	if out, err := cont.Command(ctx, "sudo", "du", "-bsx", "/").Output(testexec.DumpLogOnError); err == nil {
		testing.ContextLogf(ctx, "du before shrinking: %s", strings.TrimSpace(string(out)))
	}
	// This list was constructed by running: `sudo du -bx / | sort -n`,
	// and then deleting paths and checking that the container can still
	// be restarted.
	cleanupPaths := []string{
		"/usr/lib/gcc",
		"/usr/lib/git-core",
		"/usr/lib/python3",
		"/usr/lib/udev",
		"/usr/lib/*-linux-gnu/dri",
		"/usr/lib/*-linux-gnu/samba",
		"/usr/lib/*-linux-gnu/libLLVM-*",
		"/usr/share/doc",
		"/usr/share/fonts/X11",
		"/usr/share/fonts/truetype",
		"/usr/share/i18n",
		"/usr/share/icons",
		"/usr/share/locale",
		"/usr/share/man",
		"/usr/share/perl",
		"/usr/share/qt5",
		"/usr/share/qt6",
		"/usr/share/samba",
		"/usr/share/vim",
		"/var/cache",
		"/var/lib/apt",
		"/var/lib/dpkg",
	}
	if err := cont.Command(ctx, "sudo", "sh", "-c", fmt.Sprintf("rm -rf %s", strings.Join(cleanupPaths, " "))).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to delete files")
	}

	if err := cont.Command(ctx, "sudo", "journalctl", "--rotate", "--vacuum-size=1").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to clear journal")
	}

	if out, err := cont.Command(ctx, "sudo", "du", "-bsx", "/").Output(testexec.DumpLogOnError); err == nil {
		testing.ContextLogf(ctx, "du after shrinking:  %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// ContainerCreationWatcher is a wrapper of SignalWatcher to trace container creation progress.
type ContainerCreationWatcher struct {
	cont    *Container
	watcher *dbusutil.SignalWatcher
}

// NewContainerCreationWatcher returns a ContainerCreationWatcher.
func NewContainerCreationWatcher(ctx context.Context, cont *Container) (*ContainerCreationWatcher, error) {
	watcher, err := dbusutil.NewSignalWatcherForSystemBus(ctx,
		ciceroneDBusMatchSpec("LxdContainerDownloading"), ciceroneDBusMatchSpec("LxdContainerCreated"))
	if err != nil {
		return nil, err
	}
	return &ContainerCreationWatcher{cont, watcher}, nil
}

// Close cleans up the SignalWatcher.
func (c *ContainerCreationWatcher) Close(ctx context.Context) {
	c.watcher.Close(ctx)
}

// isWatchingContainer returns whether the signal is for the container we are watching.
func (c *ContainerCreationWatcher) isWatchingContainer(vmName, containerName, ownerID string) bool {
	return vmName == c.cont.VM.name && containerName == c.cont.containerName && ownerID == c.cont.VM.Concierge.ownerID
}

// WaitForDownload waits for cicerone to send a container download notification.
// If pct is negative, this method returns after the next notification is received.
// Otherwise, it returns only after a notification with percent pct in [0, 100] is received.
// An error is returned if ctx's deadline is reached.
func (c *ContainerCreationWatcher) WaitForDownload(ctx context.Context, pct int32) error {
	spec := ciceroneDBusMatchSpec("LxdContainerDownloading")
	sigResult := &cpb.LxdContainerDownloadingSignal{}
	for {
		if err := waitForDBusSignal(ctx, c.watcher, &spec, sigResult); err != nil {
			return err
		}
		if c.isWatchingContainer(sigResult.VmName, sigResult.ContainerName, sigResult.OwnerId) {
			if pct < 0 || sigResult.DownloadProgress == pct {
				return nil
			}
		}
	}
}

// WaitForCreationComplete waits for the container to be created.
func (c *ContainerCreationWatcher) WaitForCreationComplete(ctx context.Context) error {
	spec := ciceroneDBusMatchSpec("LxdContainerCreated")
	sigResult := &cpb.LxdContainerCreatedSignal{}
	for {
		if err := waitForDBusSignal(ctx, c.watcher, &spec, sigResult); err != nil {
			return err
		}
		if c.isWatchingContainer(sigResult.VmName, sigResult.ContainerName, sigResult.OwnerId) {
			if sigResult.GetStatus() == cpb.LxdContainerCreatedSignal_CREATED {
				return nil
			}
		}
	}
}
