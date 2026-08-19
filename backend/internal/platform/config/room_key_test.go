package config

import (
	"encoding/base64"
	"testing"
)

func TestLoadSessionRoomKeyRequiresStrongBase64Key(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	got, err := loadSessionRoomKey(func(string) string { return base64.StdEncoding.EncodeToString(key) })
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(key) {
		t.Fatalf("decoded key mismatch")
	}
}

func TestLoadSessionRoomKeyRejectsMissingOrWeakKey(t *testing.T) {
	for name, raw := range map[string]string{"missing": "", "weak": base64.StdEncoding.EncodeToString([]byte("short")), "invalid": "%%%"} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadSessionRoomKey(func(string) string { return raw }); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}
