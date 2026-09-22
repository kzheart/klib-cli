# Klib CLI

Klib 云端插件开发客户端与独立的 `klib-cloud-development` Skill。此仓库只分发安装包、Skill 和使用文档，不包含授权服务器、Guard Java 或 Native 实现。

**当前为预发布，要求配套的部署授权协议 v5 Collector 与 KlibGuard。正式服务尚未完成 v5 切换；安装成功不表示现有正式服务已经支持这些命令。**

CLI 支持 macOS ARM64、Linux x64、Windows x64。安装无需 Go、Java 或管理员权限；编译业务插件仍需项目自己的构建工具。

## 安装 CLI

当前版本：`0.1.0-rc.1`。安装脚本和二进制固定到同一版本，不会自动升级。升级或降级时重新执行目标版本安装命令；校验失败不会覆盖现有程序。安装器不修改 Shell 配置、登录凭据或 Agent Skill。

macOS / Linux：

```sh
curl -fL --proto '=https' --proto-redir '=https' \
  https://github.com/kzheart/klib-cli/releases/download/cli-v0.1.0-rc.1/install.sh \
  -o /tmp/klib-install.sh
sh /tmp/klib-install.sh --repo kzheart/klib-cli --version 0.1.0-rc.1
export PATH="$HOME/.local/bin:$PATH"
klib version
klib schema
```

可通过 `--install-dir DIR` 指定目录。将默认目录加入自己的 Shell PATH 后，新终端即可直接执行 `klib`。

Windows PowerShell：

```powershell
Invoke-WebRequest -UseBasicParsing `
  https://github.com/kzheart/klib-cli/releases/download/cli-v0.1.0-rc.1/install.ps1 `
  -OutFile "$env:TEMP\klib-install.ps1"
& "$env:TEMP\klib-install.ps1" -Repo kzheart/klib-cli -Version 0.1.0-rc.1
$env:PATH = "$env:LOCALAPPDATA\Klib\bin;$env:PATH"
klib version
klib schema
```

脚本遵循本机 PowerShell 执行策略；受策略限制时可下载 Release 中的 `.exe`，对照 `SHA256SUMS` 用 `Get-FileHash` 校验，再放入用户 PATH 目录。安装器不修改执行策略。Windows 升级前先退出正在运行的 CLI。

也可下载同一 Release 的平台二进制与 `SHA256SUMS`，通过 `--source-dir DIR` / `-SourceDir DIR` 离线安装。校验和用于核对下载完整性，不等同于代码签名；安装包未做 Apple 公证或 Windows Authenticode 签名。

## 安装独立 Skill

使用支持的 Agent Skills 安装器（需要 Node.js/npm）：

```sh
npx skills add kzheart/klib-cli --skill klib-cloud-development -g
```

按安装器提示选择 Agent。此步骤只安装 Skill，不安装 CLI，也不安装 mc-pilot。没有 Node.js 时，可以下载 Release 的 `klib-cloud-development.zip`，解压后将 `klib-cloud-development` 文件夹放入所用 Agent 的技能目录。压缩包与 CLI 来自同一版本；仓库安装跟随当前发布的 Skill。

Skill 使用 `klib schema` 了解命令，负责上传、权益、部署与发行。需要游戏测试时，另行安装独立的 mc-pilot Skill。CLI 本身无需 AI Agent 即可使用。

## 首次使用

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

每个 Release 包含三个平台的 CLI、两个安装脚本、Skill ZIP、`manifest.json`、`SHA256SUMS` 与第三方许可。源码提交及配套协议记录在 manifest 中。CLI 和 Skill 独立安装、独立卸载，按同一次发布验证版本配套。
