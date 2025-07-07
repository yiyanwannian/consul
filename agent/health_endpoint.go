// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package agent

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/internal/dnsutil"
)

const (
	serviceHealth = "service"
	connectHealth = "connect"
	ingressHealth = "ingress"
)

func (s *HTTPHandlers) HealthChecksInState(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	// Set default DC
	args := structs.ChecksInStateRequest{}
	if err := s.parseEntMeta(req, &args.EnterpriseMeta); err != nil {
		return nil, err
	}
	s.parseSource(req, &args.Source)
	args.NodeMetaFilters = s.parseMetaFilter(req)
	if done := s.parse(resp, req, &args.Datacenter, &args.QueryOptions); done {
		return nil, nil
	}

	// Pull out the service name
	args.State = strings.TrimPrefix(req.URL.Path, "/v1/health/state/")
	if args.State == "" {
		return nil, HTTPError{StatusCode: http.StatusBadRequest, Reason: "Missing check state"}
	}

	// Make the RPC request
	var out structs.IndexedHealthChecks
	defer setMeta(resp, &out.QueryMeta)
RETRY_ONCE:
	if err := s.agent.RPC(req.Context(), "Health.ChecksInState", &args, &out); err != nil {
		return nil, err
	}
	if args.QueryOptions.AllowStale && args.MaxStaleDuration > 0 && args.MaxStaleDuration < out.LastContact {
		args.AllowStale = false
		args.MaxStaleDuration = 0
		goto RETRY_ONCE
	}
	out.ConsistencyLevel = args.QueryOptions.ConsistencyLevel()

	// Use empty list instead of nil
	if out.HealthChecks == nil {
		out.HealthChecks = make(structs.HealthChecks, 0)
	}
	for i, c := range out.HealthChecks {
		if c.ServiceTags == nil {
			clone := *c
			clone.ServiceTags = make([]string, 0)
			out.HealthChecks[i] = &clone
		}
	}
	return out.HealthChecks, nil
}

func (s *HTTPHandlers) HealthNodeChecks(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	// Set default DC
	args := structs.NodeSpecificRequest{}
	if err := s.parseEntMeta(req, &args.EnterpriseMeta); err != nil {
		return nil, err
	}
	if done := s.parse(resp, req, &args.Datacenter, &args.QueryOptions); done {
		return nil, nil
	}

	// Pull out the service name
	args.Node = strings.TrimPrefix(req.URL.Path, "/v1/health/node/")
	if args.Node == "" {
		return nil, HTTPError{StatusCode: http.StatusBadRequest, Reason: "Missing node name"}
	}

	// Make the RPC request
	var out structs.IndexedHealthChecks
	defer setMeta(resp, &out.QueryMeta)
RETRY_ONCE:
	if err := s.agent.RPC(req.Context(), "Health.NodeChecks", &args, &out); err != nil {
		return nil, err
	}
	if args.QueryOptions.AllowStale && args.MaxStaleDuration > 0 && args.MaxStaleDuration < out.LastContact {
		args.AllowStale = false
		args.MaxStaleDuration = 0
		goto RETRY_ONCE
	}
	out.ConsistencyLevel = args.QueryOptions.ConsistencyLevel()

	// Use empty list instead of nil
	if out.HealthChecks == nil {
		out.HealthChecks = make(structs.HealthChecks, 0)
	}
	for i, c := range out.HealthChecks {
		if c.ServiceTags == nil {
			clone := *c
			clone.ServiceTags = make([]string, 0)
			out.HealthChecks[i] = &clone
		}
	}
	return out.HealthChecks, nil
}

func (s *HTTPHandlers) HealthServiceChecks(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	// Set default DC
	args := structs.ServiceSpecificRequest{}
	if err := s.parseEntMetaNoWildcard(req, &args.EnterpriseMeta); err != nil {
		return nil, err
	}
	s.parseSource(req, &args.Source)
	args.NodeMetaFilters = s.parseMetaFilter(req)
	if done := s.parse(resp, req, &args.Datacenter, &args.QueryOptions); done {
		return nil, nil
	}

	// Pull out the service name
	args.ServiceName = strings.TrimPrefix(req.URL.Path, "/v1/health/checks/")
	if args.ServiceName == "" {
		return nil, HTTPError{StatusCode: http.StatusBadRequest, Reason: "Missing service name"}
	}

	// Make the RPC request
	var out structs.IndexedHealthChecks
	defer setMeta(resp, &out.QueryMeta)
RETRY_ONCE:
	if err := s.agent.RPC(req.Context(), "Health.ServiceChecks", &args, &out); err != nil {
		return nil, err
	}
	if args.QueryOptions.AllowStale && args.MaxStaleDuration > 0 && args.MaxStaleDuration < out.LastContact {
		args.AllowStale = false
		args.MaxStaleDuration = 0
		goto RETRY_ONCE
	}
	out.ConsistencyLevel = args.QueryOptions.ConsistencyLevel()

	// Use empty list instead of nil
	if out.HealthChecks == nil {
		out.HealthChecks = make(structs.HealthChecks, 0)
	}
	for i, c := range out.HealthChecks {
		if c.ServiceTags == nil {
			clone := *c
			clone.ServiceTags = make([]string, 0)
			out.HealthChecks[i] = &clone
		}
	}
	return out.HealthChecks, nil
}

// HealthIngressServiceNodes should return "all the healthy ingress gateway instances
// that I can use to access this connect-enabled service without mTLS".
func (s *HTTPHandlers) HealthIngressServiceNodes(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	return s.healthServiceNodes(resp, req, ingressHealth)
}

// HealthConnectServiceNodes should return "all healthy connect-enabled
// endpoints (e.g. could be side car proxies or native instances) for this
// service so I can connect with mTLS".
func (s *HTTPHandlers) HealthConnectServiceNodes(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	return s.healthServiceNodes(resp, req, connectHealth)
}

// HealthServiceNodes 应该返回"此服务注册的所有健康实例，
// 以便我可以直接连接到它们"
func (s *HTTPHandlers) HealthServiceNodes(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	// 调用通用的健康服务节点处理方法，指定为普通服务健康查询
	return s.healthServiceNodes(resp, req, serviceHealth)
}

// healthServiceNodes 是所有健康服务节点查询的通用处理方法
func (s *HTTPHandlers) healthServiceNodes(resp http.ResponseWriter, req *http.Request, healthType string) (interface{}, error) {
	// 创建服务特定请求结构体，设置默认数据中心
	args := structs.ServiceSpecificRequest{}
	// 解析企业版元数据（命名空间、分区等），不允许通配符
	if err := s.parseEntMetaNoWildcard(req, &args.EnterpriseMeta); err != nil {
		return nil, err
	}
	// 解析请求来源信息，用于网络坐标计算
	s.parseSource(req, &args.Source)
	// 解析节点元数据过滤器
	args.NodeMetaFilters = s.parseMetaFilter(req)
	// 解析通用查询参数（数据中心、查询选项等）
	if done := s.parse(resp, req, &args.Datacenter, &args.QueryOptions); done {
		return nil, nil
	}

	// 解析对等节点名称（用于集群对等连接）
	s.parsePeerName(req, &args)
	// 解析相似组名称（企业版功能，用于故障转移）
	s.parseSamenessGroup(req, &args)
	// 对等名称和相似组是互斥的，不能同时指定
	if args.SamenessGroup != "" && args.PeerName != "" {
		return nil, HTTPError{StatusCode: http.StatusBadRequest, Reason: "peer-name and sameness-group are mutually exclusive"}
	}

	// 检查标签过滤参数
	params := req.URL.Query()
	if _, ok := params["tag"]; ok {
		// 获取所有标签参数值
		args.ServiceTags = params["tag"]
		// 标记启用标签过滤
		args.TagFilter = true
	}

	// 检查是否需要合并中央配置
	if _, ok := params["merge-central-config"]; ok {
		args.MergeCentralConfig = true
	}

	// 根据健康检查类型确定 URL 前缀
	var prefix string
	switch healthType {
	case connectHealth:
		// Connect 代理服务的健康检查
		prefix = "/v1/health/connect/"
		args.Connect = true
	case ingressHealth:
		// Ingress 网关的健康检查
		prefix = "/v1/health/ingress/"
		args.Ingress = true
	default:
		// serviceHealth 是默认类型，普通服务健康检查
		prefix = "/v1/health/service/"
	}

	// 从 URL 路径中解析服务名称
	args.ServiceName = strings.TrimPrefix(req.URL.Path, prefix)
	if args.ServiceName == "" {
		return nil, HTTPError{StatusCode: http.StatusBadRequest, Reason: "Missing service name"}
	}

	// 从查询参数中解析 passing 标志，用于设置健康过滤类型
	// 如果存在则设置为 HealthFilterIncludeOnlyPassing，否则不按健康状态过滤
	passing, err := getBoolQueryParam(params, api.HealthPassing)
	if err != nil {
		return nil, HTTPError{StatusCode: http.StatusBadRequest, Reason: "Invalid value for ?passing"}
	}
	// 默认包含所有健康状态的服务实例
	healthFilterType := structs.HealthFilterIncludeAll
	if passing {
		// 如果指定了 passing=true，只包含健康状态为 passing 的实例
		healthFilterType = structs.HealthFilterIncludeOnlyPassing
	}
	args.HealthFilterType = healthFilterType

	// 调用 RPC 客户端执行健康服务节点查询
	out, md, err := s.agent.rpcClientHealth.ServiceNodes(req.Context(), args)
	if err != nil {
		return nil, err
	}

	// 如果启用了缓存，设置缓存相关的响应头
	if args.QueryOptions.UseCache {
		setCacheMeta(resp, &md)
	}
	// 设置一致性级别到查询元数据
	out.QueryMeta.ConsistencyLevel = args.QueryOptions.ConsistencyLevel()
	// 设置查询元数据到 HTTP 响应头
	_ = setMeta(resp, &out.QueryMeta)

	// 在过滤后翻译地址，这样我们不会浪费精力
	s.agent.TranslateAddresses(args.Datacenter, out.Nodes, dnsutil.TranslateAddressAcceptAny)

	// 使用空列表而不是 nil，确保 JSON 序列化为空数组而不是 null
	if out.Nodes == nil {
		out.Nodes = make(structs.CheckServiceNodes, 0)
	}
	for i := range out.Nodes {
		if out.Nodes[i].Checks == nil {
			out.Nodes[i].Checks = make(structs.HealthChecks, 0)
		}
		for j, c := range out.Nodes[i].Checks {
			if c.ServiceTags == nil {
				clone := *c
				clone.ServiceTags = make([]string, 0)
				out.Nodes[i].Checks[j] = &clone
			}
		}
		if out.Nodes[i].Service != nil && out.Nodes[i].Service.Tags == nil {
			clone := *out.Nodes[i].Service
			clone.Tags = make([]string, 0)
			out.Nodes[i].Service = &clone
		}
	}
	return out.Nodes, nil
}

func getBoolQueryParam(params url.Values, key string) (bool, error) {
	var param bool
	if _, ok := params[key]; ok {
		val := params.Get(key)
		// Orginally a comment declared this check should be removed after Consul
		// 0.10, to no longer support using ?passing without a value. However, I
		// think this is a reasonable experience for a user and so am keeping it
		// here.
		if val == "" {
			param = true
		} else {
			var err error
			param, err = strconv.ParseBool(val)
			if err != nil {
				return false, err
			}
		}
	}
	return param, nil
}
