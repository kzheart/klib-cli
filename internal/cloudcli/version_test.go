package cloudcli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestVersionWithoutCredentials(t *testing.T) {
	for _, command := range []string{"version", "--version"} {
		var out, errOut bytes.Buffer
		if code := Run(context.Background(), []string{command}, nil, &out, &errOut, filepath.Join(t.TempDir(), "missing", "credentials.json")); code != 0 {
			t.Fatalf("version requires credentials: %d %s", code, errOut.String())
		}
		var result struct {
			Version  string `json:"version"`
			Commit   string `json:"commit"`
			Protocol int    `json:"protocol_version"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Version != Version || result.Commit != Commit || result.Protocol != 5 {
			t.Fatalf("invalid version: %s", out.String())
		}
	}
}
