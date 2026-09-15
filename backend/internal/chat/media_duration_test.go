package chat

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestParsedAudioDurationUsesPacketTiming(t *testing.T) {
	for _, tc := range []struct {
		name, metadata string
		start, end     string
		want           int
	}{
		{"understated metadata", "1", "0", "301", MaxVoiceDurationSeconds + 1},
		{"metadata longer", "20", "0", "1", 20},
		{"nonzero timestamp origin", "2", "100", "101", 2},
		{"missing metadata uses complete packets", "", "0", "1", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := probeResult{}
			result.Format.Duration = tc.metadata
			result.Packets = []probedPacket{{PTS: tc.start, Duration: "1"}, {PTS: tc.end, Duration: "1"}}
			got, err := parsedAudioDuration(result)
			if err != nil || got != tc.want {
				t.Fatalf("duration=%d err=%v want=%d", got, err, tc.want)
			}
		})
	}
}

func TestParserOutputBufferDrainsOverflow(t *testing.T) {
	output := &parserOutputBuffer{}
	data := make([]byte, maxParserOutputBytes+42)
	n, err := io.Copy(output, bytes.NewReader(data))
	if err != nil || n != int64(len(data)) || !output.exceeded || output.Len() != maxParserOutputBytes {
		t.Fatalf("write=%d err=%v exceeded=%v retained=%d", n, err, output.exceeded, output.Len())
	}
}

func TestParsedAudioDurationRejectsInvalidTimelines(t *testing.T) {
	for _, packet := range []probedPacket{{PTS: "NaN", Duration: "1"}, {PTS: "0", Duration: "-1"}, {PTS: "0", Duration: "N/A"}} {
		result := probeResult{}
		result.Format.Duration = "2"
		result.Packets = []probedPacket{packet}
		if _, err := parsedAudioDuration(result); !errors.Is(err, ErrMalformedMedia) {
			t.Fatalf("invalid timeline accepted: %#v err=%v", packet, err)
		}
	}
}
