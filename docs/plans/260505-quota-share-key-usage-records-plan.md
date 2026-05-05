# quota_share Key 自助明细查询方案

## 背景

当前公开页 `/quota-share` 已支持用户输入自己的 API key 查询 quota_share 额度窗口、可用额度和模型汇总，但无法查看“这个 key 自己产生了哪些请求”。管理员 `/usage` 页面虽然有完整用量明细，但需要登录态且包含管理员视角字段，不适合直接开放给下游 key 持有人。

本轮目标是在不改变现有登录态 `/usage`、不改变 `/v1/usage` 响应和 quota_share 当前限流口径的前提下，为 `/quota-share` 增加 key 级只读明细查询、分页、日期筛选和基础 CSV 导出。

## 用户确认

本轮根据用户提供的上一位 agent 方案直接实施。仓库规则要求“每次修改之前必须先创建规划文档，记录修改背景、关键决策、涉及文件修改等，用户同意后才能进行修改”；本文件即为实施前规划文档，用户当前请求“Implement the plan”视为对该方案的实施确认。

## 关键决策

1. 新增 API key 鉴权的只读接口：
   - `GET /v1/usage/logs`
   - `GET /v1/usage/logs/stats`
2. 两个接口都从 `Authorization: Bearer <key>` 鉴权结果读取当前 `apiKey.ID`，强制按当前 key 查询，不接受前端传入 `api_key_id`，避免越权。
3. API key 中间件把 `/v1/usage/logs*` 纳入只读 usage 路径，和现有 `/v1/usage` 一样只鉴权，不做余额、quota_share 或 rate limit 拦截，确保额度耗尽或过期的 key 仍能查自己的历史。
4. 返回专用脱敏 DTO，仅包含下游 key 用户需要的字段：
   - 时间、模型、reasoning effort、入口 endpoint、请求类型
   - token 明细、费用、耗时、首 token 时间、user agent、billing mode
   - 不返回完整 API key、账号、账号 ID、IP、用户对象、分组内部字段、上游账号倍率和管理员专属成本字段。
5. 前端 `/quota-share` 只在当前 Vue state 中保存输入 key，不写入 `localStorage` 或 `sessionStorage`。刷新页面后需要重新输入。
6. CSV 导出按当前筛选范围分页拉取全部明细，并做 CSV 注入转义。

## 涉及文件

| 文件 | 计划修改 |
|------|----------|
| `backend/internal/service/usage.go` 或相邻 service 类型文件 | 增加 key 级 usage log DTO / stats DTO / query 类型 |
| `backend/internal/repository/usage_log_repo.go` | 复用现有 usage_logs 查询能力，增加按 API key 过滤的脱敏列表与统计查询 |
| `backend/internal/handler/gateway_handler.go` 或新增 handler 文件 | 新增 `/v1/usage/logs` 与 `/v1/usage/logs/stats` handler |
| `backend/internal/middleware/api_key_auth.go` | 把 `/v1/usage/logs*` 归类为只读 usage 请求 |
| `backend/internal/server/routes/*.go` | 注册两个新路由 |
| `frontend/src/views/KeyUsageView.vue` | 增加用量明细区域、日期筛选、分页、刷新和 CSV 导出 |
| `frontend/src/types/index.ts` | 增加公开 key usage 明细与 stats 类型 |
| `frontend/src/api/usage.ts` 或相邻 API 文件 | 增加 key usage logs/stats API 封装 |
| `CHANGE_LOGS.md` | 记录本轮二次开发内容和验证结果 |

## 测试计划

### 后端

1. key A 调 `/v1/usage/logs` 只能返回 key A 的 usage_logs；即使传入 `api_key_id` 也不能查询 key B。
2. 无 key、错误 key、禁用 key 沿用现有 API key 鉴权错误。
3. 过期或 quota_exhausted key 仍可访问 `/v1/usage/logs*`，但不能绕过真实模型请求的计费/额度拦截。
4. 日期范围、分页、排序、stats 与列表过滤口径一致。
5. 响应 DTO 不包含 `api_key.key`、account、IP、管理员成本字段。

### 前端

1. `/quota-share` 输入有效 key 后加载额度汇总和明细表。
2. 日期范围变化会刷新明细和 stats。
3. 分页、刷新、空状态、接口报错状态可用。
4. CSV 导出字段正确，特殊字符做安全转义。
5. 刷新页面后 key 不保留。

### 回归

1. 现有 `/usage` 登录用户页不变。
2. 现有 `/v1/usage` 响应不变。
3. 现有 `/admin/usage` 管理员页不变。
4. quota_share 当前 5h/7d 窗口汇总逻辑不变。
