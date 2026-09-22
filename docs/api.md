# 公开开发者 API

[OpenAPI 3.0 契约](openapi.json) 描述本 CLI 所用接口。服务端还提供匿名可读的 `GET /developer/openapi.json`，方便其他语言的客户端获取契约；业务操作始终鉴权。

## 调用与身份

从服务管理员取得 v5 服务地址。远程调用使用 HTTPS，发送 `Authorization: Bearer <开发者 API Key>`。无需浏览器 Cookie 或买家身份。API Key 不可用于创建更多 Key 或修改账号密码；这些仍由原有账号管理渠道处理。

```sh
# KLIB_API_KEY 从本机秘密存储注入；不要记录请求头。
curl --fail --silent --show-error "$KLIB_ENDPOINT/developer/cloud/account" \
  -H "Authorization: Bearer $KLIB_API_KEY"
```

公开 API 是面向第三方程序的接口契约，不是匿名操作权限。服务端根据已验证账号确认商品归属、权益状态、部署归属、实例额度与吊销状态，不能在请求中伪造账号或自定额度。客户端不读数据库、不访问 SSH、不依赖服务端源码。

## 操作顺序

1. GET `/developer/cloud/account` 取得账号及 product_namespace。
2. GET/POST `/developer/products` 查询或创建商品；分页响应字段为 products、total、next_cursor。
3. POST `/developer/cloud/entitlements/owned`，JSON 为 `{"product_id":"alice.plugin"}`，取得自有权益。
4. POST `/developer/cloud/products/{productID}/builds`，multipart 仅包含 payload 文件，最大 128 MiB。
5. POST `/developer/cloud/deployments` 创建部署，或先 GET 再 PUT `/selection` 更新完整商品选择。
6. POST `/developer/cloud/deployments/{deploymentID}/enrollments`，携带当前 revision，取得 10 分钟有效的一次性登记凭据。凭据交给匹配版本的 Guard，不把 API Key 交给游戏服。
7. 验证构建后 POST `/developer/cloud/builds/{buildID}/releases`，引用同一 build_id 发行，不重新构建。

所有 cloud JSON 请求最多 64 KiB，并拒绝未知字段。部署最多 16 个商品。修改选择、吊销部署、签发登记凭据携带 revision；409 时重新读取并确认状态，不自动重试覆盖。上传、创建部署、发行等写操作不提供通用幂等键；请求结果不明时先查询，不盲目重复提交。

商品接口按 next_cursor 分页；cloud 列表目前最多返回 100 条，尚无分页，不应假定已取到账号全部资源。

## 错误与版本

401 表示缺少或失效身份，403 表示无权操作，404 表示不存在，409 表示修订/状态冲突或实例额度不足，429 表示限流，5xx 表示服务端不可用。cloud 错误为 `{"error":"CODE"}`；商品和认证接口使用 code/message。客户端应依赖 HTTP 状态与已知 code，忽略响应中的未知字段。

文档版本 1.0.0 描述 HTTP 管理契约；Guard 协议版本 5 描述游戏服运行时链路，两者不是同一版本号。路径沿用已实现的 `/developer/products` 和 `/developer/cloud`，没有另加旧接口别名。未来破坏性变更须显式版本化，并同步服务端与 CLI 测试。

正式服务尚未切换 v5。发布独立 CLI 并不等于部署服务端。
