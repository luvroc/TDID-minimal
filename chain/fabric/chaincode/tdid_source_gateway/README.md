# Gateway Contracts (Fabric / FISCO)

本目录包含两个 Go 网关合约实现：

- `fabric_gateway_contract.go`: Hyperledger Fabric 版本（基于 `contractapi`）
- `fisco_gateway_contract.go`: FISCO 版本（通过 `FiscoContext` 接口适配状态与事件）

## 功能

两份合约都提供以下方法：

- `Lock(...)`: 锁定资产，并触发 `Event_Lock`
- `Unlock(...)`: 退款解锁（回滚），并触发 `Event_Refund`
- `Mint(...)`: 铸造映射资产，并触发 `Event_Mint`
- `Burn(...)`: 销毁映射资产

## 事件

- `Event_Lock`
- `Event_Mint`
- `Event_Refund`

## 状态说明

- 交易状态：`LOCKED` / `REFUNDED` / `MINTED` / `BURNED`
- 余额键：`bal:<token>:<account>`
- 交易键：`tx:<id>`
