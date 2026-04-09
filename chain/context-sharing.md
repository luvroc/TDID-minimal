# Context Sharing

这份文档用于后续开发时快速同步链上仓库的上下文，重点说明当前 `TDID` 链上部分保留了什么、哪些文件应作为事实源、以及它和链下 `TEE` 侧代码如何对接。

## 目标

- 帮助链上开发者和 `TEE` / relay 开发者共享同一套协议语境
- 明确当前应优先参考的合约、链码、脚本和规范文件，避免继续依赖旧版遗留材料

## 当前事实源

当前链上实现建议优先参考以下位置：

1. `deployments/{env}/chain.yaml`
2. `artifacts/index.json` 及相关部署索引
3. `services/state-query`
4. `services/event-relay`
5. `services/proof-service`
6. 当前仓库中的 Fabric / FISCO 合约与脚本

如果这些位置之间出现冲突，应以“当前可执行脚本 + 最新合约源码 + 实际部署配置”三者共同交叉确认。

## 协议与合约要点

当前协议主线应围绕以下对象来理解：

- 身份侧使用 TDID 注册与治理相关合约
- SessionKey 负责 session 级公钥注册与查询
- Gateway 负责跨链主路径中的 `lock`、`mintOrUnlock`、`refund`
- V2 路径统一围绕 `transferId`、`sessionId`、`traceId`
- 关键事件通常表现为 `LockCreated`、`SettleCommitted`、`RefundExecuted`
- 目标是保持 source / target 两侧状态机和字段语义一致

## 共享数据规范

链上与链下之间当前最需要保持一致的是以下内容：

- Fabric / FISCO 两侧 payload hash 的构造方式
- FISCO compact ABI 的映射口径
- proof 负载中的字段命名与摘要计算方式
- canonical DTO 在不同语言和不同组件中的序列化一致性

## 与链下 / TEE 协同

- 链下会调用 FISCO / Fabric 对应网关完成主路径交易
- 链上字段设计需要与 session 绑定和 proof 验证路径保持一致
- 当前 proof 路径虽然仍是工程化实现，但已不再把 proof service 视为唯一事实源，source gateway 本身也承担证明生成职责
- 如果调整 Fabric / FISCO 任一侧字段或事件，必须同步检查 relay / enclave 对应解析逻辑

## 建议保留的关联文档

- `context-sharing.md`
- `docs/context.md`
- `docs/CODE_EXECUTION_PATHS_20260329.md`
- `docs/OFFCHAIN_SERVER_HANDOFF_20260329.md`
- `docs/FISCO_COMPACT_ABI_MAPPING_20260329.md`
- `docs/TRANSFER_ID_CANONICAL_SPEC.md`

## 备注

- `2026-03-28` 之前的大量阶段性 agent 文档和手工记录已经不再适合作为主要上下文来源
- 与旧版 receipt / audit 机制强绑定的材料目前只保留最小背景意义，不应再作为当前主协议实现的核心依据
