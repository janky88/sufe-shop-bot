#!/bin/bash

# Shop Bot 快速部署脚本
# 使用方法: ./deploy.sh

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查命令是否存在
check_command() {
    if ! command -v $1 &> /dev/null; then
        print_error "$1 未安装，请先安装 $1"
        exit 1
    fi
}

# 显示欢迎信息
echo "======================================"
echo "   Shop Bot 快速部署脚本"
echo "======================================"
echo ""

# 检查依赖
print_info "检查系统依赖..."
check_command docker
check_command docker-compose
print_success "依赖检查通过"

# 检查.env文件
if [ ! -f ".env" ]; then
    print_warning ".env 文件不存在，正在从模板创建..."
    if [ -f ".env.production" ]; then
        cp .env.production .env
        print_success "已创建 .env 文件，请编辑配置"
    else
        print_error "未找到 .env.production 模板文件"
        exit 1
    fi
fi

# 获取用户输入
echo ""
print_info "请准备以下信息："
echo "1. Telegram Bot Token (从 @BotFather 获取)"
echo "2. 管理员密码 (用于访问管理面板)"
echo "3. 您的 Telegram ID (发送消息给 @userinfobot 获取)"
echo "4. 您的域名 (例如: https://bot.example.com)"
echo ""

# 询问是否继续
read -p "是否继续配置? (y/n): " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    print_info "部署已取消"
    exit 0
fi

# 配置Bot Token
echo ""
read -p "请输入 Bot Token: " BOT_TOKEN
if [ -z "$BOT_TOKEN" ]; then
    print_error "Bot Token 不能为空"
    exit 1
fi

# 配置管理员密码
read -p "请设置管理员密码 (至少8个字符): " ADMIN_TOKEN
if [ ${#ADMIN_TOKEN} -lt 8 ]; then
    print_error "管理员密码至少需要8个字符"
    exit 1
fi

# 配置Telegram ID
read -p "请输入您的 Telegram ID: " ADMIN_TELEGRAM_ID
if [ -z "$ADMIN_TELEGRAM_ID" ]; then
    print_error "Telegram ID 不能为空"
    exit 1
fi

# 配置域名
read -p "请输入您的域名 (例如: https://bot.example.com): " BASE_URL
if [ -z "$BASE_URL" ]; then
    print_error "域名不能为空"
    exit 1
fi

# 更新.env文件
print_info "正在更新配置文件..."
sed -i.bak "s|BOT_TOKEN=.*|BOT_TOKEN=$BOT_TOKEN|" .env
sed -i.bak "s|ADMIN_TOKEN=.*|ADMIN_TOKEN=$ADMIN_TOKEN|" .env
sed -i.bak "s|ADMIN_TELEGRAM_IDS=.*|ADMIN_TELEGRAM_IDS=$ADMIN_TELEGRAM_ID|" .env
sed -i.bak "s|BASE_URL=.*|BASE_URL=$BASE_URL|" .env

# 生成随机密钥
JWT_SECRET=$(openssl rand -base64 32 | tr -d '=' | tr -d '/' | tr -d '+' | head -c 32)
DATA_KEY=$(openssl rand -base64 32 | tr -d '=' | tr -d '/' | tr -d '+' | head -c 32)
SESSION_SECRET=$(openssl rand -base64 32)

sed -i.bak "s|JWT_SECRET=.*|JWT_SECRET=$JWT_SECRET|" .env
sed -i.bak "s|DATA_ENCRYPTION_KEY=.*|DATA_ENCRYPTION_KEY=$DATA_KEY|" .env
sed -i.bak "s|SESSION_SECRET=.*|SESSION_SECRET=$SESSION_SECRET|" .env

print_success "配置文件更新完成"

# 选择部署方式
echo ""
print_info "请选择部署方式："
echo "1. 完整部署 (包含PostgreSQL和Redis)"
echo "2. 简单部署 (使用SQLite，适合测试)"
echo "3. 外部数据库 (1Panel或已有数据库)"
echo ""
read -p "请选择 (1/2/3): " DEPLOY_MODE

case $DEPLOY_MODE in
    1)
        print_info "开始完整部署..."
        COMPOSE_FILE="docker-compose.full.yml"
        # 确保使用PostgreSQL
        sed -i.bak "s|DB_TYPE=.*|DB_TYPE=postgres|" .env
        ;;
    2)
        print_info "开始简单部署..."
        COMPOSE_FILE="docker-compose.simple.yml"
        # 确保使用SQLite
        sed -i.bak "s|DB_TYPE=.*|DB_TYPE=sqlite|" .env
        ;;
    3)
        print_info "使用外部数据库部署..."
        COMPOSE_FILE="docker-compose.yml"
        print_warning "请确保已在.env中配置数据库连接信息"
        ;;
    *)
        print_error "无效的选择"
        exit 1
        ;;
esac

# 创建必要的目录
print_info "创建必要的目录..."
mkdir -p logs data

# 构建和启动服务
print_info "开始构建和启动服务..."
if [ "$COMPOSE_FILE" = "docker-compose.yml" ]; then
    docker-compose build --no-cache
    docker-compose up -d
else
    docker-compose -f $COMPOSE_FILE build --no-cache
    docker-compose -f $COMPOSE_FILE up -d
fi

# 等待服务启动
print_info "等待服务启动..."
sleep 10

# 检查服务状态
if [ "$COMPOSE_FILE" = "docker-compose.yml" ]; then
    docker-compose ps
else
    docker-compose -f $COMPOSE_FILE ps
fi

# 显示日志
print_info "显示最近的日志..."
if [ "$COMPOSE_FILE" = "docker-compose.yml" ]; then
    docker-compose logs --tail=20 app
else
    docker-compose -f $COMPOSE_FILE logs --tail=20 app
fi

# 完成信息
echo ""
echo "======================================"
print_success "部署完成！"
echo "======================================"
echo ""
print_info "访问地址："
echo "  管理面板: $BASE_URL/admin/"
echo "  登录密码: $ADMIN_TOKEN"
echo ""
print_info "机器人地址："
echo "  在Telegram中搜索您的机器人并发送 /start"
echo ""
print_info "常用命令："
echo "  查看日志: docker-compose logs -f app"
echo "  重启服务: docker-compose restart"
echo "  停止服务: docker-compose down"
echo ""
print_warning "请保存好您的配置信息！"
echo "======================================"

# 保存配置备份
cp .env .env.backup
print_info "配置已备份到 .env.backup"