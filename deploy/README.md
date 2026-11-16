# 部署：frp + Cloudflare Tunnel

本目录是把 AINT 从本机经域名暴露到公网的参考配置。本机与服务器都不对公网开放 AINT 的端口，公网入口只有 Cloudflare 边缘。

## 链路

```
手机浏览器
  │ https://<域名>
  ▼
Cloudflare 边缘（终止 TLS，证书自动签发与续期）
  │ Cloudflare Tunnel：服务器上的 cloudflared 向 Cloudflare 外拨的长连接
  ▼
服务器 <服务器 IP>
  │ http://127.0.0.1:7860
  ▼
frps（服务器）══ frp 隧道（frpc 主动外拨）══ frpc（本机 Windows）
  ▼
aint serve --listen 127.0.0.1 --port 7860
```

两段隧道都由内网一侧主动拨号：frpc 连服务器，cloudflared 连 Cloudflare。所以本机不需要公网 IP，路由器不需要端口映射，服务器也不需要开放 443。

## 组件职责

| 组件 | 位置 | 职责 |
| --- | --- | --- |
| aint | 本机 | 提供 `127.0.0.1:7860` 的 HTTP 服务 |
| frpc | 本机 | 把本机 7860 注册到服务器的 7860 |
| frps | 服务器 | 监听控制端口，按 token 认证后为 frpc 打开转发端口 |
| cloudflared | 服务器 | 把公网域名的请求转发到本机 7860，即 frps 的转发端口 |
| Cloudflare | 边缘 | 解析域名、终止 HTTPS、按隧道分发请求 |

## 配置步骤

1. 服务器安装 frps，配置见 `frps.toml.example`，用 systemd 常驻。控制端口只对外开这一个，转发端口由 frps 自己监听。
2. 本机放置 `frpc.toml`（见 `frpc.toml.example`），token 与 frps 保持一致。
3. 在 Cloudflare 创建隧道，添加公共主机名，服务指向 `http://127.0.0.1:7860`。DNS 里该主机名是到 `<隧道ID>.cfargotunnel.com` 的 CNAME；填 A 记录会绕过隧道，直接连服务器的 443。
4. 启动 `start-aint-frp.bat`：先起 aint，再起 frpc，并设置 `AINT_ALLOWED_HOSTS`。
5. aint 默认只接受 Host 为 IP 字面量或 localhost 的请求（防 DNS 重绑定）。经隧道访问时 Host 是域名，必须用 `AINT_ALLOWED_HOSTS=<域名>` 放行。

## 关闭与恢复

关掉 frpc，服务器上的 7860 立即消失，域名访问返回 502；cloudflared 不受影响。停掉 cloudflared，域名完全不可用，frp 链路本身仍然正常。

## 安全注意

- `frps.toml` 的 `proxyBindAddr` 应设为 `127.0.0.1`。保持默认值时 frps 会把转发端口监听在 `0.0.0.0`，公网可以直接用 `服务器IP:7860` 绕过 Cloudflare 访问。
- token、域名、服务器 IP 都不要提交进仓库。本目录所有配置均为示例值。
- 在 aint 中设置访问密码，作为应用层的最后一道校验。
- Cloudflare 在边缘解密 HTTPS，边缘到 cloudflared 之间是另一段加密。
