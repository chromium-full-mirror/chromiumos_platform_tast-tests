// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tape

import (
	"context"
	"os"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// ServiceAccountVar holds the name of the variable which stores the service account credentials for TAPE.
const ServiceAccountVar = "tape.service_account_key"

// ServiceAccount1 holds the location of service account credentials.
const ServiceAccount1 = "tape.service_account1"

// ServiceAccount2 holds the location of service account credentials.
const ServiceAccount2 = "tape.service_account2"

type clientOption struct {
	client    *client
	credsJSON []byte
}

// ClientOption provides options for getting a client for an account manager.
type ClientOption func(*clientOption)

// WithCredsJSON provides the option to get a client from service account credentials.
func WithCredsJSON(jsonData []byte) ClientOption {
	return func(opt *clientOption) {
		opt.credsJSON = jsonData
	}
}

// WithClient provides the option to use an existing client.
func WithClient(client *client) ClientOption {
	return func(opt *clientOption) {
		opt.client = client
	}
}

// GenericAccountManager holds the client and the generic accounts data.
type GenericAccountManager struct {
	client   *client
	Accounts []*GenericAccount
}

// NewGenericAccountManager leases a generic account, stores it in a GenericAccountManager struct and returns both. It requires
// a credsJSON byte array with the credentials of a service account to create a tape client for the GenericAccountManager.
func NewGenericAccountManager(ctx context.Context, credsJSON []byte, opts ...RequestAccountOption) (*GenericAccountManager, *GenericAccount, error) {
	client, err := NewClient(ctx, credsJSON)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create tape client")
	}

	return NewGenericAccountManagerFromClient(ctx, client, opts...)
}

// NewGenericAccountManagerFromClient leases a generic account, stores it in a GenericAccountManager struct and returns both. It requires
// a tape client to assign it to the GenericAccountManager.
func NewGenericAccountManagerFromClient(ctx context.Context, client *client, opts ...RequestAccountOption) (*GenericAccountManager, *GenericAccount, error) {
	manager := &GenericAccountManager{
		client: client,
	}

	account, err := manager.RequestAccount(ctx, opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to request account")
	}
	return manager, account, nil
}

// RequestAccount leases a generic account, stores it in the GenericAccountManager and returns it.
func (ah *GenericAccountManager) RequestAccount(ctx context.Context, opts ...RequestAccountOption) (*GenericAccount, error) {
	account, err := ah.client.RequestGenericAccount(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request owned test account")
	}

	ah.Accounts = append(ah.Accounts, account)

	return account, nil
}

// CleanUp releases all generic accounts that are stored in the GenericAccountManager.
func (ah *GenericAccountManager) CleanUp(ctx context.Context) error {
	var combinedErrors error
	for _, account := range ah.Accounts {
		err := ah.client.ReleaseGenericAccount(ctx, account)
		if err != nil {
			combinedErrors = errors.Wrap(combinedErrors, err.Error())
		}
	}

	if combinedErrors != nil {
		return errors.Wrap(combinedErrors, "failed to release some accounts")
	}

	return nil
}

// OwnedTestAccountManager holds the client and the Owned accounts data.
type OwnedTestAccountManager struct {
	client   *client
	Accounts []*OwnedTestAccount
}

// NewOwnedTestAccountManager leases an owned test account, stores it in an OwnedTestAccountManager struct and returns both. It requires
// a credsJSON byte array with the credentials of a service account to create a tape client for the OwnedTestAccountManager.
func NewOwnedTestAccountManager(ctx context.Context, credsJSON []byte, lock bool, opts ...RequestAccountOption) (*OwnedTestAccountManager, *OwnedTestAccount, error) {
	client, err := NewClient(ctx, credsJSON)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create tape client")
	}

	return NewOwnedTestAccountManagerFromClient(ctx, client, lock, opts...)
}

// NewOwnedTestAccountManagerFromClient leases an owned test account, stores it in an OwnedTestAccountManager struct and returns both.
// It requires a tape client to assign it to the OwnedTestAccountManager.
func NewOwnedTestAccountManagerFromClient(ctx context.Context, client *client, lock bool, opts ...RequestAccountOption) (*OwnedTestAccountManager, *OwnedTestAccount, error) {
	manager := &OwnedTestAccountManager{
		client: client,
	}

	account, err := manager.RequestAccount(ctx, lock, opts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to request account")
	}
	return manager, account, nil
}

// RequestAccount leases an owned test account, stores it in the OwnedTestAccountManager and returns it.
func (ah *OwnedTestAccountManager) RequestAccount(ctx context.Context, lock bool, opts ...RequestAccountOption) (*OwnedTestAccount, error) {
	account, err := ah.client.RequestOwnedTestAccount(ctx, lock, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request owned test account")
	}

	ah.Accounts = append(ah.Accounts, account)

	return account, nil
}

// CleanUp releases all owned test accounts that are stored in the OwnedTestAccountManager.
func (ah *OwnedTestAccountManager) CleanUp(ctx context.Context) error {
	var combinedErrors error
	for _, account := range ah.Accounts {
		err := ah.client.ReleaseOwnedTestAccount(ctx, account)
		if err != nil {
			combinedErrors = errors.Wrap(combinedErrors, err.Error())
		}
	}

	if combinedErrors != nil {
		return errors.Wrap(combinedErrors, "failed to release some accounts")
	}

	return nil
}

func (c *client) deprovisionUsingDeviceAndCustomerID(ctx context.Context, policyClient pspb.PolicyServiceClient) error {
	deviceAndCustomerIDResponse, err := policyClient.DeviceAndCustomerID(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get device and customer id")
	}
	return c.DeprovisionAndVerify(ctx, WithDeviceAndCustomerID(deviceAndCustomerIDResponse.DeviceID, deviceAndCustomerIDResponse.CustomerID))
}

func (c *client) deprovisionUsingStableDeviceSecret(ctx context.Context, policyClient pspb.PolicyServiceClient) error {
	stableDeviceSecretResponse, err := policyClient.StableDeviceSecret(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get stable device secret")
	}
	return c.DeprovisionAndVerify(ctx, WithStableDeviceSecret(stableDeviceSecretResponse.StableDeviceSecret))
}

// DeprovisionHelper is a helper function to deprovision a device in a managed domain.
func (c *client) DeprovisionHelper(ctx context.Context, rpcClient *rpc.Client, orgUnitPath string) error {
	policyClient := pspb.NewPolicyServiceClient(rpcClient.Conn)

	if errDci := c.deprovisionUsingDeviceAndCustomerID(ctx, policyClient); errDci != nil {
		if errSds := c.deprovisionUsingStableDeviceSecret(ctx, policyClient); errSds != nil {
			return errors.Wrap(errors.Join(errDci, errSds), "failed to deprovision using device/customer ids OR stable device secret")
		}
	}

	return nil
}

// DeprovisionAndVerify is a helper function to deprovision a device in a managed domain.
func (c *client) DeprovisionAndVerify(ctx context.Context, opt DeprovisionOption) error {
	if ProvidedAccount.Value() != "" {
		return nil
	}

	err := c.Deprovision(ctx, opt)
	if err != nil {
		return errors.Wrap(err, "failed to deprovision device")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isDeprovisioned, err := c.Deprovisioned(ctx, opt)
		if err != nil {
			return err
		}
		if !isDeprovisioned {
			return errors.New("device was not deprovisioned yet")
		}
		return nil
	}, &testing.PollOptions{
		Interval: 10 * time.Second,
	}); err != nil {
		return err
	}
	return nil
}

// MoveDeviceToOU is a helper function to move a device to an OU.
func (c *client) MoveDeviceToOU(ctx context.Context, rpcClient *rpc.Client, orgUnitPath string) error {
	if ProvidedAccount.Value() != "" {
		return nil
	}

	policyClient := pspb.NewPolicyServiceClient(rpcClient.Conn)
	deviceAndCustomerIDResponse, err := policyClient.DeviceAndCustomerID(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get device and customer id")
	}
	deviceID := deviceAndCustomerIDResponse.DeviceID
	customerID := deviceAndCustomerIDResponse.CustomerID

	_, err = c.MoveDevicesToOU(ctx, []string{deviceID}, orgUnitPath, customerID)
	if err != nil {
		return errors.Wrapf(err, "failed to move device %s to %s", deviceID, orgUnitPath)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isDeprovisioned, err := c.Deprovisioned(ctx, WithDeviceAndCustomerID(deviceID, customerID))
		if err != nil {
			return err
		}
		if isDeprovisioned {
			return errors.New("device not found in OU")
		}
		return nil
	}, &testing.PollOptions{
		Interval: 5 * time.Second,
	}); err != nil {
		return err
	}

	return nil
}

// Additional target keys for policies.

// NetworkKey is an additionalTargetKey for network related policies. It takes
// a network_id to identify the network that is being modified with the policy.
type NetworkKey struct {
	NetworkID string `json:"network_id"`
}

// AppKey is an additionalTargetKey for ArcPolicy related policies.
type AppKey struct {
	AppID string `json:"app_id"`
}

// GetClientFromLocalCredentials looks for service account credentials on the host
// and creates a client from them and returns it.
func GetClientFromLocalCredentials(ctx context.Context, paths []string) (*client, error) {
	if _, _, err := parseProvidedAccount(); err == nil {
		return &client{}, nil
	}
	if TapeToken.Value() != "" {
		return getClientFromToken(ctx, []byte(TapeToken.Value()))
	}

	var creds []byte = nil
	var err error = nil
	for _, path := range paths {
		if _, err = os.Stat(path); err == nil {
			creds, err = os.ReadFile(path)
			if err == nil {
				break
			}
		}
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to read service account file. If you are executing the test locally you have to pass an authentication token to tast, see go/tape-tast-doc#local-execution")
	}

	return NewClient(ctx, creds)
}
