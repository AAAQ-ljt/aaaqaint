# 本仓库说明（Fork）

- 上游项目：[wahhheee/aint](https://github.com/wahhheee/aint)，Apache-2.0
- 本 fork：[AAAQ-ljt/aint](https://github.com/AAAQ-ljt/aint)
- 本快照的父提交：上游 `98a71ac`（2026-11-14，v0.2.0-alpha.1 预发布说明）

AINT 是本地运行的 AI 互动小说运行时。本 fork 用于个人部署：在本机或家里电脑运行 AINT，经 frp 与 Cloudflare Tunnel 用域名在手机和外网访问。

## 相对上游的改动

### 部署层

- `deploy/`：frp 与 Cloudflare Tunnel 的参考配置、启动脚本和说明。所有配置为示例值，不含真实 token、域名或服务器地址。

### 服务端

- `internal/server/middleware.go`：`hostGuard` 增加 `AINT_ALLOWED_HOSTS` 环境变量，按域名放行反向代理与隧道转发过来的请求；默认行为不变，仍只接受 IP 字面量与 localhost。

### 运行时

- `internal/runtime/turn.go`：模型返回拒答时，追加一次收束提示并重试，最多两次；仍未产出时按原有路径提示换写法。

### 作品

- `internal/work/cardimport.go`：导入 SillyTavern 角色卡（PNG 中的 `ccv3`/`chara` 文本块、`.charx` 压缩包或裸 JSON），合成为 AINT 作品目录。
- `internal/store/migrations/0002_work_title.sql`：`works.title_override`，用于只改作品显示名。前端对应 `WorkRenameDialog`。

### 与上游仍有差异的部分

本快照基于较早的代码整理，部分文件与上游 `main` 的较新提交不同（例如记忆整理状态、自动调试记录）。这些差异来自快照时点与本地取舍，不是对上游的回退。

## 许可

上游代码与版权声明见 `LICENSE`。按 Apache-2.0 第 4 条，本 fork 保留原有版权与许可声明，并在此说明修改内容；本 fork 的修改同样以 Apache-2.0 发布。
