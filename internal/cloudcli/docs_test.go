package cloudcli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func wikiFixture(t *testing.T, pages map[string]string) func(string) *wikiClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("document request leaked credentials")
		}
		if body, ok := pages[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("ETag", `"fixture"`)
			_, _ = w.Write([]byte(body))
		} else {
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return func(repo string) *wikiClient {
		return &wikiClient{repo: repo, http: server.Client(), rawBase: server.URL + "/"}
	}
}
func TestWikiLiveIndexReadSearch(t *testing.T) {
	factory := wikiFixture(t, map[string]string{
		"/_Sidebar.md":   "[首页](Home)\n[数据](Data)\n[[新模块|New-Module]]\n[重复](Data)\n[外链](https://example.test/secret)\n[穿越](../secret)\n[编码穿越](%2e%2e%2fsecret)",
		"/Home.md":       "# 教程\n适用于 Klib 示例版本",
		"/Data.md":       "# 数据\n数据库支持 SQLite 与中文检索。",
		"/New-Module.md": "# 新模块\n无需更新 CLI 即可读取新增教程。",
	})
	var out bytes.Buffer
	if err := docsCommandWithClient(context.Background(), []string{"list"}, &out, factory); err != nil {
		t.Fatal(err)
	}
	var index struct {
		Pages []wikiPage `json:"pages"`
	}
	if err := json.Unmarshal(out.Bytes(), &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Pages) != 3 || index.Pages[2].Page != "New-Module" {
		t.Fatalf("dynamic index = %+v", index)
	}
	out.Reset()
	if err := docsCommandWithClient(context.Background(), []string{"read", "数据", "--repo", "owner/docs"}, &out, factory); err != nil {
		t.Fatal(err)
	}
	var doc wikiDocument
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Page != "Data" || doc.URL != "https://github.com/owner/docs/wiki/Data" || len(doc.ContentSHA256) != 64 || doc.ETag != `"fixture"` || !strings.Contains(doc.Content, "数据库") {
		t.Fatalf("bad document: %+v", doc)
	}
	out.Reset()
	if err := docsCommandWithClient(context.Background(), []string{"search", "数据库 sqlite", "--limit", "1"}, &out, factory); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Matches []struct {
			Page    string
			Snippet string
		}
		Total    int `json:"total_matches"`
		Searched int `json:"searched_pages"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Searched != 3 || len(result.Matches) != 1 || result.Matches[0].Page != "Data" || !strings.Contains(result.Matches[0].Snippet, "数据库") {
		t.Fatalf("bad search: %+v", result)
	}
}
func TestWikiFallbackAndFailedSearch(t *testing.T) {
	factory := wikiFixture(t, map[string]string{"/Home.md": "[新增教程](Added)", "/Added.md": "新的内容"})
	var out bytes.Buffer
	if err := docsCommandWithClient(context.Background(), []string{"read", "Added"}, &out, factory); err != nil {
		t.Fatal(err)
	}
	factory = wikiFixture(t, map[string]string{"/_Sidebar.md": "[缺失](Missing)", "/Home.md": "查询词"})
	out.Reset()
	if err := docsCommandWithClient(context.Background(), []string{"search", "查询词"}, &out, factory); err == nil || !strings.Contains(err.Error(), "搜索未完成") {
		t.Fatalf("partial search claimed success: %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("partial results emitted as complete")
	}
}
func TestWikiRejectsInvalidInputBeforeNetwork(t *testing.T) {
	factory := func(string) *wikiClient { t.Fatal("invalid input reached network"); return nil }
	for _, args := range [][]string{{"list", "--repo", "https://evil.test"}, {"read", "Data", "--limit", "2"}, {"search", ""}, {"search", "data", "--limit", "0"}, {"read", "Home", "extra"}, {"list", "--token", "secret"}} {
		var out bytes.Buffer
		if err := docsCommandWithClient(context.Background(), args, &out, factory); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"docs", "list", "--repo", "bad"}, nil, &out, &errOut, t.TempDir()+"/missing-credentials.json"); code != 2 || strings.Contains(errOut.String(), "登录") {
		t.Fatalf("docs depended on login: %d %s", code, &errOut)
	}
}
func TestWikiReadDoesNotFollowUnindexedURL(t *testing.T) {
	factory := wikiFixture(t, map[string]string{"/_Sidebar.md": "[数据](Data)"})
	var out bytes.Buffer
	err := docsCommandWithClient(context.Background(), []string{"read", "https://example.test/secret"}, &out, factory)
	if err == nil {
		t.Fatal("accepted external page")
	}
}
func TestWikiRejectsOversizedDocument(t *testing.T) {
	factory := wikiFixture(t, map[string]string{"/Home.md": strings.Repeat("x", maxWikiBytes+1)})
	var out bytes.Buffer
	if err := docsCommandWithClient(context.Background(), []string{"read", "Home"}, &out, factory); err == nil {
		t.Fatal("accepted oversized document")
	}
}

func TestWikiRejectsRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/unexpected" {
			t.Error("followed redirect outside original document")
			return
		}
		http.Redirect(w, r, "/unexpected", http.StatusFound)
	}))
	defer server.Close()
	c := newWikiClient(defaultWiki)
	c.rawBase = server.URL + "/"
	if _, err := c.fetch(context.Background(), c.page("Home", "Home")); err == nil {
		t.Fatal("redirect treated as document")
	}
}
