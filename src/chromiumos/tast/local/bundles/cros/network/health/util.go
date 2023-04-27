// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"chromiumos/tast/common/shillconst"
	"go.chromium.org/tast/core/errors"
)

// NetworkTypeFromShillType returns an analogous NetworkType.
func NetworkTypeFromShillType(sType string) (NetworkType, error) {
	if sType == shillconst.TypeEthernet {
		return EthernetNT, nil
	} else if sType == shillconst.TypeWifi {
		return WiFiNT, nil
	} else if sType == shillconst.TypeCellular {
		return CellularNT, nil
	} else if sType == shillconst.TypeVPN {
		return VPNNT, nil
	}

	return EthernetNT, errors.New("unknown shill type")
}

// FindMatchingNetwork searches the given list of networks to find one matching
// the shill type (sType) and guid (for non-Ethernet networks).
func FindMatchingNetwork(networks []Network, sType, guid string) (*Network, error) {
	networkType, err := NetworkTypeFromShillType(sType)
	if err != nil {
		return nil, err
	}
	for _, n := range networks {
		if networkType != n.Type {
			continue
		}
		if n.Type == EthernetNT || guid == n.GUID {
			return &n, nil
		}
	}
	return nil, errors.New("failed to find a connected network in Network Health")
}
