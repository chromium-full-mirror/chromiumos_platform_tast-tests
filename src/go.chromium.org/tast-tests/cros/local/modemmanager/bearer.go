// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package modemmanager

import (
	"context"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

// IPConfig represents a MM IPConfig dbus object
type IPConfig struct {
	Method uint32
	// optional values
	Address string
	Prefix  uint32
	DNS1    string
	DNS2    string
	DNS3    string
	Gateway string
	Mtu     uint32
}

// BearerProperties represents a MM Bearer properties dbus object
type BearerProperties struct {
	apn          string
	allowRoaming bool
	allowedAuth  uint32
	apnType      uint32
	ipType       uint32
	multiplex    uint32
	password     string
	user         string
	profileID    int32
	// Indicators of property existence
	HasApn          bool
	HasAllowRoaming bool
	HasAllowedAuth  bool
	HasApnType      bool
	HasIPType       bool
	HasMultiplex    bool
	HasPassword     bool
	HasUser         bool
	HasProfileID    bool
}

// InvalidProfileID is a sentinel value for profile ID when it is not present or when the profile is not valid.
const (
	InvalidProfileID = -1
)

// Bearer represents a MM Bearer dbus object
type Bearer struct {
	ph    *dbusutil.PropertyHolder
	props *dbusutil.Properties
	path  dbus.ObjectPath
}

// NewBearer creates a new Bearer adapter from bearer properties.
func NewBearer(ctx context.Context, bearerPath dbus.ObjectPath) (*Bearer, error) {
	ph, err := dbusutil.NewPropertyHolder(ctx, DBusModemmanagerService, DBusModemmanagerBearerInterface, bearerPath)
	if err != nil {
		return nil, err
	}

	props, err := ph.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	// Verify that all mandatory properties exist.
	_, err = props.GetString(mmconst.BearerPropertyInterface)
	if err != nil {
		return nil, errors.Wrap(err, "missing interface property")
	}
	_, err = props.GetBool(mmconst.BearerPropertyConnected)
	if err != nil {
		return nil, errors.Wrap(err, "missing connected property")
	}
	_, err = props.GetStructOfStrings(mmconst.BearerPropertyConnectionError)
	if err != nil {
		return nil, errors.Wrap(err, "missing ConnectionError property")
	}
	_, err = props.GetBool(mmconst.BearerPropertyMultiplexed)
	if err != nil {
		return nil, errors.Wrap(err, "missing multiplexed property")
	}
	_, err = parseIPConfig(props, mmconst.BearerPropertyIP4Config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read Ip4Config")
	}
	_, err = parseIPConfig(props, mmconst.BearerPropertyIP6Config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read Ip6Config")
	}

	// Inner Properties
	innerPropsGet, err := props.Get(mmconst.BearerPropertyProperties)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read bearer properties")
	}
	_, ok := innerPropsGet.(map[string]interface{})
	if !ok {
		return nil, errors.New("failed to parse bearer properties")
	}

	return &Bearer{ph: ph, path: bearerPath, props: props}, nil
}

func parseIPConfig(props *dbusutil.Properties, propertyName string) (*IPConfig, error) {
	ipConfigGet, err := props.Get(propertyName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read IPConfig")
	}
	ipConfigMap, ok := ipConfigGet.(map[string]interface{})
	if !ok {
		return nil, errors.New("failed to parse IPConfig")
	}
	var ipConfig IPConfig
	value, ok := ipConfigMap[mmconst.BearerPropertyIPMethod]
	if ok {
		ipConfig.Method, ok = value.(uint32)
	}
	if !ok {
		return nil, errors.New("failed to parse the |method| from IPConfig")
	}
	value, ok = ipConfigMap[mmconst.BearerPropertyIPAddress]
	if ok {
		ipConfig.Address, ok = value.(string)
	}
	value, ok = ipConfigMap[mmconst.BearerPropertyIPPrefix]
	if ok {
		ipConfig.Prefix, ok = value.(uint32)
	}
	value, ok = ipConfigMap[mmconst.BearerPropertyIPDns1]
	if ok {
		ipConfig.DNS1, ok = value.(string)
	}
	value, ok = ipConfigMap[mmconst.BearerPropertyIPDns2]
	if ok {
		ipConfig.DNS2, ok = value.(string)
	}
	value, ok = ipConfigMap[mmconst.BearerPropertyIPDns3]
	if ok {
		ipConfig.DNS3, ok = value.(string)
	}
	value, ok = ipConfigMap[mmconst.BearerPropertyIPGateway]
	if ok {
		ipConfig.Gateway, ok = value.(string)
	}
	value, ok = ipConfigMap[mmconst.BearerPropertyIPMtu]
	if ok {
		ipConfig.Mtu, ok = value.(uint32)
	}
	return &ipConfig, nil
}

// GetAPN gets the APN from the bearer properties
func (b *Bearer) GetAPN() (string, error) {
	properties := b.Properties()
	if !properties.HasApn {
		return "", errors.New("failed to read the APN")
	}
	return properties.apn, nil
}

// IsAPNType checks if the APN is of the type |apnType|
func (b *Bearer) IsAPNType(apnType mmconst.BearerAPNType) (bool, error) {
	properties := b.Properties()
	if !properties.HasApnType {
		return false, errors.New("failed to read the APN type")
	}
	return (properties.apnType & uint32(apnType)) != 0, nil
}

// IsIPType checks if the IP of the bearer is of the type |ipType|
func (b *Bearer) IsIPType(ipType mmconst.BearerIPFamily) (bool, error) {
	properties := b.Properties()
	if !properties.HasIPType {
		return false, errors.New("failed to read the IP type")
	}
	return (properties.ipType & uint32(ipType)) != 0, nil
}

// GetProfileID gets the profile ID from the bearer properties or
// InvalidProfileID if it is not present.
func (b *Bearer) GetProfileID() (int32, error) {
	properties := b.Properties()
	if !properties.HasProfileID {
		return InvalidProfileID, errors.New("failed to read the profile ID")
	}
	return properties.profileID, nil
}

// Interface gets the Interface value
func (b *Bearer) Interface() string {
	value, err := b.props.GetString(mmconst.BearerPropertyInterface)
	if err != nil {
		panic("missing interface property")
	}
	return value
}

// Connected gets the Connected value
func (b *Bearer) Connected() bool {
	value, err := b.props.GetBool(mmconst.BearerPropertyConnected)
	if err != nil {
		panic("missing connected property")
	}
	return value
}

// ConnectionError gets the ConnectionError value
func (b *Bearer) ConnectionError() []string {
	value, err := b.props.GetStructOfStrings(mmconst.BearerPropertyConnectionError)
	if err != nil {
		panic("missing ConnectionError property")
	}
	return value
}

// Multiplexed gets the Multiplexed value
func (b *Bearer) Multiplexed() bool {
	value, err := b.props.GetBool(mmconst.BearerPropertyMultiplexed)
	if err != nil {
		panic("missing multiplexed property")
	}
	return value
}

// IP4Config gets the IP4Config value
func (b *Bearer) IP4Config() *IPConfig {
	value, err := parseIPConfig(b.props, mmconst.BearerPropertyIP4Config)
	if err != nil {
		panic("failed to read Ip4Config")
	}
	return value
}

// IP6Config gets the IP6Config value
func (b *Bearer) IP6Config() *IPConfig {
	value, err := parseIPConfig(b.props, mmconst.BearerPropertyIP6Config)
	if err != nil {
		panic("failed to read Ip6Config")
	}
	return value
}

// Properties gets the Properties value
func (b *Bearer) Properties() BearerProperties {
	innerPropsGet, err := b.props.Get(mmconst.BearerPropertyProperties)
	if err != nil {
		panic("failed to read bearer properties")
	}
	innerProps, ok := innerPropsGet.(map[string]interface{})
	if !ok {
		panic("failed to parse bearer properties")
	}
	properties := BearerProperties{}
	// APN
	value, ok := innerProps[mmconst.BearerPropertyApn]
	if ok {
		properties.apn, ok = value.(string)
	}
	properties.HasApn = ok
	// AllowRoaming
	value, ok = innerProps[mmconst.BearerPropertyAllowRoaming]
	if ok {
		properties.allowRoaming, ok = value.(bool)
	}
	properties.HasAllowRoaming = ok
	// AllowedAuth
	value, ok = innerProps[mmconst.BearerPropertyAllowedAuth]
	if ok {
		properties.allowedAuth, ok = value.(uint32)
	}
	properties.HasAllowedAuth = ok
	// APN type
	value, ok = innerProps[mmconst.BearerPropertyApnType]
	if ok {
		properties.apnType, ok = value.(uint32)
	}
	properties.HasApnType = ok
	// IP type
	value, ok = innerProps[mmconst.BearerPropertyIPType]
	if ok {
		properties.ipType, ok = value.(uint32)
	}
	properties.HasIPType = ok
	// Multiplex
	value, ok = innerProps[mmconst.BearerPropertyMultiplex]
	if ok {
		properties.multiplex, ok = value.(uint32)
	}
	properties.HasMultiplex = ok
	// Password
	value, ok = innerProps[mmconst.BearerPropertyPassword]
	if ok {
		properties.password, ok = value.(string)
	}
	properties.HasPassword = ok
	// User
	value, ok = innerProps[mmconst.BearerPropertyUser]
	if ok {
		properties.user, ok = value.(string)
	}
	properties.HasUser = ok
	// Profile ID
	value, ok = innerProps[mmconst.BearerPropertyProfileID]
	if ok {
		properties.profileID, ok = value.(int32)
	}
	properties.HasProfileID = ok

	return properties
}
