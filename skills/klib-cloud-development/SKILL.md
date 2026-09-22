---
name: klib-cloud-development
description: 使用 klib CLI 上传 Klib 云端私人插件构建、管理自有权益和部署实例、切换构建及发行版本。适用于云端插件交付与授权联调。
---

# Klib 云端插件开发

可通过 `KLIB_CONFIG_DIR` 指定独立凭据目录，测试账号不要覆盖日常登录。先运行 `klib schema`，以当前 CLI 输出为命令事实源。此技能随部署授权协议 v5 交付；使用匹配版本的 Collector、KlibGuard 和 CLI。

## 云端操作

根据用户请求选择所需步骤；仅上传构建时，上传完成即可交付，不自动扩展为建服、游戏测试或发行。

1. 在项目中读取构建说明，生成受保护插件的通用 JAR。使用项目已有构建任务，先核对 Gradle 支持的 JDK 与项目 toolchain；不要直接沿用机器默认 JDK。不把 Guard runtime 本身当作业务插件上传。
2. `klib account` 确认云端身份。尚未登录时，用 `klib auth login --endpoint <origin> --key-stdin` 从标准输入接收管理 Key；不把 Key 写入参数、日志、Skill、服务器配置或 Git。
3. 用 `klib products list` 查询自己的商品；响应有 `next_cursor` 时用 `--cursor` 继续。新商品 ID 必须采用 `klib account` 返回的 `product_namespace` 前缀，例如 `<username>.bamteam`。执行 `klib products create --product <id> --name <name>`，无需进入网页。随后 `klib entitlements own --product <product_id>` 获取自有商品测试权益。不创建隐藏买家，不给自己认领兑换码。
4. `klib builds upload --product <product_id> --file <artifact.jar>` 上传私人构建，保存返回的 build_id、object_id 与商品 ID。上传成功仅表示制品已存储。
5. 创建或选择开发测试部署。配置文件仅含非秘密的商品选择：

```json
{
  "name": "插件联调",
  "purpose": "development",
  "products": [{
    "product_id": "<product_id>",
    "entitlement_id": "<entitlement_id>",
    "kind": "build",
    "build_id": "<build_id>"
  }]
}
```

运行 `klib deployments create --file <deployment.json>`。更新已有部署时先读取 `klib deployments get --deployment <id>`，将当前 revision 和完整 products 列表交给 `klib deployments select --deployment <id> --file <selection.json>`。冲突后重新读取并核对，不能盲目覆盖他人的修改。

6. `klib instances connect --deployment <id> --output <new-config.yml>` 将短时登记凭据写入仅当前用户可读写的文件。配置中的 `connection` 用于隔离本机身份；切换构建只更新部署选择，不重新执行 connect。实例被吊销后，connect 会生成新 connection 和新身份，旧实例不会复活。把该配置安装到测试服的 `plugins/KlibGuard/config.yml` 前，保留原有配置并核对目标测试实例。连接命令不覆盖既有文件，不代表服务器已经登记或启用插件。
7. 当任务包含实际加载验证时，核对目标服务器 `KLIB_DEPLOYMENT_ENABLED` 日志中的商品、build、object、revision 是否与部署选择一致。该日志是加载证据，不能单独证明插件业务功能正常；尚未启动服务器时，应报告等待登记或未验证加载。

登记凭据首次使用前只有 10 分钟有效，且只能登记一个实例。已登记服务器使用 Native 持有的实例身份；吊销后不可用旧凭据恢复。不要提交配置、凭据、实例身份或测试私有数据。

## 与游戏测试协作

本 Skill 可独立使用，不依赖 mc-pilot。只有任务包含游戏内行为测试时，才按用户选择使用独立的测试工具或 Skill；环境中提供 mc-pilot 时可与其组合。服务器与客户端操作、玩家动作及业务断言由测试 Skill 负责，此处不维护其命令或测试流程。

向测试步骤交接目标服务器、配置文件位置及预期 product/build/object/revision；不要传递管理 API Key。将测试结果关联到实际加载的构建，分别报告上传、加载和业务验证状态。

## 发行与结束

需要发行时，从刚才验证通过的 build_id 执行 `klib releases create --build <id> --entitlement <id> --version <semver> --channel beta|stable`。这是买家可见的发行操作，须在用户请求的发行范围内执行。不要重新构建后把新产物当作已测试制品。

一次性测试实例不再使用时，执行 `klib instances revoke --deployment <id> --instance <id>` 释放额度。保留必要的非秘密证据，报告构建、实际加载与业务验证分别达到哪一步。

CLI 退出码：0 成功；2 输入或本地配置错误；3 身份或权限失败；4 不存在、修订冲突或额度不足；5 网络或服务端错误。自动化读取 JSON，禁止根据日志中包含“成功”猜测执行结果。
