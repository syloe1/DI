# Go-Admin API 测试文档

## 项目概述
- **服务器地址**: `http://localhost:19999`
- **认证方式**: JWT Token (Bearer Token)
- **数据库**: MySQL + Redis

## 环境准备

### 1. 启动服务
```bash
cd f:\go
go run main.go
```

### 2. 检查服务状态
```bash
# 测试基础连接
curl http://localhost:19999/
```

## API 测试流程

### 第一阶段：用户认证测试

#### 1.1 用户注册
- **方法**: POST
- **URL**: `http://localhost:19999/user/register`
- **Headers**: `Content-Type: application/json`
- **Body**:
```json
{
  "username": "testuser1",
  "password": "123456"
}
```

#### 1.2 用户登录
- **方法**: POST
- **URL**: `http://localhost:19999/user/login`
- **Headers**: `Content-Type: application/json`
- **Body**:
```json
{
  "username": "testuser1",
  "password": "123456"
}
```
- **保存Token**: 将响应中的 `data.token` 保存为环境变量 `{{token}}`

#### 1.3 获取用户信息（公开接口）
- **方法**: GET
- **URL**: `http://localhost:19999/user/1`

### 第二阶段：帖子功能测试

#### 2.1 创建帖子
- **方法**: POST
- **URL**: `http://localhost:19999/auth/post/create`
- **Headers**: 
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`
- **Body**:
```json
{
  "title": "测试帖子标题",
  "content": "这是测试帖子的内容 #测试 #示例",
  "is_public": true
}
```

#### 2.2 获取帖子列表（公开）
- **方法**: GET
- **URL**: `http://localhost:19999/post/list`

#### 2.3 获取单个帖子
- **方法**: GET
- **URL**: `http://localhost:19999/post/1`

#### 2.4 获取用户帖子
- **方法**: GET
- **URL**: `http://localhost:19999/post/user/1`

#### 2.5 获取我的帖子（需要认证）
- **方法**: GET
- **URL**: `http://localhost:19999/auth/post/my`
- **Headers**: `Authorization: Bearer {{token}}`

### 第三阶段：评论功能测试

#### 3.1 创建评论
- **方法**: POST
- **URL**: `http://localhost:19999/auth/comment/create`
- **Headers**: 
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`
- **Body**:
```json
{
  "post_id": 1,
  "content": "这是一条测试评论"
}
```

#### 3.2 获取帖子评论
- **方法**: GET
- **URL**: `http://localhost:19999/comment/post/1`

### 第四阶段：互动功能测试

#### 4.1 点赞帖子
- **方法**: POST
- **URL**: `http://localhost:19999/auth/interact/like/1`
- **Headers**: `Authorization: Bearer {{token}}`

#### 4.2 点踩帖子
- **方法**: POST
- **URL**: `http://localhost:19999/auth/interact/dislike/1`
- **Headers**: `Authorization: Bearer {{token}}`

#### 4.3 收藏帖子
- **方法**: POST
- **URL**: `http://localhost:19999/auth/interact/collect/1`
- **Headers**: `Authorization: Bearer {{token}}`

#### 4.4 获取互动状态
- **方法**: GET
- **URL**: `http://localhost:19999/auth/interact/status/1`
- **Headers**: `Authorization: Bearer {{token}}`

#### 4.5 获取互动统计
- **方法**: GET
- **URL**: `http://localhost:19999/interact/count/1`

### 第五阶段：社交功能测试

#### 5.1 关注用户
- **方法**: POST
- **URL**: `http://localhost:19999/auth/social/follow/2`
- **Headers**: `Authorization: Bearer {{token}}`

#### 5.2 获取关系状态
- **方法**: GET
- **URL**: `http://localhost:19999/auth/social/relation/2`
- **Headers**: `Authorization: Bearer {{token}}`

#### 5.3 获取关注列表
- **方法**: GET
- **URL**: `http://localhost:19999/auth/social/follows`
- **Headers**: `Authorization: Bearer {{token}}`

#### 5.4 获取粉丝列表
- **方法**: GET
- **URL**: `http://localhost:19999/auth/social/followers`
- **Headers**: `Authorization: Bearer {{token}}`

### 第六阶段：私聊功能测试

#### 6.1 发送私聊消息
- **方法**: POST
- **URL**: `http://localhost:19999/auth/message/send`
- **Headers**: 
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`
- **Body**:
```json
{
  "to_uid": 2,
  "content": "你好，这是一条测试消息"
}
```

#### 6.2 获取会话列表
- **方法**: GET
- **URL**: `http://localhost:19999/auth/message/conversations`
- **Headers**: `Authorization: Bearer {{token}}`

#### 6.3 获取消息列表
- **方法**: GET
- **URL**: `http://localhost:19999/auth/message/list?target=2`
- **Headers**: `Authorization: Bearer {{token}}`

### 第七阶段：搜索功能测试

#### 7.1 搜索用户
- **方法**: GET
- **URL**: `http://localhost:19999/user/search?username=test`

#### 7.2 获取用户点赞帖子
- **方法**: GET
- **URL**: `http://localhost:19999/post/user/1/liked`

#### 7.3 获取用户收藏帖子
- **方法**: GET
- **URL**: `http://localhost:19999/post/user/1/collected`

### 第八阶段：管理员功能测试

#### 8.1 获取用户列表（管理员）
- **方法**: GET
- **URL**: `http://localhost:19999/auth/user/list`
- **Headers**: `Authorization: Bearer {{token}}`

#### 8.2 添加用户（管理员）
- **方法**: POST
- **URL**: `http://localhost:19999/auth/user/add`
- **Headers**: 
  - `Content-Type: application/json`
  - `Authorization: Bearer {{token}}`
- **Body**:
```json
{
  "username": "adminuser",
  "password": "123456",
  "email": "admin@example.com",
  "role": "user"
}
```

## Postman 环境变量设置

### 全局变量
```javascript
{
  "base_url": "http://localhost:19999",
  "token": "从登录响应中获取的JWT token"
}
```

### 测试脚本示例

#### 登录后自动设置token
```javascript
// 在登录请求的Tests标签中添加
if (pm.response.code === 200) {
    var jsonData = pm.response.json();
    pm.environment.set("token", jsonData.data.token);
    pm.environment.set("user_id", jsonData.data.user.id);
}
```

#### 请求头自动设置
在Collection的Pre-request Script中添加：
```javascript
if (pm.environment.get("token")) {
    pm.request.headers.add({
        key: "Authorization",
        value: "Bearer " + pm.environment.get("token")
    });
}
```

## 测试数据准备

### 创建测试用户
1. 注册用户A: testuser1 / 123456
2. 注册用户B: testuser2 / 123456
3. 分别登录获取token

### 创建测试帖子
1. 使用用户A创建3-5个测试帖子
2. 确保部分帖子包含话题标签（如 #测试 #示例）

## 常见问题排查

### 1. 404错误
- 检查服务器是否启动：`netstat -ano | findstr :19999`
- 检查URL路径是否正确
- 检查端口配置：`config/config.yaml`

### 2. 401认证错误
- 检查token是否过期
- 重新登录获取新token
- 检查Authorization头格式：`Bearer {{token}}`

### 3. 数据库连接错误
- 检查MySQL服务是否启动
- 检查Redis服务是否启动
- 检查数据库配置：用户名、密码、端口

### 4. 参数错误
- 检查请求体JSON格式
- 检查必填字段是否提供
- 检查参数类型是否正确

## 性能测试建议

### 并发测试
- 使用Postman的Collection Runner进行并发测试
- 设置不同的虚拟用户数量
- 监控响应时间和错误率

### 压力测试
- 测试高并发下的用户注册
- 测试大量帖子创建和查询
- 测试消息发送的并发性能

## 自动化测试

### Newman命令行测试
```bash
# 安装Newman
npm install -g newman

# 运行测试集合
newman run collection.json -e environment.json
```

### 持续集成
- 将API测试集成到CI/CD流程中
- 设置自动化测试报告
- 监控API性能和稳定性

## 测试报告

每次测试完成后，建议记录：
- 测试时间
- 测试环境
- 通过/失败的接口数量
- 发现的bug和问题
- 性能指标（响应时间、吞吐量）

---

**注意**: 在实际测试前，请确保数据库和Redis服务已正确启动，并且配置文件中的连接信息正确。