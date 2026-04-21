# go-admin API 测试文档

## 项目概览

- 服务地址：`http://localhost:19999`
- 认证方式：JWT Bearer Token
- 存储组件：MySQL + Redis
- 启动入口：`cmd/server/main.go`

## 启动项目

默认命令都在仓库根目录执行。

启动服务：

```bash
go run ./cmd/server
```

检查是否可编译：

```bash
go build ./cmd/server
```

构建并运行可执行文件：

```bash
go build -o a.exe ./cmd/server
./a.exe
```

## 认证说明

登录成功后，把返回的 token 放到请求头：

```text
Authorization: Bearer <token>
```

建议在 Postman 中配置变量：

```json
{
  "base_url": "http://localhost:19999",
  "token": ""
}
```

## 推荐测试流程

建议按下面顺序测试：

1. 用户注册 / 登录
2. 用户查询
3. 帖子创建 / 查询 / 修改 / 删除
4. 评论创建 / 查询 / 修改 / 删除
5. 互动功能
6. 社交关系
7. 私信功能
8. 管理员功能
9. WebSocket

## 一、用户模块

### 1. 用户注册

- 方法：`POST`
- URL：`{{base_url}}/user/register`
- Headers：
  - `Content-Type: application/json`

请求体：

```json
{
  "username": "testuser1",
  "password": "123456"
}
```

### 2. 用户登录

- 方法：`POST`
- URL：`{{base_url}}/user/login`
- Headers：
  - `Content-Type: application/json`

请求体：

```json
{
  "username": "testuser1",
  "password": "123456"
}
```

说明：

- 登录成功后，把返回结果中的 `data.token` 保存为 `{{token}}`

### 3. 获取用户信息

- 方法：`GET`
- URL：`{{base_url}}/user/1`

### 4. 搜索用户

- 方法：`GET`
- URL：`{{base_url}}/user/search?username=test`

### 5. 获取在线用户列表

- 方法：`GET`
- URL：`{{base_url}}/user/online/list`

### 6. 获取用户在线状态

- 方法：`GET`
- URL：`{{base_url}}/user/1/online`

### 7. 获取用户列表

- 方法：`GET`
- URL：`{{base_url}}/auth/user/list`
- Headers：
  - `Authorization: Bearer {{token}}`

### 8. 获取认证态下的用户信息

- 方法：`GET`
- URL：`{{base_url}}/auth/user/1`
- Headers：
  - `Authorization: Bearer {{token}}`

### 9. 退出登录

- 方法：`POST`
- URL：`{{base_url}}/auth/user/logout`
- Headers：
  - `Authorization: Bearer {{token}}`

### 10. 批量查询用户角色

- 方法：`POST`
- URL：`{{base_url}}/auth/user/batch-roles`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体：

```json
{
  "ids": [1, 2, 3]
}
```

### 11. 更新用户

- 方法：`PUT`
- URL：`{{base_url}}/auth/user/1`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体示例：

```json
{
  "username": "newname",
  "role": "user"
}
```

### 12. 修改密码

- 方法：`PUT`
- URL：`{{base_url}}/auth/user/password/1`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体：

```json
{
  "oldPassword": "123456",
  "newPassword": "654321"
}
```

### 13. 管理员添加用户

- 方法：`POST`
- URL：`{{base_url}}/auth/user/add`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体：

```json
{
  "username": "adminuser",
  "password": "123456",
  "role": "user"
}
```

说明：

- 该接口需要管理员权限
- 当前版本 `AddUserRequest` 没有 `email` 字段

### 14. 管理员删除用户

- 方法：`DELETE`
- URL：`{{base_url}}/auth/user/2`
- Headers：
  - `Authorization: Bearer {{token}}`

## 二、帖子模块

### 1. 创建帖子

- 方法：`POST`
- URL：`{{base_url}}/auth/post/create`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体：

```json
{
  "title": "测试帖子标题",
  "content": "这是测试帖子的正文内容",
  "is_public": true,
  "status": "published",
  "topics": "#测试 #示例",
  "images": ""
}
```

可选字段说明：

- `status`：`published` / `draft` / `scheduled`
- `publish_at`：定时发布时使用，格式如 `2026-04-21T12:00:00+08:00`

### 2. 获取帖子列表

- 方法：`GET`
- URL：`{{base_url}}/post/list`

可选查询参数：

- `topic`
- `sort=time|hot`
- `page`
- `page_size`

示例：

```text
{{base_url}}/post/list?sort=hot&page=1&page_size=10
```

### 3. 获取热门帖子

- 方法：`GET`
- URL：`{{base_url}}/post/hot?limit=10`

### 4. 获取帖子详情

- 方法：`GET`
- URL：`{{base_url}}/post/1`

### 5. 获取某个用户的帖子

- 方法：`GET`
- URL：`{{base_url}}/post/user/1`

### 6. 获取用户点赞过的帖子

- 方法：`GET`
- URL：`{{base_url}}/post/user/1/liked`

### 7. 获取用户收藏过的帖子

- 方法：`GET`
- URL：`{{base_url}}/post/user/1/collected`

### 8. 获取我的帖子

- 方法：`GET`
- URL：`{{base_url}}/auth/post/my`
- Headers：
  - `Authorization: Bearer {{token}}`

### 9. 更新帖子

- 方法：`PUT`
- URL：`{{base_url}}/auth/post/1`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体示例：

```json
{
  "title": "更新后的标题",
  "content": "更新后的正文",
  "is_public": true,
  "status": "published",
  "topics": "#更新",
  "images": ""
}
```

### 10. 删除帖子

- 方法：`DELETE`
- URL：`{{base_url}}/auth/post/1`
- Headers：
  - `Authorization: Bearer {{token}}`

## 三、评论模块

### 1. 创建评论

- 方法：`POST`
- URL：`{{base_url}}/auth/comment/create`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体：

```json
{
  "post_id": 1,
  "content": "这是一条测试评论",
  "parent_id": 0
}
```

### 2. 获取帖子评论列表

- 方法：`GET`
- URL：`{{base_url}}/comment/post/1`

### 3. 获取我的评论

- 方法：`GET`
- URL：`{{base_url}}/auth/comment/my`
- Headers：
  - `Authorization: Bearer {{token}}`

### 4. 更新评论

- 方法：`PUT`
- URL：`{{base_url}}/auth/comment/1`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体：

```json
{
  "content": "修改后的评论内容"
}
```

### 5. 删除评论

- 方法：`DELETE`
- URL：`{{base_url}}/auth/comment/1`
- Headers：
  - `Authorization: Bearer {{token}}`

## 四、互动模块

### 1. 点赞 / 取消点赞

- 方法：`POST`
- URL：`{{base_url}}/auth/interact/like/1`
- Headers：
  - `Authorization: Bearer {{token}}`

### 2. 点踩 / 取消点踩

- 方法：`POST`
- URL：`{{base_url}}/auth/interact/dislike/1`
- Headers：
  - `Authorization: Bearer {{token}}`

### 3. 收藏 / 取消收藏

- 方法：`POST`
- URL：`{{base_url}}/auth/interact/collect/1`
- Headers：
  - `Authorization: Bearer {{token}}`

### 4. 分享帖子

- 方法：`POST`
- URL：`{{base_url}}/auth/interact/share/1`
- Headers：
  - `Authorization: Bearer {{token}}`

### 5. 获取互动状态

- 方法：`GET`
- URL：`{{base_url}}/auth/interact/status/1`
- Headers：
  - `Authorization: Bearer {{token}}`

### 6. 获取互动统计

- 方法：`GET`
- URL：`{{base_url}}/interact/count/1`

## 五、社交模块

### 1. 关注 / 取消关注用户

- 方法：`POST`
- URL：`{{base_url}}/auth/social/follow/2`
- Headers：
  - `Authorization: Bearer {{token}}`

### 2. 拉黑 / 取消拉黑用户

- 方法：`POST`
- URL：`{{base_url}}/auth/social/block/2`
- Headers：
  - `Authorization: Bearer {{token}}`

### 3. 获取关系状态

- 方法：`GET`
- URL：`{{base_url}}/auth/social/relation/2`
- Headers：
  - `Authorization: Bearer {{token}}`

### 4. 获取关注列表

- 方法：`GET`
- URL：`{{base_url}}/auth/social/follows`
- Headers：
  - `Authorization: Bearer {{token}}`

### 5. 获取粉丝列表

- 方法：`GET`
- URL：`{{base_url}}/auth/social/followers`
- Headers：
  - `Authorization: Bearer {{token}}`

### 6. 获取拉黑列表

- 方法：`GET`
- URL：`{{base_url}}/auth/social/blocks`
- Headers：
  - `Authorization: Bearer {{token}}`

## 六、私信模块

### 1. 发送消息

- 方法：`POST`
- URL：`{{base_url}}/auth/message/send`
- Headers：
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`

请求体：

```json
{
  "to_uid": 2,
  "content": "你好，这是一条测试消息"
}
```

### 2. 获取会话列表

- 方法：`GET`
- URL：`{{base_url}}/auth/message/conversations`
- Headers：
  - `Authorization: Bearer {{token}}`

### 3. 获取消息列表

- 方法：`GET`
- URL：`{{base_url}}/auth/message/list?peer_id=2&page=1&page_size=20`
- Headers：
  - `Authorization: Bearer {{token}}`

说明：

- 当前版本查询参数名是 `peer_id`
- 旧文档里的 `target` 已经不适用

### 4. 删除消息

- 方法：`DELETE`
- URL：`{{base_url}}/auth/message/1`
- Headers：
  - `Authorization: Bearer {{token}}`

## 七、WebSocket

### 1. WebSocket 连接入口

- 方法：`GET`
- URL：`ws://localhost:19999/ws?token=<jwt_token>`

说明：

- 当前 WebSocket 通过 query 参数中的 `token` 认证
- 建议先调用 `/user/login` 获取 JWT，再建立 WS 连接

## 测试数据建议

### 用户

建议至少准备两个普通用户：

1. `testuser1 / 123456`
2. `testuser2 / 123456`

如果要测试管理员接口，再准备一个管理员账号。

### 帖子

建议使用用户 A 创建 3 到 5 条帖子，覆盖这些情况：

- 公开帖子
- 草稿帖子
- 带 `topics` 的帖子
- 带图片字段的帖子

### 评论 / 社交 / 私信

建议使用用户 A 和用户 B 互相操作，方便验证：

- 关注关系
- 点赞收藏
- 私信会话
- 评论归属

## Postman 脚本示例

### 登录后自动保存 token

```javascript
if (pm.response.code === 200) {
    const jsonData = pm.response.json();
    pm.environment.set("token", jsonData.data.token);
}
```

### 自动注入 Authorization 头

```javascript
if (pm.environment.get("token")) {
    pm.request.headers.upsert({
        key: "Authorization",
        value: "Bearer " + pm.environment.get("token")
    });
}
```

## 常见问题排查

### 1. 服务启动失败

优先检查：

- `config/config.yaml` 是否存在
- MySQL 是否已启动
- Redis 是否已启动
- 端口 `19999` 是否被占用

### 2. 404

优先检查：

- 服务是否已启动
- 请求方法是否正确
- URL 是否和当前路由一致

当前启动命令应为：

```bash
go run ./cmd/server
```

不是旧版本的 `go run main.go`

### 3. 401 / 403

优先检查：

- `Authorization` 头是否正确
- token 是否过期
- 是否访问了管理员接口

格式必须是：

```text
Authorization: Bearer <token>
```

### 4. 参数绑定失败

优先检查：

- JSON 字段名是否正确
- 查询参数名是否和 DTO 一致
- 是否漏传必填字段

例如：

- 消息列表用 `peer_id`，不是 `target`
- 修改密码用 `oldPassword` 和 `newPassword`

## 当前版本重点变更提醒

和旧测试文档相比，当前版本需要注意：

- 启动入口改为 `cmd/server`
- 新增了 `GET /post/hot`
- 新增了在线用户相关接口
- 新增了 `POST /auth/user/logout`
- 新增了 `DELETE /auth/message/:id`
- 消息列表参数改为 `peer_id`
- 管理员加用户请求体不再包含 `email`

## 维护建议

后面继续维护这份测试文档时，建议遵循两条：

1. 路由变化时，先改 `internal/router/router.go`，再同步改这里
2. DTO 变化时，顺手同步更新这里的请求体示例
