// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast/core/testing"
)

// Variable names of certificates for tests.
const (
	caCertVarName                            = "network.ca_certificate_credential"
	caKeyVarName                             = "network.ca_certificate_private_key"
	caCertCommonNameVarName                  = "network.ca_certificate_common_name"
	caCertOrganizationNameVarName            = "network.ca_certificate_organization_name"
	serverCertVarName                        = "network.server_certificate_credential"
	serverKeyVarName                         = "network.server_certificate_private_key"
	serverCertCommonNameVarName              = "network.server_certificate_common_name"
	serverCertOrganizationNameVarName        = "network.server_certificate_organization_name"
	clientCertVarName                        = "network.client_certificate_credential"
	clientKeyVarName                         = "network.client_certificate_private_key"
	clientCertCommonNameVarName              = "network.client_certificate_common_name"
	clientCertOrganizationNameVarName        = "network.client_certificate_organization_name"
	expiredServerCertVarName                 = "network.expired_server_certificate_credential"
	expiredServerKeyVarName                  = "network.expired_server_certificate_private_key"
	expiredServerCertCommonNameVarName       = "network.expired_server_certificate_common_name"
	expiredServerCertOrganizationNameVarName = "network.expired_server_certificate_organization_name"
)

var (
	// CaCertificateVariables are the variables required to load the CA certificate by calling LoadCertFromVars().
	CaCertificateVariables = []string{
		caCertVarName,
		caKeyVarName,
		caCertCommonNameVarName,
		caCertOrganizationNameVarName,
	}

	// ServerCertificateVariables are the variables required to load the Server certificate by calling LoadCertFromVars().
	ServerCertificateVariables = []string{
		serverCertVarName,
		serverKeyVarName,
		serverCertCommonNameVarName,
		serverCertOrganizationNameVarName,
	}

	// ClientCertificateVariables are the variables required to load the Client certificate by calling LoadCertFromVars().
	ClientCertificateVariables = []string{
		clientCertVarName,
		clientKeyVarName,
		clientCertCommonNameVarName,
		clientCertOrganizationNameVarName,
	}

	// ExpiredCertificateVariables are the variables required to load the Expired Server certificate by calling LoadCertFromVars().
	ExpiredCertificateVariables = []string{
		expiredServerCertVarName,
		expiredServerKeyVarName,
		expiredServerCertCommonNameVarName,
		expiredServerCertOrganizationNameVarName,
	}

	caCert = testing.RegisterVarString(
		caCertVarName,
		"",
		"CA certificate",
	)
	caKey = testing.RegisterVarString(
		caKeyVarName,
		"",
		"Private key used for issuing the CA certificate",
	)
	caCertCommonName = testing.RegisterVarString(
		caCertCommonNameVarName,
		"",
		"Common name of the CA certificate",
	)
	caCertOrganizationName = testing.RegisterVarString(
		caCertOrganizationNameVarName,
		"",
		"Organization name of the CA certificate",
	)
	serverCert = testing.RegisterVarString(
		serverCertVarName,
		"",
		"Server certificate",
	)
	serverKey = testing.RegisterVarString(
		serverKeyVarName,
		"",
		"Private key used for issuing the server certificate",
	)
	serverCertCommonName = testing.RegisterVarString(
		serverCertCommonNameVarName,
		"",
		"Common name of the server certificate",
	)
	serverCertOrganizationName = testing.RegisterVarString(
		serverCertOrganizationNameVarName,
		"",
		"Organization name of the server certificate",
	)
	clientCert = testing.RegisterVarString(
		clientCertVarName,
		"",
		"Client certificate",
	)
	clientKey = testing.RegisterVarString(
		clientKeyVarName,
		"",
		"Private key used for issuing the client certificate",
	)
	clientCertCommonName = testing.RegisterVarString(
		clientCertCommonNameVarName,
		"",
		"Common name of the client certificate",
	)
	clientCertOrganizationName = testing.RegisterVarString(
		clientCertOrganizationNameVarName,
		"",
		"Organization name of the client certificate",
	)
	expiredServerCert = testing.RegisterVarString(
		expiredServerCertVarName,
		"",
		"Expired server certificate",
	)
	expiredServerKey = testing.RegisterVarString(
		expiredServerKeyVarName,
		"",
		"Private key used for issuing the expired server certificate",
	)
	expiredServerCertCommonName = testing.RegisterVarString(
		expiredServerCertCommonNameVarName,
		"",
		"Common name of the expired server certificate",
	)
	expiredServerCertOrganizationName = testing.RegisterVarString(
		expiredServerCertOrganizationNameVarName,
		"",
		"Organization name of the expired server certificate",
	)
)

// LoadCertsFromVars returns a certificate.CertStore that contains a set of certificates based on runtime variables.
func LoadCertsFromVars() certificate.CertStore {
	return certificate.CertStore{
		CACred: certificate.Credential{
			Cert:       caCert.Value(),
			PrivateKey: caKey.Value(),
			Info: certificate.Info{
				CommonName:   caCertCommonName.Value(),
				Organization: caCertOrganizationName.Value(),
			},
		},
		ServerCred: certificate.Credential{
			Cert:       serverCert.Value(),
			PrivateKey: serverKey.Value(),
			Info: certificate.Info{
				CommonName:   serverCertCommonName.Value(),
				Organization: serverCertOrganizationName.Value(),
			},
		},
		ClientCred: certificate.Credential{
			Cert:       clientCert.Value(),
			PrivateKey: clientKey.Value(),
			Info: certificate.Info{
				CommonName:   clientCertCommonName.Value(),
				Organization: clientCertOrganizationName.Value(),
			},
		},
		ExpiredServerCred: certificate.Credential{
			Cert:       expiredServerCert.Value(),
			PrivateKey: expiredServerKey.Value(),
			Info: certificate.Info{
				CommonName:   expiredServerCertCommonName.Value(),
				Organization: expiredServerCertOrganizationName.Value(),
			},
		},
	}
}
