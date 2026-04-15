# Go-Admin 社交平台后端项目

## 项目简介

Go-Admin 是一个基于 Go 语言开发的完整社交平台后端系统，采用 Gin 框架构建，提供完整的用户管理、内容发布、社交互动和实时聊天功能。

## 技术栈

- **后端框架**: Gin (Go)
- **数据库**: MySQL
- **缓存**: Redis
- **认证**: JWT Token
- **实时通信**: WebSocket
- **API文档**: 支持 Postman 测试

## 功能特性

### 🔐 用户认证系统
- 用户注册与登录
- JWT Token 认证
- 密码加密存储
- 角色权限管理 (用户/管理员/超级管理员)

### 📝 内容管理系统
- 帖子发布、编辑、删除
- 帖子可见性控制 (公开/私有)
- 话题标签提取 (#标签)
- 定时发布功能

### 💬 评论与互动
- 帖子评论功能
- 点赞/点踩系统
- 收藏功能
- 分享统计

### 👥 社交关系
- 用户关注/取消关注
- 粉丝系统
- 用户拉黑功能
- 关系状态查询

### 💌 实时聊天
- 私聊消息发送
- 会话列表管理
- 消息已读状态
- 在线状态实时更新

### 🔍 搜索与发现
- 用户搜索 (按用户名)
- 用户帖子浏览
- 用户点赞/收藏内容查询
- 话题筛选

### ⚙️ 管理员功能
- 用户列表管理
- 用户信息修改
- 用户角色分配
- 用户删除操作

## 项目结构

```
go-admin/
├── api/           # API 控制器
│   ├── user.go    # 用户相关API
│   ├── post.go    # 帖子相关API
│   ├── comment.go # 评论相关API
│   ├── interact.go # 互动相关API
│   ├── social.go  # 社交相关API
│   ├── message.go # 消息相关API
│   └── ws.go      # WebSocket处理
├── config/        # 配置文件
│   ├── config.go  # 配置加载
│   └── config.yaml # 应用配置
├── core/          # 核心组件
│   ├── mysql.go   # 数据库连接
│   └── redis.go   # Redis连接
├── middleware/    # 中间件
│   ├── jwt.go     # JWT认证
│   ├── cors.go    # 跨域处理
│   └── admin.go   # 管理员权限
├── model/         # 数据模型
│   ├── user.go    # 用户模型
│   ├── post.go    # 帖子模型
│   ├── comment.go # 评论模型
│   ├── message.go # 消息模型
│   └── social.go  # 社交关系模型
├── router/        # 路由配置
│   └── router.go  # 路由定义
├── utils/         # 工具类
│   └── snowflake.go # 雪花算法ID生成
├── main.go        # 应用入口
└── test.md        # API测试文档
```

## 快速开始

### 环境要求

- Go 1.16+
- MySQL 5.7+
- Redis 5.0+

### 安装步骤

1. **克隆项目**
```bash
git clone <项目地址>
cd go-admin
```

2. **配置数据库**
修改 `config/config.yaml` 文件：
```yaml
mysql:
  host: 127.0.0.1
  port: 3307
  dbname: go
  username: root
  password: 123456

redis:
  host: 127.0.0.1
  port: 6379
  password: ""
  db: 0
```

3. **安装依赖**
```bash
go mod tidy
```

4. **启动服务**
```bash
go run main.go
```

服务将在 `http://localhost:19999` 启动

## API 文档

详细的 API 接口文档和测试流程请参考 [test.md](test.md) 文件。

### 主要 API 端点

#### 公开接口 (无需认证)
- `POST /user/register` - 用户注册
- `POST /user/login` - 用户登录
- `GET /post/list` - 获取帖子列表
- `GET /post/{id}` - 获取单个帖子
- `GET /user/search?username={keyword}` - 搜索用户

#### 认证接口 (需要 JWT Token)
- `POST /auth/post/create` - 创建帖子
- `POST /auth/comment/create` - 创建评论
- `POST /auth/interact/like/{post_id}` - 点赞帖子
- `POST /auth/social/follow/{uid}` - 关注用户
- `POST /auth/message/send` - 发送私聊消息

#### 管理员接口
- `GET /auth/user/list` - 获取用户列表
- `POST /auth/user/add` - 添加用户
- `DELETE /auth/user/{id}` - 删除用户

## 数据库设计

### 主要数据表

- **users** - 用户表
- **posts** - 帖子表
- **comments** - 评论表
- **messages** - 私聊消息表
- **likes/dislikes/collects** - 互动表
- **user_relations** - 用户关系表

### 自动迁移
项目启动时会自动创建所有需要的数据库表：
```go
core.DB.AutoMigrate(&model.User{})
core.DB.AutoMigrate(&model.Post{})
core.DB.AutoMigrate(&model.Comment{})
core.DB.AutoMigrate(&model.Like{})
// ... 其他表
```

## 功能亮点

### 1. 高性能缓存
- Redis 缓存用户信息和帖子列表
- 缓存失效时间自动管理
- 数据库查询优化

### 2. 实时通信
- WebSocket 实现实时消息推送
- 在线用户状态实时更新
- 消息已读状态同步

### 3. 权限控制
- 基于角色的权限管理
- JWT Token 认证机制
- 细粒度的操作权限控制

### 4. 内容安全
- 输入参数验证
- SQL 注入防护
- XSS 攻击防护

## 部署说明

### 开发环境
```bash
# 直接运行
go run main.go

# 或编译后运行
go build -o app.exe
./app.exe
```

### 生产环境
1. 修改配置文件中的数据库连接信息
2. 设置合适的 JWT Secret
3. 配置反向代理 (Nginx)
4. 设置进程守护 (systemd/supervisor)

## 测试

项目包含完整的 API 测试文档，支持：
- Postman 手动测试
- Newman 命令行测试
- 并发性能测试

参考 [test.md](test.md) 文件进行详细测试。

## 贡献指南

1. Fork 本项目
2. 创建功能分支
3. 提交代码变更
4. 创建 Pull Request

## 许可证

MIT License

## 联系方式

如有问题或建议，请通过以下方式联系：
- 项目 Issues
- 邮箱: [your-email@example.com]

---

**项目状态**: ✅ 功能完整，已通过全面测试  
**最后更新**: 2024年  
**版本**: v1.0.0