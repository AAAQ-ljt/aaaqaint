# 本仓库说明（Fork）

- 上游项目：[wahhheee/aint](https://github.com/wahhheee/aint)，Apache-2.0
- 本仓库：[AAAQ-ljt/aaaqaint](https://github.com/AAAQ-ljt/aaaqaint)，今后以此为准（canonical）
- 远程：`origin` = 本仓库；`upstream` = 上游，只读参考、不合并
- 本快照的父提交：上游 `98a71ac`（2026-11-14，v0.2.0-alpha.1 预发布说明）

AINT 是本地运行的 AI 互动小说运行时。本仓库用于个人部署：在本机或家里电脑运行 AINT，经 frp 与 Cloudflare Tunnel 用域名在手机和外网访问。

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

### 从上游同步的功能

本快照基于上游 `98a71ac`，并补回两项在上游此前提交中引入、快照时缺失的功能：

- `1f47a41`：记忆整理状态与完整记忆索引注入（`internal/runtime/compact*.go`、`internal/save/memory.go`、`internal/contextx`、`MemoryIndexGuide`）。
- `9277658`：阅读器的自动调试记录设置（`internal/settings`、前端 `DebugCaptureGate` 与开发者设置）。

其余差异均为上文列出的本地改动，不涉及对上游代码的回退。

## 许可

上游代码与版权声明见 `LICENSE`。按 Apache-2.0 第 4 条，本仓库保留原有版权与许可声明，并在此说明修改内容；本仓库的修改同样以 Apache-2.0 发布。
