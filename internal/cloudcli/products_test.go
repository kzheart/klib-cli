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

func TestProductCommandsUseAuthenticatedExistingAPI(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Error("missing authentication")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /developer/products":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["id"] != "fixture-product" || body["name"] != "测试商品" || body["summary"] != "用于联调" {
				t.Errorf("unexpected product: %v", body)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"product":{"id":"fixture-product"}}`)
		case "GET /developer/products":
			if r.URL.Query().Get("cursor") != "cursor+/=&value" || r.URL.Query().Get("limit") != "25" || len(r.URL.Query()) != 2 {
				t.Errorf("unsafe query: %v", r.URL.Query())
			}
			io.WriteString(w, `{"products":[],"next_cursor":"next-page"}`)
		case "GET /developer/products/fixture-product":
			io.WriteString(w, `{"product":{"id":"fixture-product"}}`)
		default:
			t.Errorf("unexpected API: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	credentials := filepath.Join(t.TempDir(), "credentials.json")
	raw, _ := json.Marshal(Credentials{Endpoint: server.URL, APIKey: testKey})
	if err := os.WriteFile(credentials, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"products", "create", "--product", "fixture-product", "--name", "测试商品", "--summary", "用于联调"},
		{"products", "list", "--cursor", "cursor+/=&value", "--limit", "25"},
		{"products", "get", "--product", "fixture-product"},
	} {
		var out, errOut bytes.Buffer
		if code := Run(context.Background(), args, strings.NewReader(""), &out, &errOut, credentials); code != 0 {
			t.Fatalf("%v: %d %s", args, code, &errOut)
		}
		if !json.Valid(out.Bytes()) || strings.Contains(out.String()+errOut.String(), testKey) {
			t.Fatal("invalid or sensitive output")
		}
	}
	for _, args := range [][]string{
		{"products", "create", "--product", "fixture-product"},
		{"products", "list", "--limit", "101"},
		{"products", "get", "--product", "../account"},
		{"products", "get", "--product", "fixture?secret=true"},
	} {
		if code := Run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard, credentials); code != 2 {
			t.Fatalf("bad input accepted: %v (%d)", args, code)
		}
	}
	if calls != 3 {
		t.Fatalf("invalid input reached API: %d", calls)
	}
}
