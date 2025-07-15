// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package consul

import (
	"fmt"
	"sort"

	"github.com/armon/go-metrics"
	hashstructure_v2 "github.com/mitchellh/hashstructure/v2"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/agent/configentry"
	"github.com/hashicorp/consul/agent/consul/state"
	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/go-bexpr"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-memdb"
)

// Health endpoint is used to query the health information
type Health struct {
	srv    *Server
	logger hclog.Logger
}

// ChecksInState is used to get all the checks in a given state
func (h *Health) ChecksInState(args *structs.ChecksInStateRequest,
	reply *structs.IndexedHealthChecks) error {
	if done, err := h.srv.ForwardRPC("Health.ChecksInState", args, reply); done {
		return err
	}

	filter, err := bexpr.CreateFilter(args.Filter, nil, reply.HealthChecks)
	if err != nil {
		return err
	}

	_, err = h.srv.ResolveTokenAndDefaultMeta(args.Token, &args.EnterpriseMeta, nil)
	if err != nil {
		return err
	}

	if err := h.srv.validateEnterpriseRequest(&args.EnterpriseMeta, false); err != nil {
		return err
	}

	return h.srv.blockingQuery(
		&args.QueryOptions,
		&reply.QueryMeta,
		func(ws memdb.WatchSet, state *state.Store) error {
			var index uint64
			var checks structs.HealthChecks
			var err error
			if len(args.NodeMetaFilters) > 0 {
				index, checks, err = state.ChecksInStateByNodeMeta(ws, args.State, args.NodeMetaFilters, &args.EnterpriseMeta, args.PeerName)
			} else {
				index, checks, err = state.ChecksInState(ws, args.State, &args.EnterpriseMeta, args.PeerName)
			}
			if err != nil {
				return err
			}
			reply.Index, reply.HealthChecks = index, checks

			// Note: we filter the results with ACLs *before* applying the user-supplied
			// bexpr filter to ensure that the user can only run expressions on data that
			// they have access to.  This is a security measure to prevent users from
			// running arbitrary expressions on data they don't have access to.
			// QueryMeta.ResultsFilteredByACLs being true already indicates to the user
			// that results they don't have access to have been removed.  If they were
			// also allowed to run the bexpr filter on the data, they could potentially
			// infer the specific attributes of data they don't have access to.
			if err := h.srv.filterACL(args.Token, reply); err != nil {
				return err
			}

			raw, err := filter.Execute(reply.HealthChecks)
			if err != nil {
				return err
			}
			reply.HealthChecks = raw.(structs.HealthChecks)

			return h.srv.sortNodesByDistanceFrom(args.Source, reply.HealthChecks)
		})
}

// NodeChecks is used to get all the checks for a node
func (h *Health) NodeChecks(args *structs.NodeSpecificRequest,
	reply *structs.IndexedHealthChecks) error {
	if done, err := h.srv.ForwardRPC("Health.NodeChecks", args, reply); done {
		return err
	}

	filter, err := bexpr.CreateFilter(args.Filter, nil, reply.HealthChecks)
	if err != nil {
		return err
	}

	_, err = h.srv.ResolveTokenAndDefaultMeta(args.Token, &args.EnterpriseMeta, nil)
	if err != nil {
		return err
	}

	if err := h.srv.validateEnterpriseRequest(&args.EnterpriseMeta, false); err != nil {
		return err
	}

	return h.srv.blockingQuery(
		&args.QueryOptions,
		&reply.QueryMeta,
		func(ws memdb.WatchSet, state *state.Store) error {
			index, checks, err := state.NodeChecks(ws, args.Node, &args.EnterpriseMeta, args.PeerName)
			if err != nil {
				return err
			}
			reply.Index, reply.HealthChecks = index, checks

			// Note: we filter the results with ACLs *before* applying the user-supplied
			// bexpr filter to ensure that the user can only run expressions on data that
			// they have access to.  This is a security measure to prevent users from
			// running arbitrary expressions on data they don't have access to.
			// QueryMeta.ResultsFilteredByACLs being true already indicates to the user
			// that results they don't have access to have been removed.  If they were
			// also allowed to run the bexpr filter on the data, they could potentially
			// infer the specific attributes of data they don't have access to.
			if err := h.srv.filterACL(args.Token, reply); err != nil {
				return err
			}

			raw, err := filter.Execute(reply.HealthChecks)
			if err != nil {
				return err
			}
			reply.HealthChecks = raw.(structs.HealthChecks)

			return nil
		})
}

// ServiceChecks is used to get all the checks for a service
func (h *Health) ServiceChecks(args *structs.ServiceSpecificRequest,
	reply *structs.IndexedHealthChecks) error {

	// Reject if tag filtering is on
	if args.TagFilter {
		return fmt.Errorf("Tag filtering is not supported")
	}

	// Potentially forward
	if done, err := h.srv.ForwardRPC("Health.ServiceChecks", args, reply); done {
		return err
	}

	filter, err := bexpr.CreateFilter(args.Filter, nil, reply.HealthChecks)
	if err != nil {
		return err
	}

	_, err = h.srv.ResolveTokenAndDefaultMeta(args.Token, &args.EnterpriseMeta, nil)
	if err != nil {
		return err
	}

	if err := h.srv.validateEnterpriseRequest(&args.EnterpriseMeta, false); err != nil {
		return err
	}

	return h.srv.blockingQuery(
		&args.QueryOptions,
		&reply.QueryMeta,
		func(ws memdb.WatchSet, state *state.Store) error {
			var index uint64
			var checks structs.HealthChecks
			var err error
			if len(args.NodeMetaFilters) > 0 {
				index, checks, err = state.ServiceChecksByNodeMeta(ws, args.ServiceName, args.NodeMetaFilters, &args.EnterpriseMeta, args.PeerName)
			} else {
				index, checks, err = state.ServiceChecks(ws, args.ServiceName, &args.EnterpriseMeta, args.PeerName)
			}
			if err != nil {
				return err
			}
			reply.Index, reply.HealthChecks = index, checks

			raw, err := filter.Execute(reply.HealthChecks)
			if err != nil {
				return err
			}
			reply.HealthChecks = raw.(structs.HealthChecks)

			// Note: we filter the results with ACLs *after* applying the user-supplied
			// bexpr filter, to ensure QueryMeta.ResultsFilteredByACLs does not include
			// results that would be filtered out even if the user did have permission.
			if err := h.srv.filterACL(args.Token, reply); err != nil {
				return err
			}

			return h.srv.sortNodesByDistanceFrom(args.Source, reply.HealthChecks)
		})
}

// ServiceNodes 返回作为服务一部分注册的所有节点，包括健康信息
func (h *Health) ServiceNodes(args *structs.ServiceSpecificRequest, reply *structs.IndexedCheckServiceNodes) error {
	// 如果当前节点不是 Leader，将请求转发到 Leader 节点
	// 这确保了所有读取操作的一致性
	if done, err := h.srv.ForwardRPC("Health.ServiceNodes", args, reply); done {
		return err
	}

	// 验证请求参数
	if args.ServiceName == "" {
		return fmt.Errorf("Must provide service name")
	}

	// 根据请求类型确定要调用的查询函数
	var f func(memdb.WatchSet, *state.Store, *structs.ServiceSpecificRequest) (uint64, structs.CheckServiceNodes, error)
	switch {
	case args.Connect:
		// Connect 代理服务查询
		f = h.serviceNodesConnect
	case args.TagFilter:
		// 带标签过滤的服务查询
		f = h.serviceNodesTagFilter
	case args.Ingress:
		// Ingress 网关服务查询
		f = h.serviceNodesIngress
	default:
		// 默认的普通服务查询
		f = h.serviceNodesDefault
	}

	// 构建 ACL 授权上下文，包含对等节点信息
	authzContext := acl.AuthorizerContext{
		Peer: args.PeerName,
	}
	// 解析 ACL 令牌并获取授权器，同时设置默认的企业版元数据
	authz, err := h.srv.ResolveTokenAndDefaultMeta(args.Token, &args.EnterpriseMeta, &authzContext)
	if err != nil {
		return err
	}

	// 验证企业版请求的有效性（命名空间、分区等）
	if err := h.srv.validateEnterpriseRequest(&args.EnterpriseMeta, false); err != nil {
		return err
	}

	// If we're doing a connect or ingress query, we need read access to the service
	// we're trying to find proxies for, so check that.
	if args.Connect || args.Ingress {
		if authz.ServiceRead(args.ServiceName, &authzContext) != acl.Allow {
			return acl.ErrPermissionDenied
		}
	}

	// 创建 bexpr 过滤器，用于复杂的表达式过滤
	filter, err := bexpr.CreateFilter(args.Filter, nil, reply.Nodes)
	if err != nil {
		return err
	}

	// 用于中央配置合并的哈希值比较，避免虚假唤醒
	var (
		priorMergeHash uint64  // 上次合并的哈希值
		ranMergeOnce   bool    // 是否已运行过一次合并
	)

	// 执行阻塞查询，支持长轮询机制
	err = h.srv.blockingQuery(
		&args.QueryOptions,    // 查询选项（等待时间、索引等）
		&reply.QueryMeta,      // 查询元数据（索引、联系时间等）
		func(ws memdb.WatchSet, state *state.Store) error {
			// 创建本次查询的响应结构体
			var thisReply structs.IndexedCheckServiceNodes

			// 获取相似组成员的参数（企业版功能，用于故障转移）
			sgIdx, sgArgs, err := h.getArgsForSamenessGroupMembers(args, ws, state)
			if err != nil {
				return err
			}

			// 遍历所有相似组参数（通常只有一个，除非使用相似组）
			for _, arg := range sgArgs {
				// 调用具体的查询函数获取服务节点
				index, nodes, err := f(ws, state, arg)
				if err != nil {
					return err
				}

				// 获取解析后的节点列表
				resolvedNodes := nodes
				// 如果需要合并中央配置
				if arg.MergeCentralConfig {
					// 遍历所有节点，为 Sidecar 代理和网关合并中央配置
					for _, node := range resolvedNodes {
						ns := node.Service
						// 只有 Sidecar 代理或网关需要合并中央配置
						if ns.IsSidecarProxy() || ns.IsGateway() {
							// 从配置条目中合并中央配置到节点服务
							cfgIndex, mergedns, err := configentry.MergeNodeServiceWithCentralConfig(ws, state, ns, h.logger)
							if err != nil {
								return err
							}
							// 如果配置索引更大，更新总索引
							if cfgIndex > index {
								index = cfgIndex
							}
							// 用合并后的服务配置替换原始配置
							*node.Service = *mergedns
						}
					}

					// 生成解析节点的哈希值，用于驱动此响应
					// 使用它来确定响应是否与之前的唤醒相同（避免虚假唤醒）
					newMergeHash, err := hashstructure_v2.Hash(resolvedNodes, hashstructure_v2.FormatV2, nil)
					if err != nil {
						return fmt.Errorf("error hashing reply for spurious wakeup suppression: %w", err)
					}
					// 如果已运行过一次且哈希值相同，说明数据未变化
					if ranMergeOnce && priorMergeHash == newMergeHash {
						// 下面的赋值不是必需的，因为 if 条件已经验证了相等性，
						// 但使得每次运行时先前值被重置为新哈希更加清晰
						priorMergeHash = newMergeHash
						reply.Index = index
						// 注意：先前的响应仍然在 *reply 中存活，这是期望的
						return errNotChanged
					} else {
						// 更新哈希值并标记已运行过一次
						priorMergeHash = newMergeHash
						ranMergeOnce = true
					}

				}

				// 设置本次查询的索引和节点列表
				thisReply.Index, thisReply.Nodes = index, resolvedNodes

				// 如果有节点元数据过滤器，应用过滤
				if len(arg.NodeMetaFilters) > 0 {
					thisReply.Nodes = nodeMetaFilter(arg.NodeMetaFilters, thisReply.Nodes)
				}

				// 注意：我们在应用用户提供的 bexpr 过滤器*之前*使用 ACL 过滤结果，
				// 以确保用户只能在他们有权访问的数据上运行表达式。
				// 这是一个安全措施，防止用户在他们无权访问的数据上运行任意表达式。
				// QueryMeta.ResultsFilteredByACLs 为 true 已经向用户表明
				// 他们无权访问的结果已被移除。如果他们也被允许在数据上运行 bexpr 过滤器，
				// 他们可能会推断出他们无权访问的数据的特定属性。
				if err := h.srv.filterACL(arg.Token, &thisReply); err != nil {
					return err
				}

				// 执行用户提供的 bexpr 表达式过滤
				raw, err := filter.Execute(thisReply.Nodes)
				if err != nil {
					return err
				}
				// 将过滤结果转换为正确的类型
				filteredNodes := raw.(structs.CheckServiceNodes)
				// 根据健康过滤类型进一步过滤节点（只保留健康的或包含所有状态）
				thisReply.Nodes = filteredNodes.Filter(structs.CheckServiceNodeFilterOptions{FilterType: arg.HealthFilterType})

				// 根据网络距离对节点进行排序，最近的节点排在前面
				if err := h.srv.sortNodesByDistanceFrom(arg.Source, thisReply.Nodes); err != nil {
					return err
				}
				// 如果找到了节点，跳出循环（用于相似组故障转移）
				if len(thisReply.Nodes) > 0 {
					break
				}
			}

			// If sameness group was used, evaluate the index of the sameness group
			// and update the index of the response if it is greater.  If sameness group is not
			// used, the sgIdx will be 0 in this evaluation.
			if sgIdx > thisReply.Index {
				thisReply.Index = sgIdx
			}

			*reply = thisReply
			return nil
		})

	// Provide some metrics
	if err == nil {
		// For metrics, we separate Connect-based lookups from non-Connect
		key := "service"
		if args.Connect {
			key = "connect"
		}
		if args.Ingress {
			key = "ingress"
		}

		metrics.IncrCounterWithLabels([]string{"health", key, "query"}, 1,
			[]metrics.Label{{Name: "service", Value: args.ServiceName}})
		// DEPRECATED (singular-service-tag) - remove this when backwards RPC compat
		// with 1.2.x is not required.
		if args.ServiceTag != "" {
			metrics.IncrCounterWithLabels([]string{"health", key, "query-tag"}, 1,
				[]metrics.Label{{Name: "service", Value: args.ServiceName}, {Name: "tag", Value: args.ServiceTag}})
		}
		if len(args.ServiceTags) > 0 {
			// Sort tags so that the metric is the same even if the request
			// tags are in a different order
			sort.Strings(args.ServiceTags)

			labels := []metrics.Label{{Name: "service", Value: args.ServiceName}}
			for _, tag := range args.ServiceTags {
				labels = append(labels, metrics.Label{Name: "tag", Value: tag})
			}
			metrics.IncrCounterWithLabels([]string{"health", key, "query-tags"}, 1, labels)
		}
		if len(reply.Nodes) == 0 {
			metrics.IncrCounterWithLabels([]string{"health", key, "not-found"}, 1,
				[]metrics.Label{{Name: "service", Value: args.ServiceName}})
		}
	}
	return err
}

// The serviceNodes* functions below are the various lookup methods that
// can be used by the ServiceNodes endpoint.

func (h *Health) serviceNodesConnect(ws memdb.WatchSet, s *state.Store, args *structs.ServiceSpecificRequest) (uint64, structs.CheckServiceNodes, error) {
	return s.CheckConnectServiceNodes(ws, args.ServiceName, &args.EnterpriseMeta, args.PeerName)
}

func (h *Health) serviceNodesIngress(ws memdb.WatchSet, s *state.Store, args *structs.ServiceSpecificRequest) (uint64, structs.CheckServiceNodes, error) {
	return s.CheckIngressServiceNodes(ws, args.ServiceName, &args.EnterpriseMeta)
}

func (h *Health) serviceNodesTagFilter(ws memdb.WatchSet, s *state.Store, args *structs.ServiceSpecificRequest) (uint64, structs.CheckServiceNodes, error) {
	// DEPRECATED (singular-service-tag) - remove this when backwards RPC compat
	// with 1.2.x is not required.
	// Agents < v1.3.0 populate the ServiceTag field. In this case,
	// use ServiceTag instead of the ServiceTags field.
	if args.ServiceTag != "" {
		return s.CheckServiceTagNodes(ws, args.ServiceName, []string{args.ServiceTag}, &args.EnterpriseMeta, args.PeerName)
	}
	return s.CheckServiceTagNodes(ws, args.ServiceName, args.ServiceTags, &args.EnterpriseMeta, args.PeerName)
}

func (h *Health) serviceNodesDefault(ws memdb.WatchSet, s *state.Store, args *structs.ServiceSpecificRequest) (uint64, structs.CheckServiceNodes, error) {
	return s.CheckServiceNodes(ws, args.ServiceName, &args.EnterpriseMeta, args.PeerName)
}
