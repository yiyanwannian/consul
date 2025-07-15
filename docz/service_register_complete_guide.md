# Consul 服务注册完整指南

## 概述

本指南提供了 Consul 服务注册机制的完整分析，包括时序图、架构设计和源代码位置。Consul 提供两种主要的服务注册方式：

1. **Agent 服务注册** (`/v1/agent/service/register`) - 通过本地 Agent 注册，支持反熵同步
2. **Catalog 直接注册** (`/v1/catalog/register`) - 直接注册到集群 Catalog

## 时序图

详细的服务注册时序图请参考：[service_register_sequence.puml](./service_register_sequence.puml)

该时序图展示了以下关键流程：
- Agent 服务注册的完整流程
- 反熵同步机制
- Catalog RPC 处理
- FSM 状态机应用
- Catalog 直接注册流程

## 架构分析

完整的架构分析请参考：[service_register_architecture.md](./service_register_architecture.md)

### 核心组件

1. **HTTP 处理层**
   - 端点注册：`agent/http_register.go`
   - Agent 端点：`agent/agent_endpoint.go:1150`
   - Catalog 端点：`agent/catalog_endpoint.go:134`

2. **Agent 本地状态管理**
   - 服务添加：`agent/agent.go:2347`
   - 状态管理：`agent/local/state.go`
   - 反熵同步：`agent/ae/ae.go`

3. **RPC 通信层**
   - Catalog 注册：`agent/consul/catalog_endpoint.go:107`
   - Leader 转发和 ACL 验证

4. **FSM 状态机**
   - Raft 日志应用：`agent/consul/fsm/commands_ce.go:153`
   - 状态转换处理

5. **状态存储层**
   - 事务处理：`agent/consul/state/catalog.go:143`
   - 数据持久化

## 关键代码位置

### HTTP 端点
```go
// agent/http_register.go:59
registerEndpoint("/v1/agent/service/register", []string{"PUT"}, (*HTTPHandlers).AgentRegisterService)

// agent/http_register.go:62  
registerEndpoint("/v1/catalog/register", []string{"PUT"}, (*HTTPHandlers).CatalogRegister)
```

### Agent 服务注册
```go
// agent/agent_endpoint.go:1150
func (s *HTTPHandlers) AgentRegisterService(resp http.ResponseWriter, req *http.Request) (interface{}, error)

// agent/agent.go:2347
func (a *Agent) AddService(req AddServiceRequest) error
```

### Catalog 注册
```go
// agent/catalog_endpoint.go:134
func (s *HTTPHandlers) CatalogRegister(resp http.ResponseWriter, req *http.Request) (interface{}, error)

// agent/consul/catalog_endpoint.go:107
func (c *Catalog) Register(args *structs.RegisterRequest, reply *struct{}) error
```

### FSM 处理
```go
// agent/consul/fsm/commands_ce.go:153
func (c *FSM) applyRegister(buf []byte, index uint64) interface{}
```

### 状态存储
```go
// agent/consul/state/catalog.go:143
func (s *Store) EnsureRegistration(idx uint64, req *structs.RegisterRequest) error
```

## 数据流向

### Agent 注册流程
```
客户端 → HTTP处理器 → Agent → 本地状态 → 反熵同步 → RPC → Catalog → FSM → 状态存储
```

### Catalog 直接注册流程
```
客户端 → HTTP处理器 → RPC → Catalog → FSM → 状态存储
```

## 关键特性

### 1. 反熵同步
- **位置**: `agent/local/state.go:1245`
- **机制**: 定期同步 + 变更触发
- **目的**: 确保本地状态与集群状态一致

### 2. ACL 权限控制
- **Token 解析**: `ResolveTokenAndDefaultMeta()`
- **权限验证**: `vetRegisterWithACL()`
- **企业元数据**: `parseEntMetaNoWildcard()`

### 3. 事务处理
- **原子性**: 单个事务处理节点、服务、检查
- **一致性**: Raft 日志确保集群一致性
- **隔离性**: 写事务隔离

### 4. Leader 转发
- **自动转发**: 非 Leader 节点自动转发到 Leader
- **透明处理**: 客户端无需关心 Leader 位置

## 错误处理

### HTTP 层
- `400 Bad Request`: 请求格式错误
- `403 Forbidden`: ACL 权限不足
- `500 Internal Server Error`: 内部错误

### RPC 层
- Leader 转发失败
- ACL 验证失败
- Raft 应用失败

## 性能优化

### 1. 批量操作
- 支持单次注册多个检查
- 事务性批量处理

### 2. 异步同步
- 反熵同步异步执行
- 不阻塞客户端请求

### 3. 缓存机制
- 本地状态缓存
- ACL 权限缓存

## 监控指标

### HTTP 指标
- `client.api.catalog_register`: 注册请求计数
- `client.api.success.catalog_register`: 成功计数
- `client.rpc.error.catalog_register`: 错误计数

### 处理指标
- `catalog.register`: 注册处理时间
- `fsm.register`: FSM 处理时间

## 最佳实践

### 1. 选择合适的注册方式
- **Agent 注册**: 适用于应用程序直接注册，支持本地缓存和反熵同步
- **Catalog 注册**: 适用于外部系统集成，直接操作集群状态

### 2. 健康检查配置
- 配置合适的检查间隔
- 使用多种检查类型组合
- 设置合理的超时时间

### 3. ACL 权限管理
- 使用最小权限原则
- 为不同服务配置专用 Token
- 定期轮换 Token

### 4. 错误处理
- 实现重试机制
- 监控注册失败率
- 设置告警阈值

## 故障排查

### 1. 注册失败
- 检查 ACL Token 权限
- 验证服务定义格式
- 查看 Consul 日志

### 2. 同步问题
- 检查反熵同步状态
- 验证网络连接
- 查看同步指标

### 3. 性能问题
- 监控注册延迟
- 检查 Raft 日志大小
- 优化批量操作

## 相关文档

- [service_register_sequence.puml](./service_register_sequence.puml) - 详细时序图
- [service_register_architecture.md](./service_register_architecture.md) - 完整架构分析
- [consul_architecture_overview.md](./consul_architecture_overview.md) - Consul 整体架构

## 总结

Consul 的服务注册机制设计精良，支持多种注册方式，具备完善的错误处理、权限控制和性能优化。通过理解其内部实现，可以更好地使用和运维 Consul 集群。

关键设计原则：
- **一致性**: 通过 Raft 确保集群状态一致
- **可用性**: 支持本地缓存和反熵同步
- **安全性**: 完善的 ACL 权限控制
- **性能**: 异步处理和批量操作
- **可观测性**: 丰富的监控指标
