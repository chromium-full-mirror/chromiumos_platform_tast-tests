// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package passpoint

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/certutil"
	"go.chromium.org/tast/core/errors"
)

const (
	testUser        = "test-user"
	testPassword    = "test-password"
	testPackageName = "app.passpoint.example.com"
)

// TestCerts is the set of certificates/private keys used for authentication
// during the tests.
var TestCerts = certificate.TestCert1()

// Auth is the authentication method the access point will expose.
type Auth int

const (
	// AuthTLS represents the EAP-TLS authentication method.
	AuthTLS Auth = iota
	// AuthTTLS represents the EAP-TTLS with MSCHAPv2 authentication method.
	AuthTTLS
)

// Credentials represents a set of Passpoint credentials with selection criteria.
type Credentials struct {
	// Domains represents the domains of the compatible service providers.
	// The first domain is the FQDN of the provider, the others (if any)
	// are the FQDNs of partner service providers.
	Domains []string
	// HomeOIs is a list of organisation identifiers (OI).
	HomeOIs []string
	// RequiredHomeOIs is a list of required organisation identifiers.
	RequiredHomeOIs []string
	// RoamingOIs is a list of roaming-compatible OIs.
	RoamingOIs []string
	// Auth is the EAP network authentication.
	Auth
	// CertID is the identifier of the client certificate in the store.
	CertID string
	// KeyID is the identifier of the client private key in the store.
	KeyID string
}

// FQDN returns the fully qualified domain name of the service provider.
// The first domain of the pc.Domains list is considered to be the FQDN
// that identifies the service provider (ARC follows the same logic).
func (pc *Credentials) FQDN() string {
	if len(pc.Domains) == 0 {
		return ""
	}
	return pc.Domains[0]
}

// OtherHomePartners returns the list compatible partners' domains.
func (pc *Credentials) OtherHomePartners() []string {
	if len(pc.Domains) < 2 {
		return []string{}
	}
	return pc.Domains[1:]
}

// ToShillProperties converts the set of credentials to a map for credentials D-Bus
// properties. ToShillProperties only supports EAP-TTLS authentication.
func (pc *Credentials) ToShillProperties() (map[string]interface{}, error) {
	props := map[string]interface{}{
		shillconst.PasspointCredentialsPropertyDomains:            pc.Domains,
		shillconst.PasspointCredentialsPropertyRealm:              pc.FQDN(),
		shillconst.PasspointCredentialsPropertyMeteredOverride:    false,
		shillconst.PasspointCredentialsPropertyAndroidPackageName: testPackageName,
		shillconst.ServicePropertyEAPCACertPEM:                    []string{TestCerts.CACred.Cert},
	}

	switch pc.Auth {
	case AuthTTLS:
		props[shillconst.ServicePropertyEAPMethod] = "TTLS"
		props[shillconst.ServicePropertyEAPInnerEAP] = "auth=MSCHAPV2"
		props[shillconst.ServicePropertyEAPIdentity] = testUser
		props[shillconst.ServicePropertyEAPPassword] = testPassword
	case AuthTLS:
		props[shillconst.ServicePropertyEAPMethod] = "TLS"
		props[shillconst.ServicePropertyEAPIdentity] = testUser
		props[shillconst.ServicePropertyEAPPin] = "000000"
		props[shillconst.ServicePropertyEAPCertID] = pc.CertID
		props[shillconst.ServicePropertyEAPKeyID] = pc.KeyID
	}

	for propName, ois := range map[string][]string{
		shillconst.PasspointCredentialsPropertyHomeOIs:          pc.HomeOIs,
		shillconst.PasspointCredentialsPropertyRequiredHomeOIs:  pc.RequiredHomeOIs,
		shillconst.PasspointCredentialsPropertyRoamingConsortia: pc.RoamingOIs,
	} {
		var propOIs []string
		for _, oi := range ois {
			u, err := strconv.ParseUint(oi, 16, 64)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse OI")
			}
			propOIs = append(propOIs, strconv.FormatUint(u, 10))
		}
		props[propName] = propOIs
	}

	return props, nil
}

// ToAndroidConfig converts the set of credentials to a base64 string of an Android-specific type application/x-wifi-config.
func (pc *Credentials) ToAndroidConfig(ctx context.Context) (string, error) {
	ppsMoProfile, err := pc.preparePPSMOProfile()
	if err != nil {
		return "", errors.Wrap(err, "failed to prepare OMA-DM PerProviderSubscription-MO XML profile")
	}

	params := map[string]string{
		"profile": base64.StdEncoding.EncodeToString([]byte(ppsMoProfile)),
		"caCert":  base64.StdEncoding.EncodeToString([]byte(TestCerts.CACred.Cert)),
	}

	if pc.Auth == AuthTLS {
		pkcs12Cert, err := certutil.PreparePKCS12Cert(ctx, TestCerts)
		if err != nil {
			return "", errors.Wrap(err, "failed to create PKCS#12 certificate")
		}
		params["clientCert"] = base64.StdEncoding.EncodeToString([]byte(pkcs12Cert))
	}

	tmpl, err := template.New("").Parse(`Content-Type: multipart/mixed; boundary={boundary}
Content-Transfer-Encoding: base64

--{boundary}
Content-Type: application/x-passpoint-profile
Content-Transfer-Encoding: base64

{{.profile}}
--{boundary}
Content-Type: application/x-x509-ca-cert
Content-Transfer-Encoding: base64

{{.caCert}}
{{- if .clientCert}}
--{boundary}
Content-Type: application/x-pkcs12
Content-Transfer-Encoding: base64

{{.clientCert}}
{{- end}}
--{boundary}--`)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse android config template")
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", errors.Wrap(err, "failed to execute android config template")
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// preparePPSMOProfile creates an OMA-DM PerProviderSubscription-MO XML profile based on the set of Passpoint credentials.
func (pc *Credentials) preparePPSMOProfile() (string, error) {
	homeSp, err := pc.preparePPSMOHomeSP()
	if err != nil {
		return "", errors.Wrap(err, "failed to prepare OMA-DM PerProviderSubscription Home SP node")
	}
	cred, err := pc.preparePPSMOCred()
	if err != nil {
		return "", errors.Wrap(err, "failed to prepare OMA-DM PerProviderSubscription-MO credential node")
	}
	return fmt.Sprintf(`<MgmtTree xmlns="syncml:dmddf1.2">
  <VerDTD>1.2</VerDTD>
  <Node>
    <NodeName>PerProviderSubscription</NodeName>
    <RTProperties>
      <Type>
        <DDFName>urn:wfa:mo:hotspot2dot0-perprovidersubscription:1.0</DDFName>
      </Type>
    </RTProperties>
    <Node>
      <NodeName>i001</NodeName>
%s
%s
    </Node>
  </Node>
</MgmtTree>`, homeSp, cred), nil
}

// preparePPSMOHomeSP creates an OMA-DM PerProviderSubscription-MO XML Home SP node based on the set of Passpoint credentials.
func (pc *Credentials) preparePPSMOHomeSP() (string, error) {
	type homeOI struct {
		Value    string
		Required string
	}
	var ois []homeOI
	for _, oi := range pc.HomeOIs {
		ois = append(ois, homeOI{
			Value:    oi,
			Required: "FALSE",
		})
	}
	for _, oi := range pc.RequiredHomeOIs {
		ois = append(ois, homeOI{
			Value:    oi,
			Required: "TRUE",
		})
	}
	var roamingOIs []string
	for _, oi := range pc.RoamingOIs {
		roamingOIs = append(roamingOIs, oi)
	}

	// Nodes are starting at index 1, we need a helper to fill the template.
	funcs := template.FuncMap{
		"inc": func(i int) int {
			return i + 1
		},
	}

	tmpl, err := template.New("").Funcs(funcs).Parse(`
      <Node>
        <NodeName>HomeSP</NodeName>
        <Node>
          <NodeName>FriendlyName</NodeName>
          <Value>Android Passpoint Config</Value>
        </Node>
        <Node>
          <NodeName>FQDN</NodeName>
          <Value>{{.FQDN}}</Value>
        </Node>
{{- if gt (len .HomeOIs) 0}}
        <Node>
          <NodeName>HomeOIList</NodeName>
{{- range $i, $oi := .HomeOIs}}
          <Node>
            <NodeName>h{{printf "%03d" (inc $i)}}</NodeName>
            <Node>
              <NodeName>HomeOI</NodeName>
              <Value>{{$oi.Value}}</Value>
            </Node>
            <Node>
              <NodeName>HomeOIRequired</NodeName>
              <Value>{{$oi.Required}}</Value>
            </Node>
          </Node>
{{- end}}
        </Node>
{{- end}}
{{- if gt (len .RoamingOIs) 0}}
        <Node>
          <NodeName>RoamingConsortiumOI</NodeName>
          <Value>{{.RoamingOIs}}</Value>
        </Node>
{{- end}}
{{- if gt (len .OtherHomePartners) 0}}
        <Node>
          <NodeName>OtherHomePartners</NodeName>
{{- range $i, $fqdn := .OtherHomePartners}}
          <Node>
            <NodeName>o{{printf "%03d" (inc $i)}}</NodeName>
            <Node>
              <NodeName>FQDN</NodeName>
              <Value>{{$fqdn}}</Value>
            </Node>
          </Node>
{{- end}}
        </Node>
{{- end}}
      </Node>
	`)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse PPS Home SP template")
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct {
		FQDN              string
		HomeOIs           []homeOI
		RoamingOIs        string
		OtherHomePartners []string
	}{
		FQDN:              pc.FQDN(),
		HomeOIs:           ois,
		RoamingOIs:        strings.Join(roamingOIs, ","),
		OtherHomePartners: pc.OtherHomePartners(),
	}); err != nil {
		return "", errors.Wrap(err, "failed to execute PPS Home SP template")
	}
	return buf.String(), nil
}

// preparePPSMOCred creates an OMA-DM PerProviderSubscription-MO XML Credential node based on the set of Passpoint credentials.
func (pc *Credentials) preparePPSMOCred() (string, error) {
	params := map[string]string{
		"realm": pc.FQDN(),
	}
	switch pc.Auth {
	case AuthTLS:
		fingerprint, err := certutil.PrepareCertSHA256Fingerprint(TestCerts)
		if err != nil {
			return "", errors.Wrap(err, "failed to get certificate's fingerprint")
		}
		params["fingerprint"] = fingerprint
	case AuthTTLS:
		params["user"] = testUser
		params["password"] = base64.StdEncoding.EncodeToString([]byte(testPassword))
	default:
		return "", errors.Errorf("unsupported authentication method: %v", pc.Auth)
	}

	tmpl, err := template.New("").Parse(`
      <Node>
        <NodeName>Credential</NodeName>
        <Node>
          <NodeName>Realm</NodeName>
          <Value>{{.realm}}</Value>
        </Node>
{{if .fingerprint}}
        <Node>
          <NodeName>DigitalCertificate</NodeName>
          <Node>
            <NodeName>CertificateType</NodeName>
            <Value>x509v3</Value>
          </Node>
          <Node>
            <NodeName>CertSHA256Fingerprint</NodeName>
            <Value>{{.fingerprint}}</Value>
          </Node>
        </Node>
{{else if and .user .password}}
        <Node>
          <NodeName>UsernamePassword</NodeName>
          <Node>
            <NodeName>MachineManaged</NodeName>
            <Value>true</Value>
          </Node>
          <Node>
            <NodeName>Username</NodeName>
            <Value>{{.user}}</Value>
          </Node>
          <Node>
            <NodeName>Password</NodeName>
            <Value>{{.password}}</Value>
          </Node>
          <Node>
            <NodeName>EAPMethod</NodeName>
            <Node>
              <NodeName>EAPType</NodeName>
              <Value>21</Value>
            </Node>
            <Node>
              <NodeName>InnerMethod</NodeName>
              <Value>MS-CHAP-V2</Value>
            </Node>
          </Node>
        </Node>
{{end}}
      </Node>`)

	if err != nil {
		return "", errors.Wrap(err, "failed to parse credentials template")
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", errors.Wrap(err, "failed to execute credentials template")
	}
	return buf.String(), nil
}
