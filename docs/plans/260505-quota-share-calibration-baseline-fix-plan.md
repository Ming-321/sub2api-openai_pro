# quota_share 校准基线与管理页可用性修复方案

## 背景

管理员 `/admin/quota-share` 页面已经可用，但默认分组选择依赖列表顺序；当测试 quota_share 分组 ID 更小时，页面无 `group_id` 参数时容易默认进入测试组，影响正式分组 11 的运营查看。

半自动校准逻辑目前也会在首次看到上游 5h/7d 窗口时直接尝试读取本地采样并生成建议。首次采样缺少可靠基线，容易把历史 `localUSD` 或窗口翻篇前的数据混入第一次建议。需要把首次观测明确变成“建立基线，不生成建议”，并在窗口翻篇或上游百分比明显回落时重建基线、清理旧采样和旧建议。

## 关键决策

- 不自动修改正式 estimated limit；系统仍只生成 pending 建议，管理员确认后才应用。
- 校准逻辑对所有 quota_share 分组生效，验收重点是正式分组 `11 / openai pro`。
- 首次看到 5h/7d 窗口时，仅持久化 `calibration_state` 基线并清空对应窗口本地采样。
- 同一窗口内只有上游百分比正向增加至少 `3%`，且距离上次校准至少 `30 分钟`，才读取 `localUSD` 并计算 pending 建议。
- 上游窗口翻篇或百分比明显回落时重建基线、清理目标窗口 `localUSD`、移除旧 pending 建议。
- Redis 新增 `ResetLocalUSD` 只删除目标窗口采样 key，不影响 key usage、总权重或另一窗口采样。
- 管理员状态接口新增上游账号当前窗口统计，用当前 quota_share group state 的 5h/7d 窗口聚合 `usage_logs.account_id`。
- 前端 `/admin/quota-share` 默认分组优先级为 URL `group_id`、当前管理员隔离的 `localStorage`、列表第一个 quota_share 分组。
- 新增管理员侧边栏入口“配额共享”，保留分组管理页中每行 quota_share 的“共享状态”入口。

## 涉及文件

| 文件 | 修改内容 |
|------|---------|
| `backend/internal/domain/quota_share.go` | 校准窗口状态补充采样开始时间字段 |
| `backend/internal/service/quota_share_service.go` | 重建校准基线、清理旧采样、限制建议生成条件 |
| `backend/internal/repository/quota_share_cache.go` | 增加 `ResetLocalUSD` 实现 |
| `backend/internal/repository/quota_share_cache_test.go` | 覆盖目标窗口采样清理 |
| `backend/internal/service/quota_share_service_test.go` | 覆盖首次基线、增量阈值、窗口翻篇/回落、偏差保护 |
| `backend/internal/repository/usage_log_repo.go` | 如已有接口不足，补充按账号与窗口聚合 usage 统计 |
| `backend/internal/service/account_usage_service.go` | 同步 UsageLogRepository 接口，覆盖 server wire 编译路径 |
| `backend/internal/service/admin_service.go` | `/quota-share-status` 响应增加 `upstream_accounts` |
| `frontend/src/types/index.ts` | 增加上游账号窗口统计类型 |
| `frontend/src/views/admin/QuotaShareView.vue` | 增加分组记忆、URL 同步、上游账号统计区域 |
| `frontend/src/components/layout/AppSidebar.vue` 或现有侧边栏文件 | 增加“配额共享”入口 |
| `frontend/src/i18n/locales/zh.ts` | 补充中文导航文案 |
| `frontend/src/i18n/locales/en.ts` | 补充英文导航文案 |
| `CHANGE_LOGS.md` | 记录本轮修改 |

## 验收标准

- `calibration_state=nil` 时首次 `TryCalibrate` 会持久化 5h/7d 基线、清空对应 `localUSD`，且不生成 pending 建议。
- 同一窗口内百分比增量不足 `3%` 时不生成建议，采样继续累积。
- 同一窗口内百分比增量达到 `3%` 且样本合理时生成 pending 建议，正式 estimated limit 不变化。
- 上游窗口翻篇或百分比回落时重建基线、清空旧采样、清除旧 pending 建议。
- 偏差超过 `3x` 时返回 rejected/insufficient 状态，不应用正式配额。
- `ResetLocalUSD` 只删除目标 `qs:lusd:{group}:{window}`。
- `/admin/groups/:id/quota-share-status` 响应包含 `upstream_accounts`，无绑定账号或窗口未初始化时可返回空列表或零值统计。
- 管理员侧边栏出现“配额共享”入口并跳转 `/admin/quota-share`。
- `/admin/quota-share?group_id=11` 优先使用 query；无 query 时恢复当前管理员上次选择；选择正式分组 11 后刷新仍停留在分组 11。
- `/admin/quota-share` 展示上游账号当前 5h/7d 窗口统计。

## 验证命令

```bash
GOMAXPROCS=1 go test -p 1 ./backend/internal/service -run 'TestQuotaShare' -count=1
GOMAXPROCS=1 go test -p 1 ./backend/internal/repository -run 'Test.*QuotaShare|TestGetAccount.*Window' -count=1
GOMAXPROCS=1 go test -p 1 ./backend/internal/handler/admin -run 'Test.*QuotaShare' -count=1
GOMAXPROCS=1 go test -p 1 ./backend/cmd/server -run '^$' -count=1
NODE_OPTIONS=--max-old-space-size=1024 npm --prefix frontend run typecheck
NODE_OPTIONS=--max-old-space-size=1024 npm --prefix frontend run build
```

在 2c2g 服务器上，上述命令只能逐条单独执行；若编译进程内存占用过高，应先停止并改为人工检查或等待低峰期。
