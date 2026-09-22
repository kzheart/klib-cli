package cloudcli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testKey = "sk_test_never_a_real_credential"

func TestLoginAndEnrollmentNeverPrintSecrets(t *testing.T) {
	token := "enr_private_test_value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Error("missing bearer")
		}
		switch r.URL.Path {
		case "/developer/cloud/account":
			io.WriteString(w, `{"account_id":"acc_fixture"}`)
		case "/developer/cloud/deployments/dep_fixture/enrollments":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "reg_fixture", "token": token, "deployment_id": "dep_fixture", "revision": 1})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	credentials := filepath.Join(dir, "credentials.json")
	output := filepath.Join(dir, "enrollment.json")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"auth", "login", "--endpoint", server.URL, "--key-stdin"}, strings.NewReader(testKey), &stdout, &stderr, credentials)
	if code != 0 {
		t.Fatalf("login: %d %s", code, stderr.String())
	}
	info, err := os.Stat(credentials)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("credentials permissions: %v %v", info, err)
	}
	code = Run(context.Background(), []string{"enrollments", "issue", "--deployment", "dep_fixture", "--revision", "1", "--output", output}, strings.NewReader(""), &stdout, &stderr, credentials)
	if code != 0 {
		t.Fatalf("enrollment: %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), testKey) || strings.Contains(stdout.String()+stderr.String(), token) {
		t.Fatal("secret printed")
	}
	raw, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(raw, []byte(token)) {
		t.Fatal("private output missing token", err)
	}
	info, err = os.Stat(output)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("enrollment file permissions", err)
	}
	code = Run(context.Background(), []string{"enrollments", "issue", "--deployment", "dep_fixture", "--revision", "1", "--output", output}, strings.NewReader(""), &stdout, &stderr, credentials)
	if code != 2 {
		t.Fatal("existing enrollment file overwritten")
	}
}

func TestRedirectCannotExfiltrateAPIKey(t *testing.T) {
	var reached bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; t.Error("redirect followed") }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	c, err := NewClient(Credentials{Endpoint: server.URL, APIKey: testKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.JSON(context.Background(), "GET", "/account", nil); err == nil || reached {
		t.Fatalf("redirect accepted: %v", err)
	}
}

func TestErrorDoesNotEchoServerCredentialReflection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": r.Header.Get("Authorization")})
	}))
	defer server.Close()
	c, err := NewClient(Credentials{Endpoint: server.URL, APIKey: testKey})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.JSON(context.Background(), "GET", "/account", nil)
	if err == nil || strings.Contains(err.Error(), testKey) {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestUploadUsesOriginalBytes(t *testing.T) {
	contents := []byte("fixture JAR bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, _, err := r.FormFile("payload")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(file)
		if err != nil || !bytes.Equal(raw, contents) {
			t.Error("uploaded bytes differ")
		}
		io.WriteString(w, `{"id":"bld_fixture"}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "product-guard.jar")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(Credentials{Endpoint: server.URL, APIKey: testKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = upload(context.Background(), c, "demo.product", path); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeEndpointAndCredentialPermissionsRejected(t *testing.T) {
	for _, endpoint := range []string{"http://example.test", "https://user:pass@example.test", "https://example.test/path", "https://example.test?token=secret", "file:///tmp/endpoint"} {
		if _, err := NewClient(Credentials{Endpoint: endpoint, APIKey: testKey}); err == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCredentials(path); err == nil {
		t.Fatal("world-readable credentials accepted")
	}
}

func TestDeveloperInputErrorsUseStableCodeAndInputExit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "INVALID_INPUT", "message": r.Header.Get("Authorization")})
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "credentials.json")
	raw, _ := json.Marshal(Credentials{Endpoint: server.URL, APIKey: testKey})
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"products", "create", "--product", "wrong-prefix", "--name", "test"}, strings.NewReader(""), &out, &errOut, file)
	if code != 2 || !strings.Contains(errOut.String(), "INVALID_INPUT") || strings.Contains(errOut.String(), testKey) {
		t.Fatalf("input classified incorrectly: %d %s", code, errOut.String())
	}
}
