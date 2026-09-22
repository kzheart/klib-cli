package cloudcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConnectWritesPrivateConfigurationWithoutCredentialsOnStdout(t *testing.T) {
	id := "dep_" + strings.Repeat("A", 32)
	token := "enr_" + strings.Repeat("A", 43)
	issued := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Error("missing management identity")
		}
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "active", "revision": 7, "products": []any{map[string]string{"product_id": "demo.product"}}})
			return
		}
		issued++
		var body struct {
			Revision int64 `json:"revision"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Revision != 7 {
			t.Error("connection did not bind observed revision")
		}
		json.NewEncoder(w).Encode(map[string]any{"deployment_id": id, "revision": 7, "token": token, "expires_at": time.Now().Add(time.Minute)})
	}))
	defer server.Close()
	dir := t.TempDir()
	credentials := filepath.Join(dir, "credentials.json")
	data, _ := json.Marshal(Credentials{Endpoint: server.URL, APIKey: testKey})
	if err := WritePrivate(credentials, data); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "config.yml")
	args := []string{"instances", "connect", "--deployment", id, "--output", output}
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), args, strings.NewReader(""), &stdout, &stderr, credentials); code != 0 {
		t.Fatalf("connect: %d %s", code, stderr.String())
	}
	content, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(content, []byte(token)) || !bytes.Contains(content, []byte("demo.product")) || bytes.Contains(content, []byte(testKey)) {
		t.Fatalf("configuration content invalid: %v", err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe config permissions: %v", err)
	}
	if strings.Contains(stdout.String()+stderr.String(), token) || strings.Contains(stdout.String()+stderr.String(), testKey) {
		t.Fatal("credentials disclosed")
	}
	if code := Run(context.Background(), args, strings.NewReader(""), &stdout, &stderr, credentials); code != 2 || issued != 1 {
		t.Fatalf("existing config issued another credential: exit %d count %d", code, issued)
	}
	secondOutput := filepath.Join(dir, "second.yml")
	if code := Run(context.Background(), []string{"instances", "connect", "--deployment", id, "--output", secondOutput}, strings.NewReader(""), &stdout, &stderr, credentials); code != 0 {
		t.Fatalf("reconnect: %d %s", code, stderr.String())
	}
	second, _ := os.ReadFile(secondOutput)
	connectionLine := func(content []byte) string {
		for _, line := range strings.Split(string(content), "\n") {
			if strings.HasPrefix(line, "connection:") {
				return line
			}
		}
		return ""
	}
	if connectionLine(content) == "" || connectionLine(content) == connectionLine(second) {
		t.Fatal("reconnection reused local identity namespace")
	}
	after, _ := os.ReadFile(output)
	if !bytes.Equal(after, content) {
		t.Fatal("existing config was changed")
	}
}

func TestPrivateNewNeverReplacesFileOrSymlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := WritePrivateNew(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateNew(path, []byte("second")); err == nil {
		t.Fatal("overwrote existing file")
	}
	link := path + ".link"
	if err := os.Symlink(path, link); err != nil {
		t.Skip(err)
	}
	if err := WritePrivateNew(link, []byte("third")); err == nil {
		t.Fatal("followed symbolic link")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "first" {
		t.Fatal("modified original file")
	}
}

func TestTransportFailuresUseServerExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	raw, _ := json.Marshal(Credentials{Endpoint: endpoint, APIKey: testKey})
	if err := WritePrivate(path, raw); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"account"}, strings.NewReader(""), &out, &errOut, path); code != 5 {
		t.Fatalf("transport failure classified as %d", code)
	}
}
