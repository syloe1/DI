#!/usr/bin/env bash
# =============================================================================
# DI 一键发布脚本（在服务器上以 root 执行）
# =============================================================================
# 前置：把发布包 scp 到服务器 /tmp/di-deploy/，包含：
#   go-admin                 后端二进制（linux/amd64）
#   seed                     演示数据工具
#   di-web-dist.tar.gz       前端构建产物（tar 内含 dist/ 目录）
#   config.yaml              新版配置（含 auth/demo 段）
#   nginx-di-web.conf        新版本站点配置（保留服务器特定 listen/server_name）
#
# 使用：
#   scp -r dist/远程包内容 wk@server:/tmp/di-deploy/
#   ssh wk@server
#   sudo bash /tmp/di-deploy/remote-deploy.sh
#
# 可选环境变量：
#   INVITE_CODE=xxxx              指定注册邀请码；不传则自动生成（已存在则不覆盖）
#   PURGE_REVIEW_ACCOUNTS=0       不清理 review_ 前缀的评审测试账号（默认清理）
#   APP_DIR / WEB_DIR / SERVICE / NGINX_CONF  覆盖默认路径
#
# 特性：先备份、失败自动回滚、启动就绪检查、seed 幂等、前后端与 nginx 全链路验收。
# =============================================================================
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/go-admin}"
WEB_DIR="${WEB_DIR:-/var/www/di-web}"
SERVICE="${SERVICE:-go-admin}"
NGINX_CONF="${NGINX_CONF:-/etc/nginx/sites-available/di-web}"
STAGE_DIR="${STAGE_DIR:-/tmp/di-deploy}"
INVITE_CODE="${INVITE_CODE:-}"
PURGE_REVIEW_ACCOUNTS="${PURGE_REVIEW_ACCOUNTS:-1}"
ENV_FILE="$APP_DIR/go-admin.env"

log() { printf '\n==> %s\n' "$*"; }
die() { printf '\n!! %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "请用 root 执行：sudo bash $0"
for f in go-admin seed di-web-dist.tar.gz config.yaml nginx-di-web.conf; do
	[ -f "$STAGE_DIR/$f" ] || die "缺少发布文件：$STAGE_DIR/$f"
done
for bin in systemctl nginx curl; do
	command -v "$bin" >/dev/null 2>&1 || die "缺少命令：$bin"
done

TS="$(date +%Y%m%d-%H%M%S)"
BK="$APP_DIR/backup-$TS"
log "备份当前版本到 $BK"
mkdir -p "$BK"
cp -a "$APP_DIR/go-admin" "$BK/go-admin" 2>/dev/null || true
cp -a "$APP_DIR/config" "$BK/config" 2>/dev/null || true
cp -a "$ENV_FILE" "$BK/go-admin.env" 2>/dev/null || true
cp -a "$NGINX_CONF" "$BK/nginx-di-web.conf" 2>/dev/null || true
cp -a "$WEB_DIR/dist" "$BK/dist" 2>/dev/null || true

rollback() {
	log "开始回滚到 $BK"
	[ -f "$BK/go-admin" ] && cp -a "$BK/go-admin" "$APP_DIR/go-admin" || true
	[ -d "$BK/config" ] && cp -a "$BK/config/." "$APP_DIR/config/" || true
	[ -f "$BK/go-admin.env" ] && cp -a "$BK/go-admin.env" "$ENV_FILE" || true
	[ -f "$BK/nginx-di-web.conf" ] && cp -a "$BK/nginx-di-web.conf" "$NGINX_CONF" || true
	if [ -d "$BK/dist" ]; then
		rm -rf "$WEB_DIR/dist"
		cp -a "$BK/dist" "$WEB_DIR/dist" || true
	fi
	systemctl restart "$SERVICE" || true
	if nginx -t >/dev/null 2>&1; then systemctl reload nginx || true; fi
	log "回滚完成；手动检查：systemctl status $SERVICE --no-pager"
}

# ---------- 1. 后端二进制 + 配置 ----------
log "安装后端二进制与新版 config.yaml"
install -m 755 "$STAGE_DIR/go-admin" "$APP_DIR/go-admin.new"
install -m 755 "$STAGE_DIR/seed" "$APP_DIR/seed.new"
install -m 644 "$STAGE_DIR/config.yaml" "$APP_DIR/config/config.yaml"
mv "$APP_DIR/go-admin.new" "$APP_DIR/go-admin"
mv "$APP_DIR/seed.new" "$APP_DIR/seed"

# ---------- 2. 环境变量：补 auth/demo 键（已存在不覆盖） ----------
log "更新环境变量（注册邀请码 / 体验 Demo）"
if ! grep -q '^APP_DEMO_ENABLED=' "$ENV_FILE"; then
	printf '\n# ---- 演示入口 / 注册风控（本次发布新增）----\nAPP_DEMO_ENABLED=true\n' >>"$ENV_FILE"
fi
if ! grep -q '^APP_AUTH_REGISTER_INVITE_CODE=' "$ENV_FILE"; then
	if [ -z "$INVITE_CODE" ]; then
		INVITE_CODE="$(openssl rand -hex 8)"
	fi
	printf 'APP_AUTH_REGISTER_INVITE_CODE=%s\n' "$INVITE_CODE" >>"$ENV_FILE"
else
	INVITE_CODE="$(grep '^APP_AUTH_REGISTER_INVITE_CODE=' "$ENV_FILE" | head -1 | cut -d= -f2-)"
fi

# ---------- 3. 重启并等待就绪 ----------
log "重启 $SERVICE"
systemctl restart "$SERVICE"
READY=0
for _ in $(seq 1 30); do
	if curl -sf -m 2 http://127.0.0.1:19999/readyz >/dev/null 2>&1; then
		READY=1
		break
	fi
	sleep 1
done
if [ "$READY" -ne 1 ]; then
	journalctl -u "$SERVICE" -n 40 --no-pager || true
	rollback
	die "后端未就绪，已回滚"
fi
log "后端已就绪"

# ---------- 4. 演示种子数据 + 清理评审测试账号 ----------
log "写入演示种子数据（幂等）"
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
"$APP_DIR/seed" -config "$APP_DIR/config/config.yaml"

if [ "$PURGE_REVIEW_ACCOUNTS" = "1" ]; then
	log "清理 review_ 前缀的评审测试账号（先备份数据库）"
	if command -v mysqldump >/dev/null 2>&1; then
		MYSQL_PWD="${APP_MYSQL_PASSWORD:-}" mysqldump \
			-h"${APP_MYSQL_HOST:-127.0.0.1}" -P"${APP_MYSQL_PORT:-3306}" \
			-u"${APP_MYSQL_USERNAME:-root}" "${APP_MYSQL_DBNAME:-go}" \
			>"$BK/go-before-purge.sql" 2>/dev/null || log "mysqldump 失败，跳过 DB 备份"
	fi
	"$APP_DIR/seed" -config "$APP_DIR/config/config.yaml" -purge-users-prefix review_ -yes ||
		log "清理 review_ 账号失败（不影响服务）"
fi

# ---------- 5. 前端静态产物 ----------
log "发布前端产物到 $WEB_DIR"
rm -rf "$WEB_DIR/dist.new"
mkdir -p "$WEB_DIR/dist.new"
tar -xzf "$STAGE_DIR/di-web-dist.tar.gz" -C "$WEB_DIR/dist.new"
if [ ! -f "$WEB_DIR/dist.new/dist/index.html" ]; then
	rollback
	die "前端包结构异常（缺少 dist/index.html），已回滚"
fi
rm -rf "$WEB_DIR/dist"
mv "$WEB_DIR/dist.new/dist" "$WEB_DIR/dist"
rmdir "$WEB_DIR/dist.new" 2>/dev/null || true
chown -R root:root "$WEB_DIR/dist"
chmod -R a+rX "$WEB_DIR/dist"

# ---------- 6. Nginx ----------
log "更新 nginx 配置并 reload"
install -m 644 "$STAGE_DIR/nginx-di-web.conf" "$NGINX_CONF"
if ! nginx -t; then
	rollback
	die "nginx 配置校验失败，已回滚"
fi
systemctl reload nginx

# ---------- 7. 验收 ----------
log "全链路验收"
echo -n "backend /readyz:        "
curl -s -m 3 http://127.0.0.1:19999/readyz || true
echo
echo -n "register/config:        "
curl -s -m 3 http://127.0.0.1:19999/user/register/config || true
echo
echo -n "nginx /api 代理:        "
curl -s -m 3 -H 'Host: di.syloe1.xyz' http://127.0.0.1/api/user/register/config || true
echo
echo -n "未登录 user/search:     "
curl -s -m 3 -o /dev/null -w 'HTTP %{http_code}（期望 404）' \
	'http://127.0.0.1:19999/user/search?username=a' || true
echo
TOKEN="$(curl -s -m 3 -X POST http://127.0.0.1:19999/user/demo-login |
	sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
if [ -n "$TOKEN" ]; then
	echo "体验 Demo 登录:         成功（token ${TOKEN:0:20}...）"
	LOGIN_OUT="$(curl -s -m 3 -o /dev/null -w '%{http_code}' -X POST \
		http://127.0.0.1:19999/auth/user/logout -H "Authorization: Bearer $TOKEN")"
	REUSE_OUT="$(curl -s -m 3 -o /dev/null -w '%{http_code}' \
		http://127.0.0.1:19999/auth/message/conversations -H "Authorization: Bearer $TOKEN")"
	echo "登出 / 旧 token 复用:   HTTP $LOGIN_OUT / $REUSE_OUT（期望 200 / 401）"
else
	log "体验 Demo 登录失败（检查 demo.enabled 与 seed）"
fi

cat <<EOF

============================================================
发布完成 ✅
  版本备份：$BK
  注册邀请码：${INVITE_CODE:-（未设置）}
  前端目录：$WEB_DIR/dist
  回滚命令：
    cp -a $BK/go-admin $APP_DIR/go-admin
    cp -a $BK/config/. $APP_DIR/config/
    cp -a $BK/go-admin.env $ENV_FILE
    cp -a $BK/nginx-di-web.conf $NGINX_CONF
    rm -rf $WEB_DIR/dist && cp -a $BK/dist $WEB_DIR/dist
    systemctl restart $SERVICE && nginx -t && systemctl reload nginx
  验收：
    curl -s https://di.syloe1.xyz/api/user/register/config
============================================================
EOF
