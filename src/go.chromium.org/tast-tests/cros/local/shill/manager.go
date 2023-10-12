// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shill

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/sysutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

const (
	dbusManagerPath      = "/" // crosbug.com/20135
	dbusManagerInterface = "org.chromium.flimflam.Manager"
	testProfileName      = "test"
	fakeProfilesDir      = "/run/shill/user_profiles"
)

// Exported constants
const (
	EnableWaitTime = 500 * time.Millisecond
)

// Manager wraps a Manager D-Bus object in shill.
type Manager struct {
	*PropertyHolder
}

// Technology is the type of a shill device's technology
type Technology string

// TetheringConfig wraps all the properties for a tethering configuration.
type TetheringConfig struct {
	Ssid               string
	Passphrase         string
	AutoDisable        bool
	Security           string
	UpstreamTechnology string
	Band               string
	MAR                bool
}

// Device technologies
// Refer to Flimflam type options in
// https://chromium.googlesource.com/chromiumos/platform2/+/refs/heads/main/system_api/dbus/shill/dbus-constants.h#334
const (
	TechnologyCellular Technology = shillconst.TypeCellular
	TechnologyEthernet Technology = shillconst.TypeEthernet
	TechnologyPPPoE    Technology = shillconst.TypePPPoE
	TechnologyVPN      Technology = shillconst.TypeVPN
	TechnologyWifi     Technology = shillconst.TypeWifi
)

// ErrProfileNotFound is the error returned by ProfileByName when the requested
// profile does not exist in Shill.
var ErrProfileNotFound = errors.New("profile not found")

// NewManager connects to shill's Manager.
func NewManager(ctx context.Context) (*Manager, error) {
	ph, err := NewPropertyHolder(ctx, dbusService, dbusManagerInterface, dbusManagerPath)
	if err != nil {
		return nil, err
	}
	return &Manager{PropertyHolder: ph}, nil
}

// FindMatchingService returns the first Service that matches |expectProps|.
// If no matching Service is found, returns shillconst.ErrorMatchingServiceNotFound.
// Note that the complete list of Services is searched, including those with Visible=false.
// To find only visible services, please specify Visible=true in expectProps.
func (m *Manager) FindMatchingService(ctx context.Context, expectProps map[string]interface{}) (*Service, error) {
	ctx, st := timing.Start(ctx, "m.FindMatchingService")
	defer st.End()

	var servicePath dbus.ObjectPath
	if err := m.Call(ctx, "FindMatchingService", expectProps).Store(&servicePath); err != nil {
		return nil, err
	}
	service, err := NewService(ctx, servicePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to instantiate service %s", servicePath)
	}
	return service, nil
}

// WaitForServiceProperties returns the first matching Service who has the expected properties.
// If there's no matching service, it polls until timeout is reached.
// Noted that it searches all services including Visible=false ones. To focus on visible services,
// please specify Visible=true in expectProps.
func (m *Manager) WaitForServiceProperties(ctx context.Context, expectProps map[string]interface{}, timeout time.Duration) (*Service, error) {
	var service *Service
	if err := testing.Poll(ctx, func(ctx context.Context) (e error) {
		service, e = m.FindMatchingService(ctx, expectProps)
		return e
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return nil, err
	}
	return service, nil
}

// ProfilePaths returns a list of profile paths.
func (m *Manager) ProfilePaths(ctx context.Context) ([]dbus.ObjectPath, error) {
	p, err := m.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	return p.GetObjectPaths(shillconst.ManagerPropertyProfiles)
}

// Profiles returns a list of profiles.
func (m *Manager) Profiles(ctx context.Context) ([]*Profile, error) {
	paths, err := m.ProfilePaths(ctx)
	if err != nil {
		return nil, err
	}

	profiles := make([]*Profile, len(paths))
	for i, path := range paths {
		profiles[i], err = NewProfile(ctx, path)
		if err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

// ActiveProfile returns the active profile.
func (m *Manager) ActiveProfile(ctx context.Context) (*Profile, error) {
	props, err := m.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	path, err := props.GetObjectPath(shillconst.ManagerPropertyActiveProfile)
	if err != nil {
		return nil, err
	}
	return NewProfile(ctx, path)
}

// Devices returns a list of devices.
func (m *Manager) Devices(ctx context.Context) ([]*Device, error) {
	p, err := m.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	paths, err := p.GetObjectPaths(shillconst.ManagerPropertyDevices)
	if err != nil {
		return nil, err
	}
	devs := make([]*Device, 0, len(paths))
	for _, path := range paths {
		d, err := NewDevice(ctx, path)
		if err != nil {
			return nil, err
		}
		devs = append(devs, d)
	}
	return devs, nil
}

// DeviceByType returns a device matching |type| or a "Device not found" error.
func (m *Manager) DeviceByType(ctx context.Context, deviceType string) (*Device, error) {
	devices, err := m.Devices(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		properties, err := d.GetProperties(ctx)
		if err != nil {
			return nil, err
		}
		t, err := properties.GetString(shillconst.DevicePropertyType)
		if err != nil {
			return nil, err
		}
		if t == deviceType {
			return d, nil
		}
	}
	return nil, errors.New("Device not found")
}

// ServicePaths returns a list of service paths.
func (m *Manager) ServicePaths(ctx context.Context) ([]dbus.ObjectPath, error) {
	p, err := m.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	return p.GetObjectPaths(shillconst.ManagerPropertyServiceCompleteList)
}

// Services returns a list of services.
func (m *Manager) Services(ctx context.Context) ([]*Service, error) {
	paths, err := m.ServicePaths(ctx)
	if err != nil {
		return nil, err
	}

	profiles := make([]*Service, len(paths))
	for i, path := range paths {
		profiles[i], err = NewService(ctx, path)
		if err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

// ServicesByTechnology returns list of Services and their Properties snapshots of the specified technology.
func (m *Manager) ServicesByTechnology(ctx context.Context, technology Technology) ([]*Service, []*dbusutil.Properties, error) {
	var matches []*Service
	var props []*dbusutil.Properties

	services, err := m.Services(ctx)
	if err != nil {
		return nil, nil, err
	}

	for _, service := range services {
		p, err := service.GetProperties(ctx)
		if err != nil {
			if dbusutil.IsDBusError(err, dbusutil.DBusErrorUnknownObject) {
				// This error is forgivable as a device may go down anytime.
				continue
			}
			return nil, nil, err
		}
		if serviceType, err := p.GetString(shillconst.ServicePropertyType); err != nil {
			testing.ContextLogf(ctx, "Error getting the type of the service %q: %v", service, err)
			continue
		} else if serviceType != string(technology) {
			continue
		}
		matches = append(matches, service)
		props = append(props, p)
	}
	return matches, props, nil
}

// ConfigureService configures a service with the given properties and returns its path.
func (m *Manager) ConfigureService(ctx context.Context, props map[string]interface{}) (dbus.ObjectPath, error) {
	var service dbus.ObjectPath
	if err := m.Call(ctx, "ConfigureService", props).Store(&service); err != nil {
		return "", errors.Wrap(err, "failed to configure service")
	}
	return service, nil
}

// ConfigureServiceForProfile configures a service at the given profile path.
func (m *Manager) ConfigureServiceForProfile(ctx context.Context, path dbus.ObjectPath, props map[string]interface{}) (dbus.ObjectPath, error) {
	var service dbus.ObjectPath
	if err := m.Call(ctx, "ConfigureServiceForProfile", path, props).Store(&service); err != nil {
		return "", errors.Wrap(err, "failed to configure service")
	}
	return service, nil
}

// CreateProfile creates a profile.
func (m *Manager) CreateProfile(ctx context.Context, name string) (dbus.ObjectPath, error) {
	var profile dbus.ObjectPath
	if err := m.Call(ctx, "CreateProfile", name).Store(&profile); err != nil {
		return "", errors.Wrap(err, "failed to create profile")
	}
	return profile, nil
}

// PushProfile pushes a profile.
func (m *Manager) PushProfile(ctx context.Context, name string) (dbus.ObjectPath, error) {
	var profile dbus.ObjectPath
	if err := m.Call(ctx, "PushProfile", name).Store(&profile); err != nil {
		return "", errors.Wrap(err, "failed to create profile")
	}
	return profile, nil
}

// RemoveProfile removes the profile with the given name.
func (m *Manager) RemoveProfile(ctx context.Context, name string) error {
	return m.Call(ctx, "RemoveProfile", name).Err
}

// PopProfile pops the profile with the given name if it is on top of the stack.
func (m *Manager) PopProfile(ctx context.Context, name string) error {
	return m.Call(ctx, "PopProfile", name).Err
}

// ProfileByName returns a profile matching |name| or an ErrProfileNotFound if
// the profile does not exist.
func (m *Manager) ProfileByName(ctx context.Context, name string) (*Profile,
	error) {
	profiles, err := m.Profiles(ctx)
	if err != nil {
		return nil, err
	}

	for _, p := range profiles {
		properties, err := p.GetProperties(ctx)
		if err != nil {
			return nil, err
		}

		n, err := properties.GetString(shillconst.ProfilePropertyName)
		if err != nil {
			return nil, err
		}

		if n == name {
			return p, nil
		}
	}
	return nil, ErrProfileNotFound
}

// PopAllUserProfiles removes all user profiles from the stack of managed profiles leaving only default profiles.
func (m *Manager) PopAllUserProfiles(ctx context.Context) error {
	return m.Call(ctx, "PopAllUserProfiles").Err
}

// RequestScan requests a scan for the specified technology.
func (m *Manager) RequestScan(ctx context.Context, technology Technology) error {
	return m.Call(ctx, "RequestScan", string(technology)).Err
}

// EnableTechnology enables a technology interface.
func (m *Manager) EnableTechnology(ctx context.Context, technology Technology) error {
	return m.Call(ctx, "EnableTechnology", string(technology)).Err
}

// DisableTechnology disables a technology interface.
func (m *Manager) DisableTechnology(ctx context.Context, technology Technology) error {
	return m.Call(ctx, "DisableTechnology", string(technology)).Err
}

// GetEnabledTechnologies returns a list of all enabled shill networking technologies.
func (m *Manager) GetEnabledTechnologies(ctx context.Context) ([]Technology, error) {
	var enabledTechnologies []Technology
	prop, err := m.GetProperties(ctx)
	if err != nil {
		return enabledTechnologies, errors.Wrap(err, "failed to get properties")
	}
	technologies, err := prop.GetStrings(shillconst.ManagerPropertyEnabledTechnologies)
	if err != nil {
		return enabledTechnologies, errors.Wrapf(err, "failed to get property: %s", shillconst.ManagerPropertyEnabledTechnologies)
	}
	for _, t := range technologies {
		enabledTechnologies = append(enabledTechnologies, Technology(t))
	}
	return enabledTechnologies, nil
}

func (m *Manager) hasTechnology(ctx context.Context, technologyProperty string, technology Technology) (bool, error) {
	prop, err := m.GetProperties(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get properties")
	}
	technologies, err := prop.GetStrings(technologyProperty)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get property: %s", technologyProperty)
	}
	for _, t := range technologies {
		if t == string(technology) {
			return true, nil
		}
	}
	return false, nil
}

// IsAvailable returns true if a technology is available.
func (m *Manager) IsAvailable(ctx context.Context, technology Technology) (bool, error) {
	return m.hasTechnology(ctx, shillconst.ManagerPropertyAvailableTechnologies, technology)
}

// IsEnabled returns true if a technology is enabled.
func (m *Manager) IsEnabled(ctx context.Context, technology Technology) (bool, error) {
	return m.hasTechnology(ctx, shillconst.ManagerPropertyEnabledTechnologies, technology)
}

// DisableTechnologyForTesting first checks whether |technology| is enabled.
// If it is enabled, it disables the technology and returns a callback that can
// be deferred to re-enable the technology when the test completes.
// (The callback should take a contexted shortened by shill.EnableWaitTime).
// If the technology is already enabled, no work is done and a nil callback is returned
func (m *Manager) DisableTechnologyForTesting(ctx context.Context, technology Technology) (func(ctx context.Context), error) {
	enabled, err := m.IsEnabled(ctx, technology)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to determine enabled state for %s", string(technology))
	}
	if !enabled {
		return nil, nil
	}
	if err := m.DisableTechnology(ctx, technology); err != nil {
		return nil, errors.Wrapf(err, "unable to disable technology: %s", string(technology))
	}
	return func(ctx context.Context) {
		m.EnableTechnology(ctx, technology)
		// Ensure the Enable completes before the test ends so that cleanup will succeed.
		const interval = 100 * time.Millisecond
		testing.Poll(ctx, func(ctx context.Context) error {
			enabled, err := m.IsEnabled(ctx, technology)
			if err != nil {
				return errors.Wrap(err, "failed to get enabled state")
			}
			if !enabled {
				return errors.New("not enabled")
			}
			return nil
		}, &testing.PollOptions{
			// Subtract |interval| seconds from EnableWaitTime for Timeout so that
			// this completes or throws an error before a context shortened by
			// EnableWaitTime times out.
			Timeout:  EnableWaitTime - interval,
			Interval: interval,
		})
	}, nil
}

// DevicesByTechnology returns list of Devices and their Properties snapshots of the specified technology.
func (m *Manager) DevicesByTechnology(ctx context.Context, technology Technology) ([]*Device, []*dbusutil.Properties, error) {
	var matches []*Device
	var props []*dbusutil.Properties

	devs, err := m.Devices(ctx)
	if err != nil {
		return nil, nil, err
	}

	for _, dev := range devs {
		p, err := dev.GetProperties(ctx)
		if err != nil {
			if dbusutil.IsDBusError(err, dbusutil.DBusErrorUnknownObject) {
				// This error is forgivable as a device may go down anytime.
				continue
			}
			return nil, nil, err
		}
		if devType, err := p.GetString(shillconst.DevicePropertyType); err != nil {
			testing.ContextLogf(ctx, "Error getting the type of the device %q: %v", dev, err)
			continue
		} else if devType != string(technology) {
			continue
		}
		matches = append(matches, dev)
		props = append(props, p)
	}
	return matches, props, nil
}

// DeviceByName returns the Device matching the given interface name.
func (m *Manager) DeviceByName(ctx context.Context, iface string) (*Device, error) {
	devs, err := m.Devices(ctx)
	if err != nil {
		return nil, err
	}

	for _, dev := range devs {
		p, err := dev.GetProperties(ctx)
		if err != nil {
			if dbusutil.IsDBusError(err, dbusutil.DBusErrorUnknownObject) {
				// This error is forgivable as a device may go down anytime.
				continue
			}
			return nil, err
		}
		if devIface, err := p.GetString(shillconst.DevicePropertyInterface); err != nil {
			testing.ContextLogf(ctx, "Error getting the device interface %q: %v", dev, err)
			continue
		} else if devIface == iface {
			return dev, nil
		}
	}
	return nil, errors.New("unable to find matching device")
}

// WaitForDeviceByName returns the Device matching the given interface name.
// If there's no match, it waits until one appears, or until timeout.
func (m *Manager) WaitForDeviceByName(ctx context.Context, iface string, timeout time.Duration) (*Device, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pw, err := m.CreateWatcher(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a PropertiesWatcher")
	}
	defer pw.Close(ctx)

	for {
		if d, err := m.DeviceByName(ctx, iface); err == nil {
			return d, nil
		}

		if _, err := pw.WaitAll(ctx, shillconst.ManagerPropertyDevices); err != nil {
			return nil, err
		}
	}
}

// SetDebugTags sets the debug tags that are enabled for logging. "tags" is a list of valid tag names separated by "+".
// Shill silently ignores invalid flags.
func (m *Manager) SetDebugTags(ctx context.Context, tags []string) error {
	return m.Call(ctx, "SetDebugTags", strings.Join(tags, "+")).Err
}

// GetDebugTags gets the list of enabled debug tags. The list is represented as a string of tag names separated by "+".
func (m *Manager) GetDebugTags(ctx context.Context) ([]string, error) {
	var tags string
	if err := m.Call(ctx, "GetDebugTags").Store(&tags); err != nil {
		return nil, err
	}
	tagsArr := strings.Split(tags, "+")
	return tagsArr, nil
}

// SetDebugLevel sets the debugging level.
func (m *Manager) SetDebugLevel(ctx context.Context, level int) error {
	return m.Call(ctx, "SetDebugLevel", level).Err
}

// GetDebugLevel gets the enabled debug level.
func (m *Manager) GetDebugLevel(ctx context.Context) (int, error) {
	var level int
	if err := m.Call(ctx, "GetDebugLevel").Store(&level); err != nil {
		return 0, err
	}
	return level, nil
}

// RecheckPortal requests shill to rerun its captive portal detector.
func (m *Manager) RecheckPortal(ctx context.Context) error {
	return m.Call(ctx, "RecheckPortal").Err
}

// RemoveTestProfile removes any existing test profile.
func (m *Manager) RemoveTestProfile(ctx context.Context) error {
	// Try popping the test profile from the stack if it exists, otherwise the
	// following removal will fail.
	errNotExist := testProfileName + " is not the active profile"
	if err := m.PopProfile(ctx, testProfileName); err != nil && err.Error() != errNotExist {
		return errors.Wrap(err, "failed to pop test profile before remove")
	}
	if err := m.RemoveProfile(ctx, testProfileName); err != nil {
		return errors.Wrap(err, "RemoveTestProfile failed")
	}
	return nil
}

// PushTestProfile creates and pushes a Shill profile for testing.
// The profile is not a user profile so it will not be reloaded if Shill crashes
// or the DUT restarts, making it safe to save changes to it while testing.
// Assumes that there are no user profiles loaded (otherwise the test profile will be ignored).
// Returns a function that should be deferred to pop and remove the profile.
func (m *Manager) PushTestProfile(ctx context.Context) (func(ctx context.Context), error) {
	if err := m.RemoveTestProfile(ctx); err != nil {
		return nil, err
	}
	if _, err := m.CreateProfile(ctx, testProfileName); err != nil {
		return nil, errors.Wrap(err, "CreateProfile failed")
	}
	if _, err := m.PushProfile(ctx, testProfileName); err != nil {
		return nil, errors.Wrap(err, "PushProfile failed")
	}
	return func(ctx context.Context) {
		if err := m.PopProfile(ctx, testProfileName); err != nil {
			testing.ContextLog(ctx, "Error popping test profile: ", err)
			return
		}
		if err := m.RemoveProfile(ctx, testProfileName); err != nil {
			testing.ContextLog(ctx, "Error removing test profile: ", err)
			return
		}
	}, nil
}

// ClaimInterface assigns the ownership of "intf" to the specified "claimer".
// The claimer will prevent Shill to manage the device (see shill/doc/manager-api.txt).
func (m *Manager) ClaimInterface(ctx context.Context, claimer, intf string) error {
	return m.Call(ctx, "ClaimInterface", claimer, intf).Err
}

// ReleaseInterface takes the ownership of "intf" from the specified "claimer"
// to give it back to Shill.
func (m *Manager) ReleaseInterface(ctx context.Context, claimer, intf string) error {
	return m.Call(ctx, "ReleaseInterface", claimer, intf).Err
}

// AddPasspointCredentials adds a set of Passpoint credentials to "profile"
// using the method AddPasspointCredentials() from Shill Manager. "props" is a
// map of properties that describes the set of credentials, the keys are defined
// in tast/common/shillconst.
func (m *Manager) AddPasspointCredentials(ctx context.Context, profile dbus.ObjectPath, props map[string]interface{}) error {
	return m.Call(ctx, "AddPasspointCredentials", profile, props).Err
}

// SetInterworkingSelectEnabled sets the "interworking enabled" property to
// |enabled| for the Wi-Fi device |iface|.
func (m *Manager) SetInterworkingSelectEnabled(ctx context.Context, iface string, enabled bool) error {
	dev, err := m.DeviceByName(ctx, iface)
	if err != nil {
		return errors.Wrapf(err, "failed to obtain device for interface %s", iface)
	}
	err = dev.PropertyHolder.SetProperty(ctx, shillconst.DevicePropertyPasspointInterworkingSelectEnabled, enabled)
	if err != nil {
		return errors.Wrapf(err, "failed to set interworking selection on %s", iface)
	}
	return nil
}

// CreateFakeUserProfile creates a fake user profile in Shill on top of the
// default profile for test purpose.
func (m *Manager) CreateFakeUserProfile(ctx context.Context, name string) (path dbus.ObjectPath, err error) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// To be a Shill user profile, the profile name must match the format "~user/identifier".
	profileName := fmt.Sprintf("~%s/shill", name)
	profilePath := filepath.Join(fakeProfilesDir, name)

	// Pop all user profiles to be sure there's no Passpoint configuration leftovers.
	// Note: Passpoint credentials can't be pushed to default profiles.
	if err := m.PopAllUserProfiles(ctx); err != nil {
		return "", errors.Wrap(err, "failed to pop user profiles")
	}

	// Remove the test profile if it still exists.
	p, err := m.ProfileByName(ctx, profileName)
	if !errors.Is(err, ErrProfileNotFound) {
		return "", err
	} else if p != nil {
		if err := m.RemoveProfile(ctx, profileName); err != nil {
			return "", errors.Wrapf(err, "failed to remove profile %s", profileName)
		}
	}

	// Obtain Shill UID et GID
	uid, err := sysutil.GetUID("shill")
	if err != nil {
		return "", errors.Wrap(err, "failed to obtain Shill user id")
	}
	gid, err := sysutil.GetGID("shill")
	if err != nil {
		return "", errors.Wrap(err, "failed to obtain Shill group id")
	}

	// Create a directory for the test user profile
	if err := os.MkdirAll(profilePath, 0700); err != nil {
		return "", errors.Wrap(err, "failed to create profile dir")
	}
	defer func(ctx context.Context) {
		if err != nil {
			if err := os.RemoveAll(profilePath); err != nil {
				testing.ContextLogf(ctx, "Failed to remove %s: %v", profilePath, err)
			}
		}
	}(cleanupCtx)
	if err := os.Chown(fakeProfilesDir, int(uid), int(gid)); err != nil {
		return "", errors.Wrap(err, "failed to chown")
	}
	if err := os.Chown(profilePath, int(uid), int(gid)); err != nil {
		return "", errors.Wrap(err, "failed to chown")
	}

	// Create and push the profile
	if _, err = m.CreateProfile(ctx, profileName); err != nil {
		return "", errors.Wrap(err, "failed to create profile")
	}
	defer func(ctx context.Context) {
		if err != nil {
			if err := m.RemoveProfile(ctx, profileName); err != nil {
				testing.ContextLogf(ctx, "Failed to remove profile %s: %v", profileName, err)
			}
		}
	}(cleanupCtx)
	path, err = m.PushProfile(ctx, profileName)
	if err != nil {
		return "", errors.Wrap(err, "failed to push profile")
	}

	return path, nil
}

// RemoveFakeUserProfile removes the fake Shill user profile created for test purpose.
func (m *Manager) RemoveFakeUserProfile(ctx context.Context, name string) error {
	profileName := fmt.Sprintf("~%s/shill", name)
	profilePath := filepath.Join(fakeProfilesDir, name)

	if err := m.PopProfile(ctx, profileName); err != nil {
		return errors.Wrapf(err, "failed to pop profile %s", profileName)
	}
	if err := m.RemoveProfile(ctx, profileName); err != nil {
		return errors.Wrapf(err, "failed to remove profile %s", profileName)
	}
	if err := os.RemoveAll(profilePath); err != nil {
		return errors.Wrapf(err, "failed to remove profile directory %s", profilePath)
	}
	return nil
}

// SetDNSProxyDOHProviders updates the mapping of DoH providers to nameserver(s).
// Nameservers are represented as string separated by ','.
func (m *Manager) SetDNSProxyDOHProviders(ctx context.Context, dohProviders map[string]interface{}) error {
	return m.Call(ctx, "SetDNSProxyDOHProviders", dohProviders).Err
}

// EnablePortalDetection will enable portal detection for the default technologies.
// This is useful for portal detection testing which uses the shillReset test fixture,
// as a shill reset will remove the default profile which makes it so no technologies
// are enabled for portal detection.
func (m *Manager) EnablePortalDetection(ctx context.Context) error {
	return m.SetPortalDetection(ctx, shillconst.PortalDetectorDefaultCheckPortalList)
}

// DisablePortalDetection disables portal detection for all technologies.
func (m *Manager) DisablePortalDetection(ctx context.Context) error {
	return m.SetPortalDetection(ctx, "")
}

// DisablePortalDetectionWithRestore disables portal detection for all
// technologies and returns a function to restore the portal detection to its
// previous state.
func (m *Manager) DisablePortalDetectionWithRestore(ctx context.Context) (func(context.Context), error) {
	portals, err := m.GetPortalDetection(ctx)
	if err != nil {
		return nil, err
	}
	if err := m.DisablePortalDetection(ctx); err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		if err := m.SetPortalDetection(ctx, portals); err != nil {
			testing.ContextLog(ctx, "Failed to restore portal detection: ", err)
		}
	}, nil
}

// SetPortalDetection enables portal detection for the technologies in the cpList.
func (m *Manager) SetPortalDetection(ctx context.Context, cpList string) error {
	if err := m.SetProperty(ctx, shillconst.ProfilePropertyCheckPortalList, cpList); err != nil {
		return errors.Wrapf(err, "failed to set portal detection on %q", cpList)
	}
	return nil
}

// GetPortalDetection returns the portal detection technologies.
func (m *Manager) GetPortalDetection(ctx context.Context) (string, error) {
	if properties, err := m.GetProperties(ctx); err != nil {
		return "", err
	} else if list, err := properties.GetString(shillconst.ProfilePropertyCheckPortalList); err != nil {
		return "", err
	} else {
		return list, nil
	}
}

// WaitForUserProfile waits until shill got the right global and user profile and returns the user profile.
func (m *Manager) WaitForUserProfile(ctx context.Context) (dbus.ObjectPath, error) {
	var path dbus.ObjectPath
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		paths, err := m.ProfilePaths(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get profile paths"))
		}
		if len(paths) > 2 {
			return testing.PollBreak(errors.Errorf("too many profiles: got %d, want 2", len(paths)))
		}
		if len(paths) == 2 {
			// Last profile is the user profile.
			path = paths[len(paths)-1]
			return nil
		}
		return errors.New("failed to wait for user profile")
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return "", err
	}
	return path, nil
}

// GetTetheringConfig returns the tethering configurations.
func (m *Manager) GetTetheringConfig(ctx context.Context) (TetheringConfig, error) {
	var tetheringConfig TetheringConfig
	p, err := m.GetProperties(ctx)
	if err != nil {
		return tetheringConfig, err
	}

	p, err = p.GetMap(shillconst.ManagerPropertyTetheringConfig)
	if err != nil {
		return tetheringConfig, errors.Wrap(err, "failed to get tethering config")
	}

	for _, fn := range []struct {
		field *string
		name  string
	}{{&tetheringConfig.Passphrase, shillconst.TetheringConfPassphrase},
		{&tetheringConfig.Band, shillconst.TetheringConfBand},
		{&tetheringConfig.Security, shillconst.TetheringConfSecurity},
		{&tetheringConfig.UpstreamTechnology, shillconst.TetheringConfUpstreamTech},
	} {
		*fn.field, err = p.GetString(fn.name)
		if err != nil {
			return tetheringConfig, errors.Wrapf(err, "failed to get property %s", fn.name)
		}
	}

	encodedSsid, err := p.GetString(shillconst.TetheringConfSSID)
	if err != nil {
		return tetheringConfig, errors.Wrapf(err, "failed to get property %s", shillconst.TetheringConfSSID)
	}
	decodedBytes, err := hex.DecodeString(encodedSsid)
	if err != nil {
		return tetheringConfig, errors.Wrapf(err, "failed to get decode ssid: %s", encodedSsid)
	}
	tetheringConfig.Ssid = string(decodedBytes[:])

	for _, fn := range []struct {
		field *bool
		name  string
	}{{&tetheringConfig.AutoDisable, shillconst.TetheringConfAutoDisable},
		{&tetheringConfig.MAR, shillconst.TetheringConfMAR},
	} {
		*fn.field, err = p.GetBool(fn.name)
		if err != nil {
			return tetheringConfig, errors.Wrapf(err, "failed to get property %s", fn.name)
		}
	}

	return tetheringConfig, nil
}

// GetTetheringCapabilities returns the tethering capabilities dict.
func (m *Manager) GetTetheringCapabilities(ctx context.Context) (*dbusutil.Properties, error) {
	p, err := m.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	return p.GetMap(shillconst.ManagerPropertyTetheringCapabilities)
}

// GetTetheringCapabilityUpstreamTechnologies returns the upstream technologies from the tethering capabilities dict.
func (m *Manager) GetTetheringCapabilityUpstreamTechnologies(ctx context.Context) ([]Technology, error) {
	p, err := m.GetTetheringCapabilities(ctx)
	if err != nil {
		return nil, err
	}

	technologies, err := p.GetStrings(shillconst.TetheringCapUpstream)
	var upstreamTechnologies []Technology
	if err != nil {
		return upstreamTechnologies, errors.Wrapf(err, "failed to get property: %s", shillconst.TetheringCapUpstream)
	}
	for _, t := range technologies {
		upstreamTechnologies = append(upstreamTechnologies, Technology(t))
	}
	return upstreamTechnologies, nil
}

// GetTetheringCapabilityDownstreamTechnologies returns the downstream technologies from the tethering capabilities dict.
func (m *Manager) GetTetheringCapabilityDownstreamTechnologies(ctx context.Context) ([]Technology, error) {
	p, err := m.GetTetheringCapabilities(ctx)
	if err != nil {
		return nil, err
	}

	technologies, err := p.GetStrings(shillconst.TetheringCapDownstream)
	var downstreamTechnologies []Technology
	if err != nil {
		return downstreamTechnologies, errors.Wrapf(err, "failed to get property: %s", shillconst.TetheringCapDownstream)
	}
	for _, t := range technologies {
		downstreamTechnologies = append(downstreamTechnologies, Technology(t))
	}
	return downstreamTechnologies, nil
}

// ConfigureTethering is a wrapper for conveniently setting of tethering configuration.
func (m *Manager) ConfigureTethering(ctx context.Context, props map[string]interface{}) error {
	return m.SetProperty(ctx, shillconst.ManagerPropertyTetheringConfig, props)
}

// EnableTethering enables tethering.
func (m *Manager) EnableTethering(ctx context.Context) error {
	return m.Call(ctx, "SetTetheringEnabled", true).Err
}

// DisableTethering disables tethering.
func (m *Manager) DisableTethering(ctx context.Context) error {
	return m.Call(ctx, "SetTetheringEnabled", false).Err
}

// TetheringStatus returns Tethering status dict.
func (m *Manager) TetheringStatus(ctx context.Context) (*dbusutil.Properties, error) {
	p, err := m.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	return p.GetMap("TetheringStatus")
}

// WaitForTetheringState waits until the tethering state is the requested one.
func (m *Manager) WaitForTetheringState(ctx context.Context, state string) (*dbusutil.Properties, error) {
	var props *dbusutil.Properties
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		if props, err = m.TetheringStatus(ctx); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read tethering status"))
		}
		if s, err := props.GetString(shillconst.TetheringStatusState); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get tethering status"))
		} else if s != state && s == shillconst.TetheringStateIdle {
			// Try a best effort read of a possible failure reason.
			errMsg, err := props.GetString(shillconst.TetheringStatusIdleReason)
			if err != nil {
				errMsg = "[Unknown]"
			}
			return testing.PollBreak(errors.Errorf("tethering status == %q, for a reason %q", s, errMsg))
		} else if s != state {
			return errors.Errorf("wrong tethering state, got %s, want %s", s, state)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return nil, errors.Wrapf(err, "failed to wait for the correct tethering status %s", state)
	}
	return props, nil
}

// GetDefaultService gets the current default shill service.
func (m *Manager) GetDefaultService(ctx context.Context) (*Service, error) {
	p, err := m.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	path, err := p.GetObjectPath(shillconst.ManagerPropertyDefaultService)
	if err != nil {
		return nil, err
	}
	return NewService(ctx, path)
}

// WaitForDefaultService waits until svc becomes the default service.
func (m *Manager) WaitForDefaultService(ctx context.Context, svc *Service) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		dSvc, err := m.GetDefaultService(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get default service")
		}
		if dSvc.ObjectPath() != svc.ObjectPath() {
			return errors.Errorf("default service mismatch got %v, want %v", dSvc, svc)
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return err
	}
	return nil
}

// GetServiceOrder gets the technology priority.
func (m *Manager) GetServiceOrder(ctx context.Context) ([]string, error) {
	var technologies string
	if err := m.Call(ctx, "GetServiceOrder").Store(&technologies); err != nil {
		return nil, err
	}
	return strings.Split(technologies, ","), nil
}

// SetServiceOrder sets the technology priority.
func (m *Manager) SetServiceOrder(ctx context.Context, technologies []string) error {
	return m.Call(ctx, "SetServiceOrder", strings.Join(technologies, ",")).Err
}

// SetServiceOrderWithRestore sets the technology priority and returns a
// function to restore the previous technology order.
func (m *Manager) SetServiceOrderWithRestore(ctx context.Context, technologies []string) (func(context.Context), error) {
	oldOrder, err := m.GetServiceOrder(ctx)
	if err != nil {
		return nil, err
	}
	if err := m.SetServiceOrder(ctx, technologies); err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		if err := m.SetServiceOrder(ctx, oldOrder); err != nil {
			testing.ContextLog(ctx, "Failed to restore service order: ", err)
		}
	}, nil
}

// CheckTetheringReadiness returns the tethering readiness status.
func (m *Manager) CheckTetheringReadiness(ctx context.Context) (string, error) {
	ctx, st := timing.Start(ctx, "m.CheckTetheringReadiness")
	defer st.End()

	var status string
	if err := m.Call(ctx, "CheckTetheringReadiness").Store(&status); err != nil {
		return "", err
	}

	return status, nil
}

// SetTetheringAllowed sets the TetheringAllowed property.
func (m *Manager) SetTetheringAllowed(ctx context.Context, allowed bool) error {
	if err := m.SetProperty(ctx, shillconst.ManagerPropertyTetheringAllowed, allowed); err != nil {
		return errors.Wrapf(err, "failed to set tethering allowed to %t", allowed)
	}
	return nil
}

// SetExperimentalTetheringFunctionality sets the ExperimentalTetheringFunctionality property.
func (m *Manager) SetExperimentalTetheringFunctionality(ctx context.Context, value bool) error {
	if err := m.SetProperty(ctx, shillconst.ManagerPropertyExperimentalTetheringFunctionality, value); err != nil {
		return errors.Wrapf(err, "failed to set experimental tethering functionality to %t", value)
	}
	return nil
}

// GetNetworksForGeolocation returns geolocation cache
func (m *Manager) GetNetworksForGeolocation(ctx context.Context) (*dbusutil.Properties, error) {
	var geolocationInfoTechnology map[string]interface{}
	if err := m.Call(ctx, "GetNetworksForGeolocation").Store(&geolocationInfoTechnology); err != nil {
		return nil, err
	}
	return dbusutil.NewProperties(geolocationInfoTechnology), nil
}

// ScanAndConnectToBestServices initiates a scan and connects to the best service for each technology.
func (m *Manager) ScanAndConnectToBestServices(ctx context.Context) error {
	return m.Call(ctx, "ScanAndConnectToBestServices").Err
}

// SetEnableDHCPQosWithRestore sets the EnableDHCPQos property and returns a
// helper to restore the property at its previous value.
func (m *Manager) SetEnableDHCPQosWithRestore(ctx context.Context, enable bool) (func(context.Context), error) {
	oldValue, err := m.GetAndSetProperty(ctx, shillconst.ManagerPropertyEnableDHCPQos, enable)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		if err := m.SetProperty(ctx, shillconst.ManagerPropertyEnableDHCPQos, oldValue); err != nil {
			testing.ContextLog(ctx, "Failed to restore EnableDHCPQos property: ", err)
		}
	}, nil
}
