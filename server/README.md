# relay-server

ShellSync 云中继服务(单二进制 Go 服务)。职责:**配对码路由 + 会话撮合 + L4 透明字节转发**。
不理解任何业务协议 —— 手机与 daemon 之间的 HTTP/WebSocket/token 认证原样穿过隧道。

设计文档:`doc/跨网络改造/跨网络配对方案.md`(§5/§7),逐步骤:`doc/跨网络改造/开发步骤程序.md`。

## 快速开始(dev)

```bash
cd server
go run ./cmd/relay-server
# → relay listening addr=127.0.0.1:8788
curl http://127.0.0.1:8788/health   # {"status":"ok","ver":"dev"}
```

配置:默认读工作目录 `config.toml`;环境变量 `RELAY_CONFIG` 指定其它路径,`RELAY_LISTEN` 覆盖监听地址。

## 调试工具 relay-probe

模拟手机端走完 claim → open → 隧道内 HTTP GET:

```bash
go run ./cmd/relay-probe -url ws://127.0.0.1:8788/ws -code 482913 -get /health
# 已配对(知道 devId)时:-dev <devId> 代替 -code
```

## 帧协议(v1)

见 `relay/frame.go` 顶部注释。要点:

- 控制帧 = WS TEXT(JSON);数据帧 = WS BINARY `[streamId:4B][len:4B][payload≤32KiB]`;
- 首帧必须 `hello{role:daemon|client, ver:1}`;
- daemon:`reg{devId,sign}` → `code{code,ttl}`(服务端 ack 同型帧);
- client:`claim{code}`(一次性,限速 5 次/分/IP,超限拉黑 10 分钟)→ `open{devId?}`;
- 流:服务端分配 `streamId`,daemon `accept` 后双向透传;`close`/断连双向清理。

## 部署

生产(Docker Compose + Caddy 自动 TLS)见 `deploy/relay/`。进程内不做 TLS——由 Caddy 终结。

## 状态

全部内存、无数据库:重启 = 所有连接重建(daemon 指数退避重连,配对码重新生成)。
隐私红线:不记录隧道内容;日志只含连接元数据(IP、devId、字节数)。
