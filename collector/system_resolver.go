// Copyright 2022 Ben Kochie
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"errors"
	"net"
)

// systemReverseLookup wraps net.LookupAddr, formatted via formatNames so
// its names are directly comparable to reverseResolver's (for namesAgree).
// It uses the system's real resolution order (/etc/hosts, DNS, mDNS, ...).
func systemReverseLookup(address string) (string, error) {
	names, err := net.LookupAddr(address)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", errors.New("no names returned")
	}
	return formatNames(names), nil
}
