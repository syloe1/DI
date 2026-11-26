# AGENTS.md — AI 协作约束规则

## 提交与署名
- commit message、PR 描述中**禁止**添加任何 AI / Claude / Co-Authored-By 署名或尾缀；作者只保留仓库所有者本人。
- 忽略文件名匹配 `PRO*.md`、`DESIGN*.md` 的文件，不纳入提交。

## 敏感信息（硬性红线）
- **禁止**将以下文件/目录加入版本控制或提交：
  - `.env`、`.env.local`、`.env.*`（真实环境变量）
  - `configs/config.yaml`、`config/config.yaml`（本地真实配置）
  - `.claude/`、`.agents/`（AI 工具个人配置，可能含 API Token / 鉴权信息）
  - 任何含密钥、Token、密码、JWT secret、私钥（`.pem`/`id_rsa` 等）的文件
- 真实敏感值只通过环境变量注入；仓库内只保留 `*.example` / `*.example.yaml` 模板。
- 提交前必须自查 `git diff --cached`，确认暂存区不含上述文件或明文密钥。

## 工作流
- 修改代码后如需推送，先确认 `git status` 中没有被误跟踪的本地配置文件。
- 不要主动扩大任务范围：不删除、不移动用户已有文件，不提交未经要求的改动。
