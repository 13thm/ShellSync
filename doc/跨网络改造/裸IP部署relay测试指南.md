# 裸 IP 部署 relay + 手机流量实测指南

> 场景：**没有域名**，直接用 VPS 公网 IP 部署 relay-server，手机关 Wi-Fi 用流量完成跨网配对测试。
> 本方案跑的是**明文 ws**（无 TLS），仅适合个人测试，不适合长期对外。

---

## 0. 前提清单

| 项 | 值 |
|---|---|
| VPS 公网 IP | `39.105.37.57`（阿里云，忽略内网 `172.25.1.11`） |
| relay 端口 | `8788`（TCP） |
| 本地产物 | `server/bin/relay-server`（Linux amd64 静态二进制，7.3 MB） |
| 打包产物 | `server/bin/relay-deploy.tar.gz`（二进制 + 配置，3.1 MB） |

如果还没编译，在 `server/` 目录执行（Windows PowerShell）：

```powershell
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -trimpath -ldflags "-s -w" -o bin/relay-server ./cmd/relay-server
Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED   # 用完记得清掉，否则后续本地构建会交叉编译
```

架构核对：VPS 是 x86_64 → 用 `GOARCH=amd64`；如是 ARM 机器（`uname -m` 显示 `aarch64`）→ 改 `GOARCH=arm64` 重新编译。

---

## 1. 上传到 VPS

在**你电脑**上执行（任选其一）：

```bash
# 方式 A：WSL / Git Bash 自带 scp
scp server/bin/relay-deploy.tar.gz root@39.105.37.57:/opt/

# 方式 B：PowerShell（Windows 10+ 自带 OpenSSH 客户端）
scp E:\code\app\ShellSync\server\bin\relay-deploy.tar.gz root@39.105.37.57:/opt/
```

> 提示：阿里云 ECS 若用密钥对登录，加 `-i <私钥路径>`；密码登录则直接输密码。

---

## 2. VPS 上部署并启动

SSH 登录后：

```bash
mkdir -p /opt/shellsync-relay
tar xzf /opt/relay-deploy.tar.gz -C /opt/shellsync-relay
cd /opt/shellsync-relay

# 直接裸跑：监听所有网卡，日志打到前台
RELAY_LISTEN=0.0.0.0:8788 ./relay-server
```

看到这行即成功：

```
... relay listening addr=0.0.0.0:8788
```

> tar 包里的 `config.prod.toml`（监听 127.0.0.1 + Caddy 方案）本流程**不用**，`RELAY_LISTEN` 环境变量优先级最高，直接覆盖即可。
>
> 嫌 SSH 断开就挂掉的话，先简单用 nohup：`nohup env RELAY_LISTEN=0.0.0.0:8788 ./relay-server > relay.log 2>&1 &`（正式自启见 §7）。

---

## 3. 放行端口（关键，最常翻车）

两层都要查，任何一层没放行都连不上：

### 3.1 阿里云安全组（必做）

1. 控制台 → 云服务器 ECS → 实例 → 找到这台机器 → **安全组** → 配置规则
2. **入方向** → 手动添加：
   - 协议类型：**自定义 TCP**
   - 端口范围：**8788/8788**
   - 授权对象（源）：`0.0.0.0/0`
3. 保存

### 3.2 VPS 系统防火墙（一般默认关，顺手确认）

```bash
systemctl is-active firewalld ufw 2>/dev/null   # 都显示 inactive/未找到就不用管
# 若在跑，放行：
firewall-cmd --permanent --add-port=8788/tcp && firewall-cmd --reload   # firewalld
ufw allow 8788/tcp                                                       # ufw
```

---

## 4. 本地 daemon 指向公网 relay

编辑 `C:\Users\<你>\.shellsync\config.json`，把 `cloud` 节改成：

```json
"cloud": {
  "enabled": true,
  "url": "ws://39.105.37.57:8788/ws"
}
```

> - 协议是 `ws://`（明文，因为没有 TLS）；路径必须是 `/ws`
> - `enabled` 保持 true；也可以在 Desktop「设置」页用云中继开关实时切换
> - 如果改过 `SHELLSYNC_DATA_DIR` 开发环境，去对应数据目录改 config

然后**完全退出 ShellSync（Desktop）并重开**，让 daemon 重启并重新拨号。

验证 daemon 连上了 relay（任选）：

- Desktop 设置页云中继状态显示 online；
- VPS 上 relay 前台日志出现 daemon 的 `hello` / `reg` 连接记录。

---

## 5. 手机流量实测

### 5.1 先验证公网链路（不配对）

手机**关掉 Wi-Fi、开流量**，浏览器打开：

```
http://39.105.37.57:8788/health
```

看到 `{"status":"ok","ver":"dev"}` → 公网到 relay 的路通了。

### 5.2 真实配对

1. Desktop →「设置 → 生成配对码」，确认二维码是 **v2**（日志/界面含 cloud 信息；如果还是 v1，说明 daemon 没连上 relay，回 §4 排查）；
2. 手机 App → 扫码；
3. App 行为：先试局域网直连（3 秒超时）→ 失败后自动回落云隧道 → `claim 配对码 → 建隧道 → 隧道内 /api/pair/verify` → 拿到 token 进主页；
4. 主页连接路径指示应为「云」。

### 5.3 无手机时的隧道自检（可选，本地即可）

```bash
cd server
go run ./cmd/relay-probe -url ws://39.105.37.57:8788/ws -code <配对码> -get /health
```

probe 模拟手机走完整云路径，能打印 daemon 的 HTTP 响应即整条链路 OK。

---

## 6. 排查速查表

| 现象 | 原因 | 处理 |
|---|---|---|
| 手机打不开 `/health` | 安全组没放行 8788 / relay 没跑 / 系统防火墙拦了 | §3、§2；VPS 上 `ss -tlnp \| grep 8788` 确认监听 `0.0.0.0:8788` |
| `/health` 通，但二维码还是 v1 | 本地 daemon 没连上 relay | config.json 的 cloud.url 是否改对、ShellSync 是否重启、设置页云开关是否开 |
| 二维码 v2，配对报「均不可达」 | 手机没走云（还在 Wi-Fi 里直连失败？）或配对码过期 | 手机确实关 Wi-Fi；重新生成配对码再扫；claim 限速 5 次/分/IP，频繁失败会被拉黑 10 分钟，等一会 |
| relay 日志有 daemon 无手机连接 | 手机侧问题 | 看 §5.1 是否通、App 是否 release 版（release 不吃开发者 relay 覆盖，用二维码自带地址） |
| 连上后卡顿/断流 | 运营商对长连接干扰 | 正常现象，App/daemon 都有重连；正式使用建议 §8 上 TLS |

---

## 7. 让 relay 常驻（可选，测通后）

```bash
cat > /etc/systemd/system/shellsync-relay.service <<'EOF'
[Unit]
Description=ShellSync relay-server
After=network-online.target
Wants=network-online.target

[Service]
WorkingDirectory=/opt/shellsync-relay
Environment=RELAY_LISTEN=0.0.0.0:8788
ExecStart=/opt/shellsync-relay/relay-server
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now shellsync-relay
systemctl status shellsync-relay          # active (running) 即 OK
journalctl -u shellsync-relay -f          # 看实时日志
```

---

## 8. 测完收尾 / 后续升级

**临时收摊：**

- VPS：`systemctl stop shellsync-relay`（或 Ctrl+C / kill），安全组删掉 8788 规则
- 本地：config.json 的 `cloud.url` 改回 `ws://127.0.0.1:8788/ws`，重启 ShellSync

**之后有域名了再正规化**（自动 TLS + 只开 443）：

1. 域名 A 记录 → `39.105.37.57`
2. VPS 上编辑上传包里的 `Caddyfile`，把 `relay.shellsync.example.com` 换成你的域名
3. `cd /opt/shellsync-relay && docker compose up -d`（或继续裸跑 relay + 单独装 Caddy）
4. 本地 `cloud.url` → `wss://你的域名/ws`（无端口 = 443，手机端自动走 TLS）
5. 安全组关掉 8788，只留 443/22
