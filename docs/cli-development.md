# CLI 云端开发

此流程面向部署授权协议 v5，尚未发布到正式服务。CLI、Collector 和 Guard 必须配套。开发自己的插件不需要创建买家或认领兑换码。

## 构建与登录

安装方式见仓库 README。运行 `klib schema` 查看当前命令。云端 Skill 与 mc-pilot 独立安装。

```sh
# 测试账号使用独立目录，避免覆盖日常登录。
export KLIB_CONFIG_DIR=/your/private/klib-test
klib auth login --endpoint "$KLIB_ENDPOINT" --key-stdin < /your/private/developer-key
klib account
klib products list
```

API Key 仅用于管理接口，不传给游戏服。凭据文件为 0600；CLI 不跟随 HTTP 重定向。远端地址要求 HTTPS，回环 HTTP 仅用于隔离测试。

账号返回 `product_namespace`；商品 ID 必须以此为前缀。例如返回 `alice.` 时：

```sh
klib products create --product alice.bamteam --name BamTeam
klib entitlements own --product alice.bamteam
```

## 上传与连接

先按业务项目的构建说明选择 JDK。BamTeam 当前构建命令为 `./gradlew :bamteam-cloud:guardProductJar --no-configuration-cache`，其 Gradle 使用 JDK 17 可构建；不要直接套用机器默认 JDK。

```sh
klib builds upload --product alice.bamteam --file bamteam-cloud/build/libs/bamteam-cloud-0.1.4-guard.jar
```

保存返回的权益 ID 和构建 ID，创建 `deployment.json`：

```json
{
  "name": "BamTeam 联调",
  "purpose": "development",
  "products": [{
    "product_id": "alice.bamteam",
    "entitlement_id": "<返回的权益 ID>",
    "kind": "build",
    "build_id": "<返回的构建 ID>"
  }]
}
```

```sh
klib deployments create --file deployment.json
klib instances connect --deployment <部署ID> --output /your/private/new-config.yml
```

把生成的配置安装到目标服 `plugins/KlibGuard/config.yml`，保留旧配置后重启。`connection` 决定本机身份目录，不能手工替换或在切换构建时更改。登记凭据只在首次连接时使用，10 分钟内有效。connect 返回 `waiting_for_instance`，不代表加载成功。

## 验证、更新与重新连接

任务包含游戏行为验证时，可使用独立的 mc-pilot Skill 连接真实玩家，执行业务动作。BamTeam 可用 `/bamteam ui`、`/bamteam create`、`/bamteam view` 核对界面、创建结果和成员状态；测试场景先排除生物伤害等干扰。不要把命令发送成功当作业务成功。

同时核对服务器 `KLIB_DEPLOYMENT_ENABLED` 日志中的 product、build、object 和 revision。产品可能有延迟初始化，还应检查业务 READY 与玩家行为。

上传新构建后先 `klib deployments get --deployment <部署ID>`，使用返回的 revision 和完整 products 列表生成 selection.json，再执行：

```sh
klib deployments select --deployment <部署ID> --file selection.json
```

selection.json 结构为 `{"revision":1,"products":[...]}`。此操作固定新构建，保留原实例身份；重启后重新核对加载证据。修订冲突时先重新读取。

吊销后重新连接时，再次执行 connect 并使用新配置。它生成新的 connection，建立新实例，不会复活旧身份。已登记的配置清空 enrollment-token 后仍使用同一身份。

## 发行与收尾

```sh
klib releases create --build <已验证构建ID> --entitlement <权益ID> --version 0.1.4 --channel beta
klib instances list --deployment <部署ID>
klib instances revoke --deployment <部署ID> --instance <实例ID>
```

发行引用刚验证的同一个不可变制品，不重新构建。CLI 写入含凭据的文件时拒绝覆盖。停止本次测试客户端与服务器，清理临时凭据，不提交账号 Key、配置中的登记凭据、实例身份或内部审计数据。

退出码：0 成功，2 输入错误，3 身份或权限失败，4 不存在/修订冲突/额度不足，5 网络或服务端错误。所有输出为 JSON。
