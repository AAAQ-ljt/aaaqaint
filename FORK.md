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
- 书架排序：打开存档或新建存档时更新 `works.updated_at`，作品按最近游玩倒序排列（`internal/work/service.go` 的 `Touch`、`internal/save/service.go`、`internal/server/api_saves.go`）。

### 运行时

- 记忆索引（同步上游 `1f47a41`）：阶段开始时优先注入 `/memory/index.md` 的完整内容，文件不存在时回退到自动目录；整理流程增加同步索引与核对状态的步骤。涉及 `internal/save/memory.go`、`internal/contextx`、`internal/runtime/compact*.go`、`internal/prompts`。

### 作品

- `internal/work/cardimport.go`：导入 SillyTavern 角色卡（PNG 中的 `ccv3`/`chara` 文本块、`.charx` 压缩包或裸 JSON），合成为 AINT 作品目录。
- `internal/store/migrations/0002_work_title.sql`：`works.title_override`，用于只改作品显示名。前端对应 `WorkRenameDialog`。

### 已同步的上游功能

- 已同步上游 `v0.2.1-alpha.1`（8 个提交）：Windows 系统手动代理、Android Termux 域名解析、拒答与异常流终止识别、整理结果按事务提交并在中断后保留重试目标、阅读器中断提示与重试。
- 同步后移除了本地早先的“拒答自动重试”补丁，改为上游的明确中断提示（正文保留，可换写法重试）。

### 与上游仍有差异的部分

上游 `9277658` 的阅读器自动调试记录尚未同步：本地仍由阅读页顶栏的调试开关手动控制。

## 许可

上游代码与版权声明见 `LICENSE`。按 Apache-2.0 第 4 条，本仓库保留原有版权与许可声明，并在此说明修改内容；本仓库的修改同样以 Apache-2.0 发布。
