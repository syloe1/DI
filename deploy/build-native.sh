#!/usr/bin/env bash
# =============================================================================
# 本地交叉编译 → 可选上传并重启（生产不走 Docker 的部署方式）
# =============================================================================
# 用法：
#   ./deploy/build-native.sh                 # 只编译，产出 dist/go-admin
#   ./deploy/build-native.sh root@1.2.3.4    # 编译 + 上传 + 重启 systemd 服务
#
# 可覆盖的环境变量：
#   REMOTE_DIR=/opt/go-admin   服务器上的应用根目录（默认 /opt/go-admin）
#   SERVICE=go-admin           systemd 单元名（默认 go-admin）
#
# 需要以 root（或有 sudo 免密的账号）登录服务器：远程要执行 mv + systemctl restart。
# 目录与二进制都是 root:root，服务用户 go-admin 只读+可执行，无需 chown。
#
# 前置条件（首次部署需先在服务器上完成）：
#   - 已按 docs/deploy-native.md 建好目录、用户、config.yaml、go-admin.env
#   - 已安装 deploy/systemd/go-admin.service
#   本脚本只负责「替换二进制 + 重启」，不负责首次环境搭建。
set -euo pipefail

cd "$(dirname "$0")/.."

HOST="${1:-}"
REMOTE_DIR="${REMOTE_DIR:-/opt/go-admin}"
SERVICE="${SERVICE:-go-admin}"
OUT="dist/go-admin"

echo "==> 交叉编译 linux/amd64（CGO_ENABLED=0 → 纯静态二进制，无需服务器装 Go）"
# 不加 -ldflags="-s -w"：保留符号，pprof / 崩溃栈才看得到函数名。
# 想换体积就自己加，代价是 pprof 里只剩地址。
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	go build -trimpath -o "$OUT" ./cmd/server

echo "==> 产出："
ls -lh "$OUT"

if [ -z "$HOST" ]; then
	cat <<EOF

仅编译完成，未上传。上传并重启：

  ./deploy/build-native.sh user@your-server

或手动：

  scp $OUT user@your-server:$REMOTE_DIR/go-admin.new
  ssh user@your-server 'cd $REMOTE_DIR && mv go-admin.new go-admin && systemctl restart $SERVICE'
EOF
	exit 0
fi

# 先传到 .new 再 mv：Linux 不允许覆盖正在运行的二进制（ETXTBSY），
# 直接 scp 到 go-admin 会失败；mv 是 rename(2)，运行中的旧进程不受影响。
STAGE="$REMOTE_DIR/go-admin.new"
echo "==> 上传到 $HOST:$STAGE"
scp -q "$OUT" "$HOST:$STAGE"

echo "==> 替换并重启 $SERVICE"
if ! ssh "$HOST" "
	set -e
	chmod 755 '$STAGE'
	mv '$STAGE' '$REMOTE_DIR/go-admin'
	systemctl restart $SERVICE
	sleep 2
	systemctl is-active $SERVICE
"; then
	echo "!! 服务重启后未处于 active 状态，查看日志：" >&2
	echo "   ssh $HOST 'journalctl -u $SERVICE -n 50 --no-pager'" >&2
	exit 1
fi

echo "==> 完成，$SERVICE 已重启"
