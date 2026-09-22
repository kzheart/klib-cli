package cloudcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Run returns stable exit codes: 0 success, 2 invalid/local input, 3 authentication,
// 4 conflict/not-found/quota, 5 transport/server error. Output is always JSON.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, credentialsPath string) int {
	err := run(ctx, args, in, out, credentialsPath)
	if err == nil {
		return 0
	}
	code := 2
	var transport *TransportError
	if errors.As(err, &transport) {
		code = 5
	}
	var api *APIError
	if errors.As(err, &api) {
		switch api.Status {
		case 400, 422:
			code = 2
		case 401, 403:
			code = 3
		case 404, 409:
			code = 4
		default:
			code = 5
		}
	}
	_ = json.NewEncoder(errOut).Encode(map[string]any{"ok": false, "error": err.Error(), "exit_code": code})
	return code
}

func run(ctx context.Context, args []string, in io.Reader, out io.Writer, credentialsPath string) error {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
		return json.NewEncoder(out).Encode(map[string]any{"name": "klib", "version": Version, "commit": Commit, "protocol_version": ProtocolVersion})
	}
	if len(args) == 1 && args[0] == "schema" {
		return json.NewEncoder(out).Encode(map[string]any{"name": "klib", "schema_version": 1, "output": "json", "commands": []string{"version", "auth login --endpoint URL --key-stdin", "account", "products list [--cursor CURSOR] [--limit N]", "products get --product ID", "products create --product ID --name NAME [--summary TEXT]", "entitlements list", "entitlements own --product ID", "deployments list", "deployments get --deployment ID", "deployments create --file JSON", "deployments select --deployment ID --file JSON", "deployments revoke --deployment ID --revision N", "enrollments issue --deployment ID --revision N --output FILE", "instances connect --deployment ID --output CONFIG_YML", "instances list --deployment ID", "instances revoke --deployment ID --instance ID", "builds list --product ID", "builds upload --product ID --file JAR", "releases create --build ID --entitlement ID --version VERSION --channel beta|stable"}})
	}
	if len(args) == 0 {
		return errors.New("请执行 klib schema 查看命令")
	}
	group, action := args[0], ""
	rest := args[1:]
	if group != "account" {
		if len(rest) == 0 {
			return errors.New("缺少子命令，请执行 klib schema")
		}
		action = rest[0]
		rest = rest[1:]
	}
	f := flag.NewFlagSet("klib", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	endpoint := f.String("endpoint", "", "云端地址")
	keyStdin := f.Bool("key-stdin", false, "从标准输入读取 Key")
	product := f.String("product", "", "商品")
	name := f.String("name", "", "商品名称")
	summary := f.String("summary", "", "商品简介")
	cursor := f.String("cursor", "", "下一页游标")
	limit := f.Int("limit", 50, "每页数量")
	dep := f.String("deployment", "", "部署")
	instance := f.String("instance", "", "实例")
	file := f.String("file", "", "输入文件")
	output := f.String("output", "", "输出文件")
	revision := f.Int64("revision", 0, "部署 revision")
	build := f.String("build", "", "构建")
	entitlement := f.String("entitlement", "", "权益")
	version := f.String("version", "", "发行版本")
	channel := f.String("channel", "beta", "发行通道")
	if err := f.Parse(rest); err != nil || len(f.Args()) != 0 {
		return errors.New("命令参数无效，请执行 klib schema")
	}
	if group == "auth" && action == "login" {
		if !*keyStdin || *endpoint == "" {
			return errors.New("登录需要 --endpoint 和 --key-stdin；不要把 API Key 放到命令参数")
		}
		raw, err := io.ReadAll(io.LimitReader(in, 514))
		if err != nil {
			return errors.New("无法从标准输入读取 Key")
		}
		defer clear(raw)
		c := Credentials{Endpoint: *endpoint, APIKey: strings.TrimSpace(string(raw))}
		client, err := NewClient(c)
		if err != nil {
			return err
		}
		account, err := client.JSON(ctx, "GET", "/account", nil)
		if err != nil {
			return err
		}
		saved, err := json.Marshal(c)
		if err != nil {
			return err
		}
		defer clear(saved)
		if err = WritePrivate(credentialsPath, saved); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]any{"ok": true, "account": json.RawMessage(account)})
	}
	c, err := ReadCredentials(credentialsPath)
	if err != nil {
		return err
	}
	client, err := NewClient(c)
	if err != nil {
		return err
	}
	if group == "products" {
		return productCommand(ctx, client, action, *product, *name, *summary, *cursor, *limit, out)
	}
	method, path := "GET", ""
	var body any
	switch group + " " + action {
	case "account ":
		path = "/account"
	case "entitlements list":
		path = "/entitlements"
	case "entitlements own":
		if *product == "" {
			return errors.New("缺少 --product")
		}
		method, path, body = "POST", "/entitlements/owned", map[string]string{"product_id": *product}
	case "deployments list":
		path = "/deployments"
	case "deployments get":
		if *dep == "" {
			return errors.New("缺少 --deployment")
		}
		path = "/deployments/" + *dep
	case "deployments create", "deployments select":
		if *file == "" {
			return errors.New("缺少 --file JSON 配置文件")
		}
		raw, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		if len(raw) > 64<<10 || !json.Valid(raw) {
			return errors.New("部署配置必须是小于 64 KiB 的 JSON")
		}
		body = json.RawMessage(raw)
		method, path = "POST", "/deployments"
		if action == "select" {
			if *dep == "" {
				return errors.New("缺少 --deployment")
			}
			method, path = "PUT", "/deployments/"+*dep+"/selection"
		}
	case "deployments revoke":
		if *dep == "" || *revision < 1 {
			return errors.New("需要 --deployment 和 --revision")
		}
		method, path, body = "POST", "/deployments/"+*dep+"/revoke", map[string]int64{"revision": *revision}
	case "instances connect":
		return connectInstance(ctx, client, *dep, *output, out)
	case "instances list":
		if *dep == "" {
			return errors.New("缺少 --deployment")
		}
		path = "/deployments/" + *dep + "/instances"
	case "instances revoke":
		if *dep == "" || *instance == "" {
			return errors.New("需要 --deployment 和 --instance")
		}
		method, path = "POST", "/deployments/"+*dep+"/instances/"+*instance+"/revoke"
	case "builds list":
		if *product == "" {
			return errors.New("缺少 --product")
		}
		path = "/products/" + *product + "/builds"
	case "builds upload":
		if *product == "" || *file == "" {
			return errors.New("需要 --product 和 --file")
		}
		raw, err := upload(ctx, client, *product, *file)
		if err != nil {
			return err
		}
		return printJSON(out, raw)
	case "releases create":
		if *build == "" || *entitlement == "" || *version == "" {
			return errors.New("需要 --build、--entitlement 和 --version")
		}
		method, path, body = "POST", "/builds/"+*build+"/releases", map[string]string{"entitlement_id": *entitlement, "version": *version, "channel": *channel}
	case "enrollments issue":
		if *dep == "" || *revision < 1 || *output == "" {
			return errors.New("需要 --deployment、--revision 和 --output；登记凭据只写入私有文件")
		}
		if _, err = os.Lstat(*output); !errors.Is(err, os.ErrNotExist) {
			return errors.New("登记输出文件已存在或不可检查，请指定新路径")
		}
		raw, err := client.JSON(ctx, "POST", "/deployments/"+*dep+"/enrollments", map[string]int64{"revision": *revision})
		if err != nil {
			return err
		}
		defer clear(raw)
		if err = WritePrivateNew(*output, raw); err != nil {
			return fmt.Errorf("登记凭据已签发但未保存，请重新签发并等待旧凭据到期：%w", err)
		}
		var summary map[string]any
		if err = json.Unmarshal(raw, &summary); err != nil {
			return err
		}
		delete(summary, "token")
		summary["output_file"] = *output
		return json.NewEncoder(out).Encode(summary)
	default:
		return errors.New("未知命令，请执行 klib schema")
	}
	raw, err := client.JSON(ctx, method, path, body)
	if err != nil {
		return err
	}
	return printJSON(out, raw)
}

func printJSON(w io.Writer, raw json.RawMessage) error {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, raw, "", "  "); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, formatted.String())
	return err
}

func upload(ctx context.Context, c *Client, product, path string) (json.RawMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 128<<20 {
		return nil, errors.New("制品必须是 1 字节至 128 MiB 的普通 JAR 文件")
	}
	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	done := make(chan error, 1)
	go func() {
		part, err := form.CreateFormFile("payload", filepath.Base(path))
		if err == nil {
			_, err = io.Copy(part, io.LimitReader(file, (128<<20)+1))
		}
		if err == nil {
			err = form.Close()
		}
		_ = writer.CloseWithError(err)
		done <- err
	}()
	raw, requestErr := c.Request(ctx, "POST", "/products/"+product+"/builds", form.FormDataContentType(), reader)
	_ = reader.CloseWithError(requestErr)
	writeErr := <-done
	if requestErr != nil {
		return nil, requestErr
	}
	if writeErr != nil {
		return nil, errors.New("上传文件读取失败")
	}
	return raw, nil
}

// Product operations use the existing authenticated developer API.
func productCommand(ctx context.Context, client *Client, action, product, name, summary, cursor string, limit int, out io.Writer) error {
	method, path := "GET", "/products"
	var query url.Values
	var body io.Reader
	switch action {
	case "list":
		if limit < 1 || limit > 100 {
			return errors.New("--limit 必须在 1 至 100 之间")
		}
		query = url.Values{"limit": {strconv.Itoa(limit)}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
	case "get":
		if product == "" {
			return errors.New("缺少 --product")
		}
		path += "/" + product
	case "create":
		if product == "" || strings.TrimSpace(name) == "" {
			return errors.New("需要 --product 和 --name")
		}
		method = "POST"
		raw, err := json.Marshal(map[string]string{"id": product, "name": name, "summary": summary})
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	default:
		return errors.New("未知商品命令，请执行 klib schema")
	}
	raw, err := client.request(ctx, method, "/developer", path, query, "application/json", body)
	if err != nil {
		return err
	}
	return printJSON(out, raw)
}
