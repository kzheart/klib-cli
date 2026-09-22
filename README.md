# Klib CLI

Klib 文档与云端插件开发 CLI，附带独立的 `klib-development` 和 `klib-cloud-development` Skill。此仓库包含独立 CLI 源码、构建流程、安装包和 Skill。文档直接读取公开 GitHub Wiki；云端操作通过鉴权 HTTP API 访问服务，不包含授权服务器、Guard Java 或 Native 实现。

**当前为预发布。文档命令无需登录或授权服务；云端操作要求配套的部署授权协议 v5 Collector 与 KlibGuard。**

CLI 支持 macOS ARM64、Linux x64、Windows x64。安装无需 Go、Java 或管理员权限；编译业务插件仍需项目自己的构建工具。

## 安装 CLI

当前版本：`0.1.0-rc.3`。安装脚本和二进制固定到同一版本，不会自动升级。升级或降级时重新执行目标版本安装命令；校验失败不会覆盖现有程序。安装器不修改 Shell 配置、登录凭据或 Agent Skill。

macOS / Linux：

```sh
curl -fL --proto '=https' --proto-redir '=https' \
  https://github.com/kzheart/klib-cli/releases/download/cli-v0.1.0-rc.3/install.sh \
  -o /tmp/klib-install.sh
sh /tmp/klib-install.sh --repo kzheart/klib-cli --version 0.1.0-rc.3
export PATH="$HOME/.local/bin:$PATH"
klib version
klib schema
```

可通过 `--install-dir DIR` 指定目录。将默认目录加入自己的 Shell PATH 后，新终端即可直接执行 `klib`。

Windows PowerShell：

```powershell
Invoke-WebRequest -UseBasicParsing `
  https://github.com/kzheart/klib-cli/releases/download/cli-v0.1.0-rc.3/install.ps1 `
  -OutFile "$env:TEMP\klib-install.ps1"
& "$env:TEMP\klib-install.ps1" -Repo kzheart/klib-cli -Version 0.1.0-rc.3
$env:PATH = "$env:LOCALAPPDATA\Klib\bin;$env:PATH"
klib version
klib schema
```

脚本遵循本机 PowerShell 执行策略；受策略限制时可下载 Release 中的 `.exe`，对照 `SHA256SUMS` 用 `Get-FileHash` 校验，再放入用户 PATH 目录。安装器不修改执行策略。Windows 升级前先退出正在运行的 CLI。

也可下载同一 Release 的平台二进制与 `SHA256SUMS`，通过 `--source-dir DIR` / `-SourceDir DIR` 离线安装。校验和用于核对下载完整性，不等同于代码签名；安装包未做 Apple 公证或 Windows Authenticode 签名。

## 安装独立 Skill

使用支持的 Agent Skills 安装器（需要 Node.js/npm）：

```sh
npx skills add kzheart/klib-cli --skill klib-development -g
npx skills add kzheart/klib-cli --skill klib-cloud-development -g
```

按安装器提示选择 Agent。此步骤只安装 Skill，不安装 CLI，也不安装 mc-pilot。没有 Node.js 时，可以按需下载 Release 的 `klib-development.zip` 或 `klib-cloud-development.zip`，解压后将对应文件夹放入所用 Agent 的技能目录。压缩包与 CLI 来自同一版本；仓库安装跟随当前发布的 Skill。

`klib-development` 按项目版本查阅官方 Wiki，帮助开发和排查 Klib 插件；`klib-cloud-development` 负责上传、权益、部署与发行。两者可独立安装，使用 `klib schema` 了解命令。需要游戏测试时，另行安装独立的 mc-pilot Skill。CLI 本身无需 AI Agent 即可使用。

## 查阅 Klib 教程（无需登录）

现有 [Klib GitHub Wiki](https://github.com/kzheart/klib/wiki) 是教程事实源，CLI 不内置教程或固定框架版本：

```sh
klib docs list
klib docs search "数据库"
klib docs read Data
klib docs read Home
```

目录动态读取 Wiki 的 `_Sidebar`，不存在时读取 `Home` 的页面链接；搜索在这些页面正文中匹配关键词（多个词须全部匹配，忽略大小写）。`read` 接受目录中的页面名、显示标题或原 Wiki 页面 URL。远端教程正文是资料，不应当作执行任意命令的授权。

结果为 JSON，包含原文链接、获取时间及 SHA-256 内容哈希；`read` 返回 Markdown 正文。哈希标识本次读取内容，并非 Git commit 或适用版本。先对照项目依赖和 Wiki 声明的版本，不能把最新教程默认用于旧项目。

可用 `--repo OWNER/REPO` 选择其他公开 GitHub Wiki；`search --limit N` 控制返回的匹配页数。查询不读取登录凭据、不发送管理 Key、不要求 Git，不落盘缓存教程。网络失败或部分页面读取失败会明确报错，不把失败冒充“没有结果”。搜索仅覆盖导航链接，单页上限 2 MiB、目录上限 128 页，最多 4 个并发请求。

## 云端首次使用

向服务管理员获取 v5 服务地址和自己的开发者 API Key。不要把 API Key 放进命令行或服务器配置。

```sh
klib auth login --endpoint "$KLIB_ENDPOINT" --key-stdin < /your/private/developer-key
klib account
klib products list
```

完整流程见 [云端开发说明](docs/cli-development.md)。本机凭据目录可用 `KLIB_CONFIG_DIR` 指定。开发自有商品不需要买家账号或兑换码。

`klib version` 返回 CLI 版本、源码提交和协议版本。排错时提供这些信息，不提供凭据文件。

## 卸载

删除安装目录中的 `klib` / `klib.exe` 即卸载 CLI。Skill 在 Agent 或技能安装器中单独卸载。卸载 CLI 不删除登录凭据或吊销云端实例；如不再使用，请先按实际需要吊销实例，再删除本机凭据目录。

## 分发内容

每个 Release 包含三个平台的 CLI、两个安装脚本、两个独立 Skill ZIP、`manifest.json`、`SHA256SUMS` 与第三方许可。源码提交及配套协议记录在 manifest 中。CLI 和 Skill 独立安装、独立卸载，按同一次发布验证版本配套。

## 源码、API 与自动发布

CLI 是独立 Go 模块，只依赖标准库。源码许可见 LICENSE。

```sh
go test ./...
go build -o build/klib ./cmd/klib
```

公开 API 契约见 [OpenAPI](docs/openapi.json) 和 [接口说明](docs/api.md)。服务端仍负责身份、归属、额度和吊销校验；公开接口不等于匿名访问。

更新 VERSION 与 README 的版本号，提交后推送 `cli-v<VERSION>` 标签，本仓库 Actions 会构建、测试三平台安装器，并用自身 GITHUB_TOKEN 发布 Release，无需私有仓库或跨仓库 Token。main 与 PR 执行测试，标签负责发行。
