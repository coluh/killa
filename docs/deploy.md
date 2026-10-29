# 部署上线（国内 VPS + Caddy）

目标：单台 VPS 跑 `app`（Go + 静态资源）与 `caddy`（TLS/反代），对外提供
`https://<域名>` 与 `wss://<域名>/ws`。

## 0. 前置条件

- VPS：2 核 2G 起，Ubuntu 22.04/24.04，公网 IP。
- 域名：A 记录指向 VPS；**中国大陆服务器必须先完成 ICP 备案**，否则 80/443 会被拦截。
- 安全组/防火墙放行：22/tcp、80/tcp、443/tcp、443/udp。
- Docker Engine + Compose 插件。

## 1. 初始化服务器

```bash
sudo apt update && sudo apt upgrade -y
# Docker 官方脚本
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER"   # 重新登录后生效

sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 443/udp
sudo ufw enable
```

验证：`docker run --rm hello-world`。

## 2. 拉代码与配置

```bash
git clone <repo-url> killa
cd killa
cp .env.example .env
chmod 600 .env
# 编辑 .env：KILLA_DOMAIN=manyblocks.online
```

`KILLA_ORIGINS` 同域部署可留空；如果页面与游戏服务器不同域，填页面完整 Origin，
逗号分隔。`KILLA_ROOMS` 控制并发房间数（默认 16，即 128 个座位；每房 4v4）。
如需回退旧仓库图，可在 compose 的 `command` 里加 `-map classic`。

## 3. 构建并启动

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f caddy
```

首次启动 Caddy 会自动申请 TLS 证书（Let's Encrypt）。等待 `docker compose ps`
中两个服务都变为 `healthy` / `running`。

## 4. 验证

```bash
curl -fsS https://manyblocks.online/healthz     # ok
curl -fsS https://manyblocks.online/status      # 房间/人数/运行时长
curl -fsSI https://manyblocks.online/ | head    # 200 + 安全响应头
```

浏览器打开域名，点击「进入战场」；再开一个窗口（或另一台设备）验证：
- 8/8 满员、倒计时后进入 live；
- 移动/射击/换弹/配装/计分板正常；
- `wss://` 连接无报错（DevTools → Network → WS）。

## 5. 常用运维

```bash
docker compose logs -f app        # 加入/离开/开局/结算日志
docker compose restart app
docker compose up -d --build      # 更新版本
docker stats                      # CPU / 内存
curl -fsS https://<域名>/healthz
```

更新流程：`git pull && docker compose up -d --build`（会短暂重启，进行中的对局丢失）。

## 6. 数据与备份

当前版本没有数据库，战绩只在内存中；重启即重置。需要备份的只有：

- `caddy_data` 卷（TLS 证书，丢失后会自动重新签发）；
- `.env` 与代码仓库。

后续加入账号/战绩时再接 Postgres 并补充备份策略。

## 7. 压测

本机对已部署实例跑 8 个客户端（房间上限 8 人）：

```bash
cd server
go run ./cmd/loadtest -url wss://manyblocks.online/ws -clients 8 -seconds 20
```

输出包含每个客户端平均快照频率、射击事件数、RTT p50/p95。本地基线（同一台机器）：
8 客户端 / 8 秒，19.7 快照/秒/客户端，0 错误，RTT p50 < 1ms。公网主要看 RTT 与丢包；
单房间 8 人时 app 进程 CPU 应远低于 1 核，若接近打满，先降低 tick 频率或拆分房间。

## 8. 容量与限制

- 单进程多房间：每房最多 8 人，容量 = `KILLA_ROOMS × 8`，默认 16 房（128 座）。
- 匹配策略：优先加入人最多的未满房间，满 8 人后自动开新房；空房保留复用。
- 同时在线目标 < 100；更大规模需要拆分进程/匹配服与共享状态。
- 无账号、无持久化；游客 ID 仅存在于当前会话。
- 反作弊为服务端权威 + 基础校验，尚未接入行为检测。
- 机器人会占满空位；真人加入时自动让位。

## 9. 故障排查

| 现象 | 排查 |
| --- | --- |
| 证书申请失败 | DNS 未生效、80/443 未放行，或域名未备案 |
| 502 Bad Gateway | `docker compose ps` 看 app 是否 healthy；`logs app` |
| WebSocket 连接后立即断开 | 检查 Origin 白名单与反向代理是否保留 Host；看 app 日志 |
| 页面能开但对局连不上 | DevTools 看 `/ws` 是否 101；Caddy 日志 |
| CPU 飙高 | `docker stats`；`/status` 看房间与人数；确认是否有异常连接刷消息 |

## 10. 安全清单

- [ ] `.env` 权限 600，不进仓库
- [ ] 只暴露 80/443，app 端口不映射到公网
- [x] `KILLA_ORIGINS` 按需收紧
- [x] 连接数上限（256）与单连接消息限流（400/s）
- [ ] 定期 `apt upgrade` 与镜像更新
- [x] 日志轮转已开启（compose 中 10MB × 3）
- [x] 隐私说明/用户协议页面模板（`/privacy.html`、`/terms.html`）
- [ ] 补充运营主体与联系方式，完成 ICP 备案与合规审查
