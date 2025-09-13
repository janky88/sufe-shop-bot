# Shop Bot 部署指南

## 目录
1. [系统要求](#系统要求)
2. [快速开始](#快速开始)
3. [配置说明](#配置说明)
4. [部署方式](#部署方式)
5. [管理员设置](#管理员设置)
6. [故障排除](#故障排除)

## 系统要求

- Docker 20.10+
- Docker Compose 2.0+
- 2GB+ RAM
- 10GB+ 磁盘空间
- Linux/macOS/Windows (with WSL2)

## 快速开始

### 1. 克隆项目
```bash
git clone https://github.com/Shannon-x/sufe-shop-bot.git
cd sufe-shop-bot
```

### 2. 准备配置文件
```bash
# 复制环境变量模板
cp .env.production .env

# 编辑配置文件
nano .env
```

**必须配置的项目：**
- `BOT_TOKEN` - Telegram机器人令牌（从 @BotFather 获取）
- `ADMIN_TOKEN` - 管理面板访问密码（请使用强密码）
- `ADMIN_TELEGRAM_IDS` - 管理员的Telegram ID（重要！）
- `BASE_URL` - 您的域名（例如：https://bot.example.com）

### 3. 获取您的Telegram ID
发送消息给 [@userinfobot](https://t.me/userinfobot) 获取您的Telegram ID

### 4. 启动服务

#### 方式一：完整部署（包含数据库）
```bash
# 使用完整的docker-compose文件
docker-compose -f docker-compose.full.yml up -d
```

#### 方式二：1Panel部署（使用外部数据库）
```bash
# 使用默认的docker-compose文件
docker-compose up -d
```

### 5. 验证部署
```bash
# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f app
```

## 配置说明

### 核心配置

| 配置项 | 说明 | 示例 |
|--------|------|------|
| BOT_TOKEN | Telegram机器人令牌 | 123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11 |
| ADMIN_TOKEN | 管理面板密码 | your_very_secure_password_here |
| ADMIN_TELEGRAM_IDS | 管理员Telegram ID列表 | 123456789,987654321 |
| BASE_URL | 网站域名 | https://bot.example.com |

### 数据库配置

#### SQLite（默认）
```env
DB_TYPE=sqlite
DB_NAME=shop.db
```

#### PostgreSQL（推荐生产环境）
```env
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
DB_NAME=shopbot
DB_USER=shopbot
DB_PASSWORD=your_password
```

### 支付配置（可选）
```env
EPAY_PID=your_merchant_id
EPAY_KEY=your_secret_key
EPAY_GATEWAY=https://pay.gateway.com
```

## 部署方式

### 1. Docker Compose 完整部署

使用 `docker-compose.full.yml` 文件，包含：
- PostgreSQL 数据库
- Redis 缓存
- Shop Bot 应用

```bash
docker-compose -f docker-compose.full.yml up -d
```

### 2. 1Panel 部署

1. 在1Panel中创建PostgreSQL数据库
2. 配置 `.env` 文件中的数据库连接信息
3. 使用默认的 `docker-compose.yml` 文件部署

### 3. 独立部署

如果您已有数据库和Redis：
```bash
docker-compose -f docker-compose.simple.yml up -d
```

## 管理员设置

### 自动初始化管理员

系统会根据 `ADMIN_TELEGRAM_IDS` 配置自动创建管理员账户：

1. 在 `.env` 文件中设置您的Telegram ID：
   ```env
   ADMIN_TELEGRAM_IDS=123456789
   ```

2. 启动应用后，系统会自动：
   - 创建管理员账户
   - 设置接收通知权限
   - 配置管理面板访问权限

3. 访问管理面板：
   ```
   https://your-domain.com/admin/
   ```
   使用 `ADMIN_TOKEN` 作为密码登录

### 添加多个管理员

在 `ADMIN_TELEGRAM_IDS` 中使用逗号分隔多个ID：
```env
ADMIN_TELEGRAM_IDS=123456789,987654321,555666777
```

## 功能验证

### 1. 测试机器人
- 在Telegram中搜索您的机器人
- 发送 `/start` 命令
- 应该收到欢迎消息

### 2. 测试管理面板
- 访问 `https://your-domain.com/admin/`
- 使用 `ADMIN_TOKEN` 登录
- 检查各项功能是否正常

### 3. 测试支付（如已配置）
- 创建测试商品
- 尝试购买流程
- 检查支付回调

## 故障排除

### 常见问题

#### 1. 机器人不响应
- 检查 `BOT_TOKEN` 是否正确
- 查看日志：`docker-compose logs app`
- 确认防火墙允许出站HTTPS连接

#### 2. 数据库连接失败
- 检查数据库服务是否运行
- 验证数据库连接信息
- 查看数据库日志

#### 3. 管理面板无法访问
- 检查端口9147是否开放
- 验证 `ADMIN_TOKEN` 设置
- 查看nginx/反向代理配置

#### 4. 支付功能不工作
- 验证 `BASE_URL` 配置正确
- 检查支付网关配置
- 确认回调URL可访问

### 查看日志
```bash
# 查看所有服务日志
docker-compose logs

# 查看应用日志
docker-compose logs app

# 实时查看日志
docker-compose logs -f app

# 查看最后100行
docker-compose logs --tail=100 app
```

### 重启服务
```bash
# 重启所有服务
docker-compose restart

# 重启单个服务
docker-compose restart app
```

### 更新应用
```bash
# 拉取最新代码
git pull

# 重新构建并启动
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

## 备份与恢复

### 备份数据库

#### SQLite
```bash
cp data/shop.db backup/shop_$(date +%Y%m%d).db
```

#### PostgreSQL
```bash
docker-compose exec postgres pg_dump -U shopbot shopbot > backup/shopbot_$(date +%Y%m%d).sql
```

### 恢复数据库

#### SQLite
```bash
cp backup/shop_20240101.db data/shop.db
docker-compose restart app
```

#### PostgreSQL
```bash
docker-compose exec -T postgres psql -U shopbot shopbot < backup/shopbot_20240101.sql
```

## 安全建议

1. **使用强密码**
   - `ADMIN_TOKEN` 至少16个字符
   - 数据库密码使用随机生成

2. **限制访问**
   - 使用防火墙限制数据库端口
   - 配置nginx限制管理面板访问

3. **定期备份**
   - 设置自动备份任务
   - 测试恢复流程

4. **监控日志**
   - 定期检查异常登录
   - 监控支付异常

5. **及时更新**
   - 关注项目更新
   - 定期更新依赖

## 支持

如遇到问题，请：
1. 查看[故障排除](#故障排除)部分
2. 查看项目 [Issues](https://github.com/Shannon-x/sufe-shop-bot/issues)
3. 提交新的 Issue

---

祝您使用愉快！🚀