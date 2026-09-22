package cloudcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Credentials struct {
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key"`
}
type Client struct {
	Credentials Credentials
	HTTP        *http.Client
}
type TransportError struct{ Message string }

func (e *TransportError) Error() string { return e.Message }

type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("云端请求失败（HTTP %d，%s）", e.Status, e.Code)
}

func validEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("endpoint 必须是无路径、凭据或查询参数的 HTTPS 地址")
	}
	ip := net.ParseIP(u.Hostname())
	local := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return errors.New("远程 endpoint 必须使用 HTTPS")
	}
	return nil
}

func NewClient(c Credentials) (*Client, error) {
	if err := validEndpoint(c.Endpoint); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(c.APIKey, "sk_") || len(c.APIKey) > 512 || strings.ContainsAny(c.APIKey, "\r\n\x00") {
		return nil, errors.New("开发者 API Key 格式无效")
	}
	c.Endpoint = strings.TrimRight(c.Endpoint, "/")
	return &Client{Credentials: c, HTTP: &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Request(ctx context.Context, method, path, contentType string, body io.Reader) (json.RawMessage, error) {
	return c.request(ctx, method, "/developer/cloud", path, nil, contentType, body)
}

func (c *Client) request(ctx context.Context, method, base, path string, query url.Values, contentType string, body io.Reader) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#\r\n") || strings.Contains(path, "..") || strings.Contains(path, "%") {
		return nil, errors.New("无效的 API 路径")
	}
	target := c.Credentials.Endpoint + base + path
	if len(query) != 0 {
		target += "?" + query.Encode()
	}
	r, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, errors.New("无法创建云端请求")
	}
	r.Header.Set("Authorization", "Bearer "+c.Credentials.APIKey)
	r.Header.Set("Accept", "application/json")
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	response, err := c.HTTP.Do(r)
	if err != nil {
		return nil, &TransportError{Message: "无法连接云端服务，请检查 endpoint 和网络"}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return nil, &TransportError{Message: "云端响应不可读取或过大"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(raw, &payload)
		// Do not echo arbitrary server responses: proxies may include request credentials.
		code := "REQUEST_FAILED"
		if payload.Error == "" {
			payload.Error = payload.Code
		}
		switch payload.Error {
		case "INVALID_INPUT", "NOT_FOUND", "AUTHORIZATION_DENIED", "REVISION_OR_STATE_CONFLICT", "INSTANCE_QUOTA_EXCEEDED", "UNAUTHORIZED", "INTERNAL_ERROR":
			code = payload.Error
		}
		return nil, &APIError{Status: response.StatusCode, Code: code}
	}
	if !json.Valid(raw) {
		return nil, &TransportError{Message: "云端未返回有效 JSON"}
	}
	return raw, nil
}

func (c *Client) JSON(ctx context.Context, method, path string, v any) (json.RawMessage, error) {
	var body io.Reader
	if v != nil {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	return c.Request(ctx, method, path, "application/json", body)
}

func ReadCredentials(path string) (Credentials, error) {
	var c Credentials
	info, err := os.Lstat(path)
	if err != nil {
		return c, errors.New("尚未登录，请先执行 klib auth login")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return c, errors.New("凭据文件必须是仅当前用户可读写的普通文件（0600）")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, errors.New("无法读取凭据文件")
	}
	defer clear(raw)
	if err = json.Unmarshal(raw, &c); err != nil {
		return c, errors.New("凭据文件格式无效")
	}
	return c, nil
}

func WritePrivate(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("凭据目录必须是真实目录")
	}
	f, err := os.CreateTemp(dir, ".klib-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return nil
}

// WritePrivateNew claims the destination atomically; it never replaces an existing configuration.
func WritePrivateNew(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Dir(path))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("输出目录必须是真实目录")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	saved := false
	defer func() {
		f.Close()
		if !saved {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	saved = true
	return nil
}
