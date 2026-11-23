// Copyright Ben Kochie
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
	"strings"
	"testing"

	"github.com/facebook/time/ntp/chrony"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type authdataTestCollector struct {
	a *chrony.AuthData
}

func (c authdataTestCollector) Describe(_ chan<- *prometheus.Desc) {}

func (c authdataTestCollector) Collect(ch chan<- prometheus.Metric) {
	emitAuthdataMetrics(ch, "192.0.2.1", "ntp.example.org", c.a)
}

func TestAuthdataMetrics(t *testing.T) {
	tests := []struct {
		name string
		a    chrony.AuthData
		want string
	}{
		{
			name: "nts",
			a: chrony.AuthData{
				Mode: chrony.AuthModeNTS, KeyID: 1, KeyType: 15, KeyLength: 256,
				LastKEAgo: 123387, Cookies: 8, CookieLength: 100,
			},
			want: `
# HELP chrony_authdata_cookie_length_bytes Chrony source length of the next NTS cookie in bytes
# TYPE chrony_authdata_cookie_length_bytes gauge
chrony_authdata_cookie_length_bytes{source_address="192.0.2.1",source_name="ntp.example.org"} 100
# HELP chrony_authdata_cookies Chrony source number of NTS cookies held
# TYPE chrony_authdata_cookies gauge
chrony_authdata_cookies{source_address="192.0.2.1",source_name="ntp.example.org"} 8
# HELP chrony_authdata_info Chrony source authentication info, mode is none, symmetric or nts
# TYPE chrony_authdata_info gauge
chrony_authdata_info{key_id="1",key_type="15",mode="nts",source_address="192.0.2.1",source_name="ntp.example.org"} 1
# HELP chrony_authdata_ke_attempts Chrony source NTS-KE attempts since the last successful key exchange
# TYPE chrony_authdata_ke_attempts gauge
chrony_authdata_ke_attempts{source_address="192.0.2.1",source_name="ntp.example.org"} 0
# HELP chrony_authdata_key_length_bits Chrony source authentication key length in bits
# TYPE chrony_authdata_key_length_bits gauge
chrony_authdata_key_length_bits{source_address="192.0.2.1",source_name="ntp.example.org"} 256
# HELP chrony_authdata_last_ke_age_seconds Chrony source time since the last successful NTS-KE in seconds
# TYPE chrony_authdata_last_ke_age_seconds gauge
chrony_authdata_last_ke_age_seconds{source_address="192.0.2.1",source_name="ntp.example.org"} 123387
# HELP chrony_authdata_nak Chrony source whether the last NTS response was a NAK
# TYPE chrony_authdata_nak gauge
chrony_authdata_nak{source_address="192.0.2.1",source_name="ntp.example.org"} 0
`,
		},
		{
			// no successful NTS-KE yet: no last_ke_age sample
			name: "nts without key exchange",
			a: chrony.AuthData{
				Mode: chrony.AuthModeNTS, KEAttempts: 3,
				LastKEAgo: chrony.AuthDataNoKeyExchange, NAK: true,
			},
			want: `
# HELP chrony_authdata_cookie_length_bytes Chrony source length of the next NTS cookie in bytes
# TYPE chrony_authdata_cookie_length_bytes gauge
chrony_authdata_cookie_length_bytes{source_address="192.0.2.1",source_name="ntp.example.org"} 0
# HELP chrony_authdata_cookies Chrony source number of NTS cookies held
# TYPE chrony_authdata_cookies gauge
chrony_authdata_cookies{source_address="192.0.2.1",source_name="ntp.example.org"} 0
# HELP chrony_authdata_info Chrony source authentication info, mode is none, symmetric or nts
# TYPE chrony_authdata_info gauge
chrony_authdata_info{key_id="0",key_type="0",mode="nts",source_address="192.0.2.1",source_name="ntp.example.org"} 1
# HELP chrony_authdata_ke_attempts Chrony source NTS-KE attempts since the last successful key exchange
# TYPE chrony_authdata_ke_attempts gauge
chrony_authdata_ke_attempts{source_address="192.0.2.1",source_name="ntp.example.org"} 3
# HELP chrony_authdata_key_length_bits Chrony source authentication key length in bits
# TYPE chrony_authdata_key_length_bits gauge
chrony_authdata_key_length_bits{source_address="192.0.2.1",source_name="ntp.example.org"} 0
# HELP chrony_authdata_nak Chrony source whether the last NTS response was a NAK
# TYPE chrony_authdata_nak gauge
chrony_authdata_nak{source_address="192.0.2.1",source_name="ntp.example.org"} 1
`,
		},
		{
			name: "symmetric",
			a: chrony.AuthData{
				Mode: chrony.AuthModeSymmetric, KeyID: 10, KeyType: 2, KeyLength: 160,
				LastKEAgo: chrony.AuthDataNoKeyExchange,
			},
			want: `
# HELP chrony_authdata_info Chrony source authentication info, mode is none, symmetric or nts
# TYPE chrony_authdata_info gauge
chrony_authdata_info{key_id="10",key_type="2",mode="symmetric",source_address="192.0.2.1",source_name="ntp.example.org"} 1
# HELP chrony_authdata_key_length_bits Chrony source authentication key length in bits
# TYPE chrony_authdata_key_length_bits gauge
chrony_authdata_key_length_bits{source_address="192.0.2.1",source_name="ntp.example.org"} 160
`,
		},
		{
			name: "none",
			a:    chrony.AuthData{Mode: chrony.AuthModeNone, LastKEAgo: chrony.AuthDataNoKeyExchange},
			want: `
# HELP chrony_authdata_info Chrony source authentication info, mode is none, symmetric or nts
# TYPE chrony_authdata_info gauge
chrony_authdata_info{key_id="0",key_type="0",mode="none",source_address="192.0.2.1",source_name="ntp.example.org"} 1
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := testutil.CollectAndCompare(authdataTestCollector{a: &tt.a}, strings.NewReader(tt.want)); err != nil {
				t.Error(err)
			}
		})
	}
}
