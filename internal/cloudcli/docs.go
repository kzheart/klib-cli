package cloudcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const defaultWiki = "kzheart/klib"
const maxWikiBytes = 2 << 20
const maxWikiPages = 128

var wikiRepoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var wikiLinks = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)|\[\[([^\]\n]+)\]\]`)

type wikiPage struct {
	Page  string `json:"page"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
type wikiDocument struct {
	wikiPage
	Content       string `json:"content"`
	ContentSHA256 string `json:"content_sha256"`
	FetchedAt     string `json:"fetched_at"`
	ETag          string `json:"etag,omitempty"`
}
type wikiClient struct {
	repo    string
	http    *http.Client
	rawBase string
}

func newWikiClient(repo string) *wikiClient {
	return &wikiClient{repo: repo, rawBase: "https://raw.githubusercontent.com/wiki/" + repo + "/", http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return errors.New("Wiki 请求不接受重定向")
	}}}
}
func (c *wikiClient) page(slug, title string) wikiPage {
	return wikiPage{Page: slug, Title: title, URL: "https://github.com/" + c.repo + "/wiki/" + url.PathEscape(slug)}
}
func (c *wikiClient) fetch(ctx context.Context, p wikiPage) (wikiDocument, error) {
	var doc wikiDocument
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.rawBase+url.PathEscape(p.Page)+".md", nil)
	if err != nil {
		return doc, err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Cache-Control", "no-cache")
	response, err := c.http.Do(req)
	if err != nil {
		return doc, &TransportError{Message: "无法读取 Wiki，请检查网络后重试；未使用本地或内置教程"}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return doc, &APIError{Status: response.StatusCode, Code: "WIKI_UNAVAILABLE"}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxWikiBytes+1))
	if err != nil {
		return doc, &TransportError{Message: "Wiki 下载未完成"}
	}
	if len(body) > maxWikiBytes || !utf8.Valid(body) || len(body) == 0 {
		return doc, &TransportError{Message: "Wiki 页面过大、为空或不是 UTF-8 文本"}
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/html") {
		return doc, &TransportError{Message: "Wiki 返回了 HTML，不能作为 Markdown 教程使用"}
	}
	hash := sha256.Sum256(body)
	return wikiDocument{wikiPage: p, Content: string(body), ContentSHA256: hex.EncodeToString(hash[:]), FetchedAt: time.Now().UTC().Format(time.RFC3339Nano), ETag: response.Header.Get("ETag")}, nil
}
func wikiSlug(repo, target string) string {
	// Follow only pages of the selected public GitHub Wiki, never arbitrary document links.
	if strings.HasPrefix(target, "https://github.com/"+repo+"/wiki/") {
		target = strings.TrimPrefix(target, "https://github.com/"+repo+"/wiki/")
	}
	target = strings.SplitN(target, "#", 2)[0]
	decoded, err := url.PathUnescape(target)
	if err != nil {
		return ""
	}
	decoded = strings.TrimSuffix(strings.TrimPrefix(decoded, "./"), ".md")
	if decoded == "" || strings.ContainsAny(decoded, "/\\:?\x00\r\n") || strings.Contains(decoded, "..") || strings.HasPrefix(decoded, "_") {
		return ""
	}
	return strings.ReplaceAll(decoded, " ", "-")
}
func (c *wikiClient) index(ctx context.Context) ([]wikiPage, wikiDocument, error) {
	index, err := c.fetch(ctx, c.page("_Sidebar", "目录"))
	// Wikis without a sidebar can maintain their entry links in Home instead.
	var api *APIError
	if errors.As(err, &api) && api.Status == 404 {
		index, err = c.fetch(ctx, c.page("Home", "首页"))
	}
	if err != nil {
		return nil, index, err
	}
	pages := []wikiPage{c.page("Home", "首页")}
	seen := map[string]bool{"home": true}
	for _, m := range wikiLinks.FindAllStringSubmatch(index.Content, -1) {
		title, target := m[1], m[2]
		if m[3] != "" {
			parts := strings.SplitN(m[3], "|", 2)
			title, target = parts[0], parts[0]
			if len(parts) == 2 {
				target = parts[1]
			}
		}
		slug := wikiSlug(c.repo, target)
		if slug == "" || seen[strings.ToLower(slug)] {
			continue
		}
		if len(pages) >= maxWikiPages {
			return nil, index, &TransportError{Message: "Wiki 目录超过 128 页，未返回截断目录"}
		}
		seen[strings.ToLower(slug)] = true
		pages = append(pages, c.page(slug, title))
	}
	if len(pages) == 1 && index.Page == "_Sidebar" {
		return nil, index, &TransportError{Message: "Wiki 目录中没有可读取的页面链接"}
	}
	return pages, index, nil
}
func docsCommand(ctx context.Context, args []string, out io.Writer) error {
	return docsCommandWithClient(ctx, args, out, newWikiClient)
}
func docsCommandWithClient(ctx context.Context, args []string, out io.Writer, factory func(string) *wikiClient) error {
	if len(args) == 0 {
		return errors.New("请执行 klib docs list、klib docs search 关键词 或 klib docs read 页面")
	}
	action := args[0]
	if action != "list" && action != "search" && action != "read" {
		return errors.New("未知 docs 子命令")
	}
	f := flag.NewFlagSet("docs", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	repo := f.String("repo", defaultWiki, "公开 GitHub Wiki 仓库")
	limit := f.Int("limit", 20, "最多匹配页数（search）")
	var flags, positionals []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if arg == "--repo" || arg == "--limit" {
				i++
				if i >= len(args) {
					return errors.New("缺少文档命令选项值")
				}
				flags = append(flags, args[i])
			}
		} else {
			positionals = append(positionals, arg)
		}
	}
	if err := f.Parse(flags); err != nil || len(f.Args()) != 0 {
		return errors.New("无效的 docs 参数")
	}
	if !wikiRepoPattern.MatchString(*repo) || strings.Contains(*repo, "..") || *limit < 1 || *limit > 100 {
		return errors.New("--repo 必须为 OWNER/REPO，--limit 范围为 1..100")
	}
	if (action == "list" && len(positionals) != 0) || (action != "list" && len(positionals) != 1) || (len(positionals) == 1 && strings.TrimSpace(positionals[0]) == "") {
		return errors.New("list 不接受位置参数；search/read 需要一个关键词或页面参数（含空格时请加引号）")
	}
	if action != "search" {
		var invalid bool
		f.Visit(func(v *flag.Flag) {
			if v.Name == "limit" {
				invalid = true
			}
		})
		if invalid {
			return errors.New("--limit 只适用于 docs search")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	c := factory(*repo)
	pages, index, err := c.index(ctx)
	if err != nil {
		return err
	}
	emit := func(v any) error { return json.NewEncoder(out).Encode(v) }
	switch action {
	case "list":
		return emit(map[string]any{"repository": *repo, "index_url": index.URL, "index_content_sha256": index.ContentSHA256, "fetched_at": index.FetchedAt, "pages": pages, "scope": "sidebar_or_home_links"})
	case "read":
		wanted := strings.TrimSpace(positionals[0])
		for _, p := range pages {
			if strings.EqualFold(wanted, p.Page) || strings.EqualFold(wanted, p.Title) || wikiSlug(*repo, wanted) == p.Page {
				doc, err := c.fetch(ctx, p)
				if err != nil {
					return err
				}
				return emit(doc)
			}
		}
		return &APIError{Status: 404, Code: "WIKI_PAGE_NOT_IN_INDEX"}
	}
	type match struct {
		wikiPage
		ContentSHA256 string `json:"content_sha256"`
		FetchedAt     string `json:"fetched_at"`
		Snippet       string `json:"snippet"`
		score         int
	}
	results := make([]match, 0)
	terms := strings.Fields(strings.ToLower(positionals[0]))
	var mu sync.Mutex
	var firstErr error
	jobs := make(chan wikiPage)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				doc, err := c.fetch(ctx, p)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("Wiki 搜索未完成（%s）：%w", p.Page, err)
					}
					mu.Unlock()
					continue
				}
				text := strings.ToLower(p.Title + "\n" + p.Page + "\n" + doc.Content)
				found := true
				score := 0
				for _, term := range terms {
					if !strings.Contains(text, term) {
						found = false
						break
					}
					score += min(strings.Count(strings.ToLower(doc.Content), term), 100)
					if strings.Contains(strings.ToLower(p.Title+" "+p.Page), term) {
						score += 10
					}
				}
				if !found {
					continue
				}
				snippet := ""
				for _, line := range strings.Split(doc.Content, "\n") {
					for _, term := range terms {
						if strings.Contains(strings.ToLower(line), term) {
							snippet = strings.TrimSpace(line)
							break
						}
					}
					if snippet != "" {
						break
					}
				}
				if runes := []rune(snippet); len(runes) > 240 {
					snippet = string(runes[:240]) + "…"
				}
				mu.Lock()
				results = append(results, match{wikiPage: p, ContentSHA256: doc.ContentSHA256, FetchedAt: doc.FetchedAt, Snippet: snippet, score: score})
				mu.Unlock()
			}
		}()
	}
	for _, p := range pages {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].Page < results[j].Page
	})
	total := len(results)
	if total > *limit {
		results = results[:*limit]
	}
	return emit(map[string]any{"repository": *repo, "query": positionals[0], "searched_pages": len(pages), "total_matches": total, "matches": results, "index_url": index.URL, "index_content_sha256": index.ContentSHA256, "scope": "sidebar_or_home_links"})
}
