# 2026-05-04 quota_share 半自动校准与 v0.1.122 升级维护计划

## 背景

当前 `quota_share` 已支持 GPT Pro 订阅按权重分发、窗口同步、DB 用量汇总、自动校准和 overflow 兜底。现有自动校准会在采样满足条件后直接更新分组的 `estimated_5h_limit_usd` / `estimated_7d_limit_usd`，这会让生产配额在无人确认时发生变化。

本轮目标是保留 quota_share 全功能，但把校准改为半自动：系统只生成 pending 建议，管理员确认后才应用到正式 estimated limit。同时补齐低资源回归脚本，并把 upstream `v0.1.122` 合并作为首次升级演练。

## 关键决策

1. `CheckLimits()`、`/quota-share` 用户查询页和管理员状态页的限流计算继续只读取正式 estimated limit，pending 建议不参与限流。
2. `TryCalibrate()` 不再直接写正式 estimated limit；只更新 `groups.calibration_state` 中的 pending 建议和采样状态。
3. pending 建议记录建议值、当前正式值、本地 USD、上游百分比起止、百分比增量、EMA 参数、计算时间、状态和原因。
4. 管理员通过新接口应用或丢弃建议；应用时才写正式 `estimated_5h_limit_usd` / `estimated_7d_limit_usd`。
5. 每日提醒使用前端 `localStorage` / `sessionStorage`，不新增数据库表。`忽略` 当天不再提醒，`下次再通知` 仅关闭当前登录会话提醒。
6. 测试和升级脚本默认面向 2c2g 服务器：Go 串行、前端低内存、只跑定向低资源回归。
7. 合并 upstream `v0.1.122` 时，`openai_chat_completions.go` 冲突同时保留 upstream raw Chat Completions 修复与本地 `activeAPIKey` / `overflowedFromGroupID` usage 记录逻辑。

## 涉及文件

| 文件 | 计划修改 |
| --- | --- |
| `backend/internal/domain/quota_share.go` | 扩展校准状态结构，增加 pending 建议与不可更新原因 |
| `backend/internal/service/quota_share_service.go` | 校准生成 pending，新增读取提醒、应用、丢弃建议逻辑 |
| `backend/internal/repository/group_repo.go` | 增加只更新 calibration_state 的方法，并保留正式配额应用路径 |
| `backend/internal/service/admin_service.go` | 暴露 quota_share 校准状态、应用、丢弃接口 |
| `backend/internal/handler/admin/group_handler.go` | 新增管理 API handler |
| `backend/internal/server/routes/admin.go` | 注册 quota_share 校准管理路由 |
| `frontend/src/api/admin/groups.ts` | 增加校准 API 封装 |
| `frontend/src/types/index.ts` | 增加 pending 校准类型 |
| `frontend/src/views/admin/QuotaShareView.vue` | 增加半自动校准面板与操作按钮 |
| `frontend/src/components/admin/QuotaShareCalibrationReminder.vue` | 新增后台每日提醒组件 |
| `frontend/src/layouts/AdminLayout.vue` | 挂载提醒组件 |
| `scripts/test-quota-share-upgrade.sh` | 新增低资源回归脚本 |
| `Makefile` | 增加低资源回归 target |
| `CHANGE_LOGS.md` | 记录本轮改动、升级结果和回退方式 |

## 升级 Checklist

- auth middleware：确认 quota_share Key 不被余额/订阅检查误拦截。
- billing eligibility：确认 standard/subscription/quota_share 分支互不影响。
- OpenAI gateway：确认 Responses、Messages、Chat Completions、Images usage 记录保留 `activeAPIKey` 与 `overflowed_from_group_id`。
- usage log：确认新增 upstream migration 与本地 overflow/quota_share 字段并存。
- account scheduling：确认账号池不可用时仍可触发 quota_share overflow group。
- frontend router/i18n：确认 `/admin/quota-share`、`/quota-share` 和后台全局提醒可访问。
- Key 管理弹窗：确认 `quota_weight` 与 `quota_share_overflow_group_id` 仍可配置。
- migrations：确认 upstream `134_affiliate_ledger_audit_snapshots.sql` 与本地 `134/135/136` quota_share 迁移不会产生 checksum mismatch。

## 验证计划

1. 后端定向测试：quota_share 限额、DB 用量回退、半自动校准 pending、应用/丢弃接口、overflow 记录。
2. 前端定向测试：`/admin/quota-share` pending 展示、每日提醒忽略/稍后/立即更新、`/quota-share` 不展示 pending。
3. 低资源脚本：`scripts/test-quota-share-upgrade.sh` 默认设置 `GOMAXPROCS=2`、`GOMEMLIMIT=1200MiB`、`GOFLAGS="-p=1 -count=1"`、`NODE_OPTIONS="--max-old-space-size=1024"`。
4. 构建验证：后端定向 build、前端 typecheck/build。
5. 可选 live smoke：仅 `RUN_LIVE_SMOKE=1` 时访问真实服务，默认不消耗 GPT Pro 配额。

## 回退锚点

- 当前分支：`feature/quota-share`
- 当前 HEAD：`66bf851a fix quota share overflow fallback`
- upstream 基线：当前本地 main / origin/main 为 `0.1.121`
- 目标 upstream：`v0.1.122`
- 回退方式：保留当前提交锚点和数据库备份；如上线后异常，回退到本轮升级前镜像和提交，pending 校准字段位于既有 `groups.calibration_state` JSON 中，不影响正式 estimated limit。

