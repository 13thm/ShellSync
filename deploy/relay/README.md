# relay 生产部署(单 VPS)

一套 Docker Compose 起 relay-server + Caddy(自动 TLS)。对应《开发步骤程序.md》R1-11。

## 步骤

1. **VPS**:2C4G 起,国内(需备案)或香港(免备案);安全组只放 443 与 22。
2. **DNS**:`relay.shellsync.example.com` A 记录 → VPS IP。
3. **构建上传**:

   ```bash
   cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
     go build -trimpath -ldflags "-s -w" -o bin/relay-server ./cmd/relay-server
   scp bin/relay-server deploy/relay/* user@vps:/opt/shellsync-relay/
   ```

   (或直接用 `docker build` 打镜像,见 docker-compose.yml 注释。)

4. **改配置**:VPS 上编辑 `Caddyfile` 把域名换成自己的;`config.prod.toml` 默认即可。
5. **启动**:`cd /opt/shellsync-relay && docker compose up -d`
6. **验收**:`curl https://relay.shellsync.example.com/health` → `{"status":"ok",...}`,
   证书链完整。

## 切换上线(共 3 步,见《修改计划》§2.3)

1. 本部署(本文);
2. 改 `~/.shellsync/config.json` 的 `cloud.url` → `wss://relay.shellsync.example.com/ws`;
3. 重启 daemon(重开 ShellSync)。

## 运维

- 回滚 = `docker compose stop relay`:所有手机云路径断开,自动回落局域网,无需发版;
- 单机故障自愈:`restart: unless-stopped`,daemon 指数退避重连,秒级闪断;
- 观测:`curl https://relay.../metrics`(连接数/活跃流/字节数);
- 拉黑滥用:VPS 防火墙按 IP 封禁(relay 自身亦有 claim 限速与拉黑)。
