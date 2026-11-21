// Copyright The Prometheus Authors
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
	"bytes"
	"encoding/binary"
	"log/slog"
	"testing"

	"github.com/facebook/time/ntp/chrony"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// fakeChronyConn answers every request with a fixed reply.
type fakeChronyConn struct {
	reply []byte
}

func (c *fakeChronyConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *fakeChronyConn) Read(p []byte) (int, error) { return copy(p, c.reply), nil }

// trackingReply encodes a minimal 'tracking' reply as sent by chronyd, with
// all values zero except the leap status.
func trackingReply(t *testing.T, leapStatus uint16) []byte {
	t.Helper()
	reply := struct {
		Head chrony.ReplyHead
		// Mirrors chrony's RPY_Tracking.
		RefID      uint32
		IPAddr     chrony.IPAddr
		Stratum    uint16
		LeapStatus uint16
		RefTime    [3]uint32
		Floats     [10]int32
	}{
		Head: chrony.ReplyHead{
			Version: 6,
			PKTType: 2, // PKT_TYPE_CMD_REPLY
			Reply:   chrony.RpyTracking,
		},
		LeapStatus: leapStatus,
	}
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, reply); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTrackingLeapStatus(t *testing.T) {
	// Values from chrony's NTP_Leap enum (ntp.h).
	for _, leap := range []uint16{0, 1, 2, 3} {
		client := &chrony.Client{Connection: &fakeChronyConn{reply: trackingReply(t, leap)}}
		e := Exporter{logger: slog.New(slog.DiscardHandler)}

		ch := make(chan prometheus.Metric, 32)
		if err := e.getTrackingMetrics(e.logger, ch, client); err != nil {
			t.Fatalf("leap %d: getTrackingMetrics: %v", leap, err)
		}
		close(ch)

		got := map[string]float64{}
		for m := range ch {
			if m.Desc() != trackingLeapStatus.desc {
				continue
			}
			var pb dto.Metric
			if err := m.Write(&pb); err != nil {
				t.Fatal(err)
			}
			got[pb.GetLabel()[0].GetValue()] = pb.GetGauge().GetValue()
		}
		if len(got) != len(leapStatusNames) {
			t.Fatalf("leap %d: got %d leap_status series, want %d", leap, len(got), len(leapStatusNames))
		}
		for i, state := range leapStatusNames {
			want := 0.0
			if uint16(i) == leap {
				want = 1.0
			}
			if got[state] != want {
				t.Errorf("leap %d: leap_status{leap_status=%q} = %v, want %v", leap, state, got[state], want)
			}
		}
	}
}
