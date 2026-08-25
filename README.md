# cleanroom-monitor-release

`cleanroom-monitor-release` 是面向洁净室监测工程师、质量复核员和设施合规负责人的合规放行服务。服务以一次监测活动为聚合边界，保存采样方案、校准证据、测量轮次、自动发现、人工裁决、整改补测、冻结清单、放行凭据与顺序审计事件。

## 业务流程

活动按以下状态单向推进：

`Draft -> Ready -> Executing -> ReviewPending -> Remediation -> Executing -> ReviewPending -> Frozen -> Certified`

- `Draft`：创建区域、洁净等级、计划日期、采样点、单位、重复次数与阈值；服务会规范化文本、校验跨点位单位和容量，并返回 `planSummary`。
- `Ready`：校准期覆盖计划时刻，且有效仪器覆盖全部计划指标；分批登记时由 `readiness` 返回已覆盖与缺失指标。
- `Executing`：提交不可变测量轮次；服务原子校验来源与整批样本，并通过 `samplingProgress` 返回点位重复序号缺口和总体完成比例。
- `ReviewPending`：结束采样后生成确定性检查批次，形成缺少重复测量、低于下限或高于上限的结构化发现项。
- `Remediation`：复核员逐项裁决后，工程师提交关联原轮次且完整覆盖目标点位的整改补测；无关发现不会被一并关闭。
- `Frozen`：所有发现均通过或已由补测闭环后，先执行只读预检，再以候选版本和候选 `manifestHash` 原子冻结并禁止证据改写。
- `Certified`：设施合规负责人签发绑定冻结版本和清单哈希的不可变放行凭据。

所有写请求都使用 `idempotencyKey`；除创建外还必须提供当前 `expectedVersion`。同一幂等键和相同请求会重放首次结果，同一键对应不同请求会返回冲突。角色标识为 `monitoring_engineer`、`quality_reviewer` 和 `facility_compliance`。

## 构建

要求 Go 1.22 或更高版本。

```text
go build ./cmd/server
```

## 运行

默认监听高位回环地址 `127.0.0.1:19091`，数据库默认为当前目录的 `cleanroom-monitor.db`：

```text
go run ./cmd/server
```

显式指定监听地址和数据库：

```text
go run ./cmd/server -addr=127.0.0.1:19110 -db=./monitor.db
```

未传 `-addr` 时，可以通过 `PORT` 指定端口号，服务会绑定到 `127.0.0.1:<PORT>`。显式 `-addr` 的优先级高于 `PORT`。

## 测试与自检

运行全部回归测试：

```text
go test ./...
```

有界 selfcheck 会创建内存 SQLite 数据库、启动真实回环监听，并通过 HTTP 完成建档、仪器就绪、采样、自动检查、冻结、签发、凭据验证和审计查询，然后主动关闭服务：

```text
go run ./cmd/server -selfcheck -addr=127.0.0.1:19091
```

## HTTP API

所有请求与响应使用 JSON。写请求的 `Content-Type` 必须是 `application/json`，未知 JSON 字段会被拒绝。错误响应包含稳定的 `error.code`、中文 `error.message` 和 `error.requestId`，同时响应头返回 `X-Request-ID`。

主要路由：

- `POST /api/v1/campaigns`：创建活动与采样方案。
- `POST /api/v1/campaigns/{campaignId}/instruments`：登记校准证据并检查指标覆盖。
- `POST /api/v1/campaigns/{campaignId}/rounds`：提交日常采样或整改补测轮次。
- `POST /api/v1/campaigns/{campaignId}/submit-review`：结束采样、执行自动检查并提交复核。
- `GET /api/v1/campaigns/{campaignId}/findings`：按当前检查批次、待裁决集合和历史集合查询结构化发现。
- `POST /api/v1/campaigns/{campaignId}/findings/{findingId}/decision`：裁决单个发现项。
- `GET /api/v1/campaigns/{campaignId}/freeze/preflight?candidateVersion=<version>`：只读返回全部冻结阻断项；无阻断项时返回候选哈希和证据计数。
- `POST /api/v1/campaigns/{campaignId}/freeze`：携带与 `expectedVersion` 相同的 `candidateVersion` 及预检 `manifestHash`，冻结同一证据修订。
- `POST /api/v1/campaigns/{campaignId}/credential`：签发放行凭据。
- `GET /api/v1/campaigns/{campaignId}`：查询活动完整视图。
- `GET /api/v1/campaigns/{campaignId}/timeline`：查询按单调序号排列的审计时间线。
- `GET /api/v1/campaigns/{campaignId}/credential/verify`：只读重新计算清单和凭据摘要，返回活动、冻结版本和清单绑定、清单完整性、凭据摘要的稳定分项结果。
- `GET /healthz`：进程健康检查。

SQLite 在单个短写事务中原子保存聚合版本、规范化证据表、检查批次、幂等结果和审计事件。凭据校验还会比对聚合快照与规范化证据表，定位隔离数据库副本中的证据或摘要篡改。服务启动时执行版本化迁移，并拒绝高于当前程序支持范围的数据库 schemaVersion。
