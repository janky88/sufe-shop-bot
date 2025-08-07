# Telegram Shop Bot - 电商机器人系统

一个功能完整的 Telegram 电商机器人系统，支持商品展示、库存管理、在线支付、多语言、群组推送等功能。

## 功能特性

### 核心功能
- 🛍️ **商品管理**: 支持多类别商品展示，实时库存管理
- 💳 **支付集成**: 集成彩虹易支付，支持多种支付方式
- 🔐 **卡密系统**: 自动发货，支持批量导入卡密
- 💰 **余额系统**: 用户余额充值、消费，充值卡兑换
- 🌐 **多语言支持**: 中文/英文界面，自动语言检测
- 📊 **管理后台**: Web 管理界面，商品/订单/用户管理
- 📢 **消息推送**: 支持用户/群组消息广播，库存更新通知
- 🔄 **失败重试**: 发货失败自动重试机制
- 📈 **监控指标**: Prometheus 指标采集
- 🚀 **高性能**: Redis 缓存，支持 Webhook 模式

### 用户功能
- 商品浏览与搜索
- 在线下单支付
- 订单查询
- 购买历史
- 余额查询与充值
- 多语言切换
- 客服联系

### 管理功能
- 商品上下架管理
- 库存批量导入
- 订单状态管理
- 用户管理
- 消息模板编辑
- 广播消息发送
- 数据统计分析

## 技术架构

- **语言**: Go 1.22+
- **Web框架**: Gin
- **ORM**: GORM
- **数据库**: PostgreSQL/MySQL
- **缓存**: Redis
- **消息队列**: 内置 Channel 实现
- **Bot框架**: telegram-bot-api
- **监控**: Prometheus
- **容器化**: Docker

## 快速开始

### 环境要求

- Go 1.22 或更高版本
- PostgreSQL 12+ 或 MySQL 8+
- Redis 6+ (可选)
- Docker & Docker Compose (用于容器化部署)

### 获取代码

```bash
git clone https://github.com/yourusername/telegram-shop-bot.git
cd telegram-shop-bot
```

### 配置文件

创建 `config.yaml` 配置文件：

```yaml
# Telegram Bot 配置
telegram:
  token: "YOUR_BOT_TOKEN"
  webhook_url: "https://yourdomain.com/webhook"  # Webhook模式使用
  mode: "polling"  # polling 或 webhook

# 数据库配置
database:
  driver: "postgres"  # postgres 或 mysql
  dsn: "host=localhost user=shopbot password=password dbname=shopbot port=5432 sslmode=disable"
  # MySQL DSN 示例: "shopbot:password@tcp(localhost:3306)/shopbot?charset=utf8mb4&parseTime=True&loc=Local"

# Redis 缓存配置（可选）
redis:
  url: "redis://localhost:6379/0"
  # 密码保护: "redis://:password@localhost:6379/0"

# HTTP 服务器配置
server:
  port: 8080
  admin_username: "admin"
  admin_password: "secure_password"

# 彩虹易支付配置
epay:
  api_url: "https://pay.example.com"
  pid: "10001"
  key: "your_secret_key"

# 日志配置
log:
  level: "info"  # debug, info, warn, error
  format: "json" # json 或 text

# 语言配置
language:
  default: "zh"
  supported: ["zh", "en"]

# 消息推送配置
broadcast:
  workers: 10
  rate_limit: 30  # 每秒消息数

# 失败重试配置
retry:
  max_attempts: 3
  initial_delay: "1m"
  max_delay: "1h"
```

### 本地开发部署

1. **安装依赖**

```bash
go mod download
```

2. **初始化数据库**

PostgreSQL:
```sql
CREATE DATABASE shopbot;
```

MySQL:
```sql
CREATE DATABASE shopbot CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

3. **运行程序**

```bash
go run cmd/server/main.go
```

程序会自动创建数据库表结构。

4. **访问管理后台**

打开浏览器访问 `http://localhost:8080/admin`，使用配置文件中的管理员账号登录。

### Docker 部署

#### 使用 Docker Compose（推荐）

1. **创建 docker-compose.yml**

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    container_name: shopbot-db
    environment:
      POSTGRES_DB: shopbot
      POSTGRES_USER: shopbot
      POSTGRES_PASSWORD: shopbot_password
    volumes:
      - postgres_data:/var/lib/postgresql/data
    networks:
      - shopbot-net
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    container_name: shopbot-redis
    command: redis-server --requirepass redis_password
    volumes:
      - redis_data:/data
    networks:
      - shopbot-net
    restart: unless-stopped

  app:
    build: .
    container_name: shopbot-app
    depends_on:
      - postgres
      - redis
    environment:
      - CONFIG_PATH=/app/config.yaml
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./templates:/app/templates
      - ./static:/app/static
    ports:
      - "8080:8080"
    networks:
      - shopbot-net
    restart: unless-stopped

volumes:
  postgres_data:
  redis_data:

networks:
  shopbot-net:
    driver: bridge
```

2. **创建生产环境配置文件 config.yaml**

```yaml
telegram:
  token: "YOUR_BOT_TOKEN"
  webhook_url: "https://yourdomain.com/webhook"
  mode: "webhook"  # 生产环境推荐使用 webhook

database:
  driver: "postgres"
  dsn: "host=postgres user=shopbot password=shopbot_password dbname=shopbot port=5432 sslmode=disable"

redis:
  url: "redis://:redis_password@redis:6379/0"

server:
  port: 8080
  admin_username: "admin"
  admin_password: "your_secure_admin_password"

epay:
  api_url: "https://pay.example.com"
  pid: "10001"
  key: "your_secret_key"

log:
  level: "info"
  format: "json"

language:
  default: "zh"
  supported: ["zh", "en"]

broadcast:
  workers: 20
  rate_limit: 50

retry:
  max_attempts: 5
  initial_delay: "30s"
  max_delay: "1h"
```

3. **启动服务**

```bash
# 构建并启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f app

# 停止服务
docker-compose down

# 停止并删除数据
docker-compose down -v
```

#### 单独使用 Docker

1. **构建镜像**

```bash
docker build -t telegram-shop-bot:latest .
```

2. **运行容器**

```bash
# 创建网络
docker network create shopbot-net

# 运行 PostgreSQL
docker run -d \
  --name shopbot-db \
  --network shopbot-net \
  -e POSTGRES_DB=shopbot \
  -e POSTGRES_USER=shopbot \
  -e POSTGRES_PASSWORD=shopbot_password \
  -v shopbot-postgres:/var/lib/postgresql/data \
  postgres:15-alpine

# 运行 Redis
docker run -d \
  --name shopbot-redis \
  --network shopbot-net \
  -v shopbot-redis:/data \
  redis:7-alpine redis-server --requirepass redis_password

# 运行应用
docker run -d \
  --name shopbot-app \
  --network shopbot-net \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/templates:/app/templates \
  -v $(pwd)/static:/app/static \
  telegram-shop-bot:latest
```

### 生产环境部署

#### 1. 域名与 SSL 配置

使用 Nginx 作为反向代理：

```nginx
server {
    listen 80;
    server_name bot.yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name bot.yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/bot.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/bot.yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /webhook {
        proxy_pass http://localhost:8080/webhook;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

#### 2. Systemd 服务配置

创建 `/etc/systemd/system/shopbot.service`：

```ini
[Unit]
Description=Telegram Shop Bot
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=shopbot
Group=shopbot
WorkingDirectory=/opt/shopbot
ExecStart=/opt/shopbot/shopbot
Restart=always
RestartSec=5
Environment="CONFIG_PATH=/opt/shopbot/config.yaml"

# 安全限制
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/shopbot/logs

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
systemctl daemon-reload
systemctl enable shopbot
systemctl start shopbot
systemctl status shopbot
```

#### 3. 设置 Telegram Webhook

```bash
curl -F "url=https://bot.yourdomain.com/webhook" \
     https://api.telegram.org/bot<YOUR_BOT_TOKEN>/setWebhook
```

验证 Webhook：

```bash
curl https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getWebhookInfo
```

### 数据库备份

#### PostgreSQL 备份

```bash
# 备份
pg_dump -h localhost -U shopbot -d shopbot > backup_$(date +%Y%m%d_%H%M%S).sql

# 恢复
psql -h localhost -U shopbot -d shopbot < backup_20240101_120000.sql
```

#### MySQL 备份

```bash
# 备份
mysqldump -h localhost -u shopbot -p shopbot > backup_$(date +%Y%m%d_%H%M%S).sql

# 恢复
mysql -h localhost -u shopbot -p shopbot < backup_20240101_120000.sql
```

#### 自动备份脚本

创建 `/opt/shopbot/backup.sh`：

```bash
#!/bin/bash
BACKUP_DIR="/opt/shopbot/backups"
DB_NAME="shopbot"
DB_USER="shopbot"
DB_PASS="shopbot_password"
KEEP_DAYS=7

# 创建备份目录
mkdir -p $BACKUP_DIR

# 备份数据库
PGPASSWORD=$DB_PASS pg_dump -h localhost -U $DB_USER -d $DB_NAME | gzip > $BACKUP_DIR/backup_$(date +%Y%m%d_%H%M%S).sql.gz

# 删除旧备份
find $BACKUP_DIR -name "backup_*.sql.gz" -mtime +$KEEP_DAYS -delete
```

添加到 crontab：

```bash
0 2 * * * /opt/shopbot/backup.sh
```

## 监控与维护

### Prometheus 监控

在 `prometheus.yml` 中添加：

```yaml
scrape_configs:
  - job_name: 'shopbot'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

可监控的指标：
- `shopbot_orders_total` - 订单总数
- `shopbot_orders_amount_total` - 订单总金额
- `shopbot_active_users_total` - 活跃用户数
- `shopbot_products_stock_total` - 商品库存总量
- `shopbot_payment_callbacks_total` - 支付回调数
- `shopbot_broadcast_messages_sent_total` - 广播消息发送数

### 日志管理

使用 logrotate 管理日志：

创建 `/etc/logrotate.d/shopbot`：

```
/opt/shopbot/logs/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0644 shopbot shopbot
    sharedscripts
    postrotate
        systemctl reload shopbot
    endscript
}
```

### 性能优化

1. **数据库优化**
   - 为常用查询字段添加索引
   - 定期执行 VACUUM（PostgreSQL）
   - 优化查询语句

2. **Redis 缓存策略**
   - 商品信息缓存 10 分钟
   - 用户信息缓存 5 分钟
   - 热门商品永久缓存，手动失效

3. **并发优化**
   - 使用 Webhook 模式减少轮询开销
   - 合理设置广播 worker 数量
   - 使用数据库连接池

## 常见问题

### 1. Bot 无响应

检查事项：
- Bot Token 是否正确
- 网络是否可以访问 Telegram API
- 查看日志是否有错误信息

### 2. 支付回调失败

检查事项：
- 回调 URL 是否可以从外网访问
- 签名密钥是否正确
- 查看支付平台的回调日志

### 3. 数据库连接失败

检查事项：
- 数据库服务是否运行
- 连接字符串是否正确
- 防火墙是否允许连接

### 4. 消息发送失败

可能原因：
- 用户屏蔽了 Bot
- 发送频率过快被限制
- 网络连接问题

## 开发指南

### 项目结构

```
├── cmd/
│   └── server/          # 程序入口
├── internal/
│   ├── app/            # 应用程序容器
│   ├── bot/            # Telegram Bot 逻辑
│   ├── broadcast/      # 广播服务
│   ├── cache/          # Redis 缓存
│   ├── config/         # 配置管理
│   ├── epay/           # 支付集成
│   ├── httpadmin/      # Web 管理后台
│   ├── i18n/           # 国际化
│   ├── store/          # 数据存储层
│   └── worker/         # 后台任务
├── templates/          # HTML 模板
├── static/            # 静态资源
├── migrations/        # 数据库迁移
├── docker/           # Docker 相关文件
├── config.yaml       # 配置文件
├── Dockerfile        # Docker 镜像定义
├── docker-compose.yml # Docker Compose 配置
└── README.md         # 本文档
```

### 添加新功能

1. **添加新的数据模型**

在 `internal/store/models.go` 中定义模型：

```go
type YourModel struct {
    ID        uint      `gorm:"primaryKey"`
    // 字段定义
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

2. **添加新的 Bot 命令**

在 `internal/bot/handlers.go` 中添加处理器：

```go
func (b *Bot) handleYourCommand(ctx context.Context, msg *tgbotapi.Message) {
    // 命令逻辑
}
```

3. **添加新的管理页面**

在 `internal/httpadmin/handlers.go` 中添加路由：

```go
func (s *Server) handleYourPage(c *gin.Context) {
    // 页面逻辑
}
```

### 测试

运行测试：

```bash
go test ./...
```

运行特定测试：

```bash
go test -v ./internal/store -run TestYourFunction
```

## 安全建议

1. **定期更新依赖**

```bash
go get -u ./...
go mod tidy
```

2. **使用强密码**
   - 管理后台密码
   - 数据库密码
   - Redis 密码

3. **限制访问**
   - 使用防火墙限制数据库访问
   - 管理后台使用 IP 白名单
   - 启用 Telegram Bot 的域名白名单

4. **数据加密**
   - 使用 HTTPS
   - 敏感数据加密存储
   - 定期备份并加密

## 贡献指南

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

## 联系方式

- 项目主页: [https://github.com/yourusername/telegram-shop-bot](https://github.com/yourusername/telegram-shop-bot)
- 问题反馈: [https://github.com/yourusername/telegram-shop-bot/issues](https://github.com/yourusername/telegram-shop-bot/issues)

## 致谢

- [Telegram Bot API](https://core.telegram.org/bots/api)
- [Gin Web Framework](https://gin-gonic.com/)
- [GORM](https://gorm.io/)
- 所有贡献者