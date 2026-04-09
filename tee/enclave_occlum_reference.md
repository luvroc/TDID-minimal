# TEE Enclave Boundary Reference

这份文档用于说明当前 `TDID-Final` 中哪些能力应放在 Occlum Enclave 内执行，哪些仍属于 host 侧职责。

## 当前属于 Enclave 的部分

下面这些组件承载了密钥、session、摘要与 sealed state 等安全敏感逻辑，当前应被视为 enclave 内核心：

- `tee/key_manager_impl.go`
- `tee/signer_impl.go`
- `tee/session_manager_impl.go`
- `tee/hasher_impl.go`
- `tee/state_store_sealed_file.go`
- `tee/state.go`

这些组件共同承担：

- session 密钥管理
- 签名与摘要生成
- sealed state 持久化与恢复
- enclave 边界内的状态一致性维护

## 当前属于 Host 的部分

下面这些职责当前仍由 host 侧承担：

- 对外 RPC / HTTP 暴露
- 链上客户端调用
- 网络转发与对端通信
- 编排与工作流驱动
- 运行时部署、日志和运维控制

## 最小自检方式

```bash
cd /path/to/tdid-open-minimal/tee
go test ./... -count=1
```

当前这条自检路径主要用于确认：

- session 相关逻辑没有回退
- sealed state 读写仍然正常
- enclave boundary 的主要接口没有断裂

## 当前边界说明

- enclave 侧对外通过受控 RPC / HTTP 边界提供能力
- host 不应直接持有 `NodeKeyBlob` 或 `SessionKeyBlob` 的可导出明文语义
- sealed state 应继续被视为 enclave 边界内资产，而不是普通宿主文件
