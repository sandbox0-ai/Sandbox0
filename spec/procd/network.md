# Procd - 网络隔离设计规范

## 一、设计目标

在 **Procd层面** 实现动态网络隔离，无需与 K8s 交互，支持：
- **IP/CIDR 过滤**：精确控制出站流量目标 IP
- **域名过滤**：支持域名、通配符域名（*.example.com）
- **DNS 欺骗防护**：防火墙独立解析域名，不信任沙箱内 DNS
- **私有 IP 黑名单**：默认阻止访问内网 IP
- **动态配置**：运行时通过 API 修改网络策略
- **协议分离**：TCP 通过代理过滤，UDP/ICMP 直接过滤
- **暂停/恢复持久化**：策略在 pause/resume 后保持有效

---

## 二、架构设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│            Procd Network Architecture (Pod Level Isolation)                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Pod (由 CNI 配置，已有独立网络命名空间和 IP)                                │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                    Pod Network Namespace                               │  │
│   │                                                                       │  │
│   │   用户进程/Procd 的出站流量                                            │  │
│   │        │                                                              │  │
│   │        ▼                                                              │  │
│   │   ┌─────────────────────────────────────────────────────────────┐     │  │
│   │   │  nftables OUTPUT chain (Procd 配置)                          │     │  │
│   │   │  ┌───────────────────────────────────────────────────────┐  │     │  │
│   │   │  │  1. whitelist 模式:                                     │  │     │  │
│   │   │  │     - 允许白名单 IP (userAllowSet)                       │  │     │  │
│   │   │  │     - TCP → REDIRECT to TCPProxy (127.0.0.1:1080)      │  │     │  │
│   │   │  │     - 阻止其他                                           │  │     │  │
│   │   │  │                                                       │  │     │  │
│   │   │  │  2. allow-all 模式:                                    │  │     │  │
│   │   │  │     - 直接放行，不经过 TCPProxy                         │  │     │  │
│   │   │  └───────────────────────────────────────────────────────┘  │     │  │
│   │   └─────────────────────────────────────────────────────────────┘     │  │
│   │                                                                       │  │
│   │   ┌─────────────────────────┐         ┌─────────────────────────┐       │  │
│   │   │      Procd (PID=1)      │         │    用户进程 (子进程)     │       │  │
│   │   │  ┌───────────────────┐  │         │  - REPL/Shell           │       │  │
│   │   │  │  NetworkManager   │  │         │  - 所有出站流量        │       │  │
│   │   │  │  ┌─────────────┐  │  │         │    受规则控制          │       │  │
│   │   │  │  │ Firewall    │  │  │         │                         │       │  │
│   │   │  │  │ (nftables)  │  │  │         │                         │       │  │
│   │   │  │  │ TCPProxy    │  │  │         │                         │       │  │
│   │   │  │  │ (SOCKS5)    │  │  │         │                         │       │  │
│   │   │  │  │ DNSResolver │  │  │         │                         │       │  │
│   │   │  │  └─────────────┘  │  │         │                         │       │  │
│   │   │  │  HTTP API (8080)  │  │         │                         │       │  │
│   │   │  └───────────────────┘  │         │                         │       │  │
│   │   └─────────────────────────┘         └─────────────────────────┘       │  │
│   │                                                                       │  │
│   │                          ▼                                            │  │
│   │              veth (CNI 配置，到 Host 网桥)                             │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│   说明:                                                                      │
│   1. Procd 运行在 Pod 内 (PID=1)，使用 Pod 的网络命名空间                     │
│   2. nftables OUTPUT 链过滤 Pod 级别的出站流量                                │
│   3. 需要 NET_ADMIN capability (配置防火墙)                                  │
│   4. 推荐使用 Kata 运行时以提升安全性                                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、数据结构定义

### 3.1 网络策略结构

```go
// NetworkPolicy 网络策略 (运行时可修改)
type NetworkPolicy struct {
    // 策略模式
    Mode NetworkPolicyMode `json:"mode"`

    // 出站规则
    Egress *NetworkEgressPolicy `json:"egress,omitempty"`

    // 入站规则 (预留，目前一般不开放入站)
    Ingress *NetworkIngressPolicy `json:"ingress,omitempty"`

    // 最后更新时间
    UpdatedAt time.Time `json:"updated_at"`
}

type NetworkPolicyMode string

const (
    // NetworkModeAllowAll 允许所有出站流量 (不启用防火墙)
    NetworkModeAllowAll NetworkPolicyMode = "allow-all"

    // NetworkModeDenyAll 拒绝所有出站流量 (默认拒绝)
    NetworkModeDenyAll NetworkPolicyMode = "deny-all"

    // NetworkModeWhitelist 白名单模式 (仅允许明确指定的流量)
    NetworkModeWhitelist NetworkPolicyMode = "whitelist"
)

// NetworkEgressPolicy 出站策略
type NetworkEgressPolicy struct {
    // 允许的CIDR列表 (IP地址或CIDR块)
    AllowCIDRs []string `json:"allow_cidrs,omitempty"`

    // 允许的域名列表
    // 支持: "example.com", "*.example.com", "*"
    AllowDomains []string `json:"allow_domains,omitempty"`

    // 拒绝的CIDR列表 (优先级高于Allow)
    DenyCIDRs []string `json:"deny_cidrs,omitempty"`

    // TCP代理端口 (0表示不使用代理)
    // 非0时TCP流量会被重定向到此端口的SOCKS5代理
    TCPProxyPort int32 `json:"tcp_proxy_port,omitempty"`
}

// NetworkIngressPolicy 入站策略 (预留)
type NetworkIngressPolicy struct {
    // 允许的端口列表
    AllowPorts []int32 `json:"allow_ports,omitempty"`

    // 允许的源CIDR列表
    AllowSources []string `json:"allow_sources,omitempty"`
}
```

### 3.2 网络配置

```go
// NetworkConfig 网络配置
type NetworkConfig struct {
    // Sandbox ID
    SandboxID string

    // TCP代理配置
    TCPProxyPort    int32   // 默认 1080
    EnableTCPProxy  bool

    // DNS配置
    DNSServers      []string // ["8.8.8.8", "8.8.4.4"]

    // 预定义黑名单 (默认阻止的IP范围)
    DefaultDenyCIDRs []string
}
```

---

## 四、NetworkManager 实现

### 4.1 核心接口

```go
// NetworkManager 网络管理器 (Procd核心组件)
type NetworkManager struct {
    mu               sync.RWMutex

    // 防火墙
    firewall         *Firewall
    nftConn          *nftables.Conn

    // TCP代理
    tcpProxy         *TCPProxy
    tcpProxyPort     int32

    // DNS解析器 (独立解析，防止DNS欺骗)
    dnsResolver      *DNSResolver

    // 当前策略
    currentPolicy    *NetworkPolicy

    // 配置
    config           *NetworkConfig
}

// NewNetworkManager 创建网络管理器
func NewNetworkManager(config *NetworkConfig) (*NetworkManager, error) {
    nm := &NetworkManager{
        config:       config,
        tcpProxyPort: config.TCPProxyPort,
    }

    // 1. 初始化防火墙 (在当前 Pod 网络命名空间)
    firewall, err := NewFirewall(config.DefaultDenyCIDRs)
    if err != nil {
        return nil, fmt.Errorf("create firewall: %w", err)
    }
    nm.firewall = firewall

    // 2. 初始化DNS解析器
    nm.dnsResolver = NewDNSResolver(config.DNSServers)

    // 3. 设置默认策略 (allow-all)
    nm.currentPolicy = &NetworkPolicy{
        Mode:     NetworkModeAllowAll,
        Egress:   &NetworkEgressPolicy{},
        UpdatedAt: time.Now(),
    }

    return nm, nil
}

// SetupNetwork 配置网络 (Procd启动时调用一次)
func (nm *NetworkManager) SetupNetwork() error {
    nm.mu.Lock()
    defer nm.mu.Unlock()

    // 在当前 Pod 网络命名空间初始化 nftables 规则
    if err := nm.firewall.Initialize(); err != nil {
        return err
    }

    return nil
}

// UpdatePolicy 更新网络策略 (动态调用)
func (nm *NetworkManager) UpdatePolicy(policy *NetworkPolicy) error {
    nm.mu.Lock()
    defer nm.mu.Unlock()

    // 1. 更新 Firewall 规则
    if err := nm.firewall.UpdatePolicy(policy); err != nil {
        return err
    }

    // 2. 更新 TCPProxy 白名单
    if nm.tcpProxy != nil && policy.Egress != nil {
        // 更新域名白名单
        nm.tcpProxy.allowDomains = NewDomainMatcher(policy.Egress.AllowDomains)

        // 更新 IP 白名单 (构建 ipnet.IPNetSet)
        allowIPs := ipnet.NewIPNetSet()
        for _, cidr := range policy.Egress.AllowCIDRs {
            _, network, _ := net.ParseCIDR(cidr)
            allowIPs.AddIPNet(network)
        }
        nm.tcpProxy.allowIPs = allowIPs
    }

    // 3. 保存当前策略
    nm.currentPolicy = policy
    nm.currentPolicy.UpdatedAt = time.Now()

    return nil
}
```

### 4.2 Firewall 实现

```go
// Firewall 防火墙 (基于nftables)
type Firewall struct {
    conn              *nftables.Conn
    table             *nftables.Table
    outputChain       *nftables.Chain

    // IP集合 (使用nftables sets)
    predefinedAllowSet set.Set  // 预定义允许集合 (通常为空)
    predefinedDenySet  set.Set  // 预定义拒绝集合 (私有IP)

    userAllowSet       set.Set  // 用户定义允许集合
    userDenySet        set.Set  // 用户定义拒绝集合

    // TCP重定向规则 (用于域名过滤)
    tcpRedirectRule    *nftables.Rule

    allowedMark        uint32  // 0x1
}

// NewFirewall 创建防火墙
func NewFirewall(defaultDenyCIDRs []string) (*Firewall, error) {
    conn, err := nftables.New(nftables.AsLasting)
    if err != nil {
        return nil, err
    }

    table := &nftables.Table{
        Name:   "sb0-firewall",
        Family: nftables.TableFamilyINet,
    }
    conn.AddTable(table)

    // 创建 OUTPUT filter 链 (用于过滤出站流量)
    outputChain := &nftables.Chain{
        Name:     "SANDBOX0_OUTPUT",
        Table:    table,
        Type:     nftables.ChainTypeFilter,
        Hooknum:  nftables.ChainHookOutput,     // OUTPUT hook 用于出站流量
        Priority: nftables.ChainPriorityFilter, // 标准优先级
        Policy:   nftables.ChainPolicyAccept,   // 默认放行
    }
    conn.AddChain(outputChain)

    // 创建IP集合
    predefinedAllowSet, _ := set.New(conn, table, "predef_allow", nftables.TypeIPAddr)
    predefinedDenySet, _ := set.New(conn, table, "predef_deny", nftables.TypeIPAddr)
    userAllowSet, _ := set.New(conn, table, "user_allow", nftables.TypeIPAddr)
    userDenySet, _ := set.New(conn, table, "user_deny", nftables.TypeIPAddr)

    fw := &Firewall{
        conn:               conn,
        table:              table,
        outputChain:        outputChain,
        predefinedAllowSet: predefinedAllowSet,
        predefinedDenySet:  predefinedDenySet,
        userAllowSet:       userAllowSet,
        userDenySet:        userDenySet,
        allowedMark:        0x1,
    }

    // 初始化预定义黑名单
    if err := fw.predefinedDenySet.ClearAndAddElements(conn, defaultDenyCIDRs); err != nil {
        return nil, err
    }

    // 安装基础规则
    if err := fw.installBaseRules(); err != nil {
        return nil, err
    }

    return fw, nil
}

// installBaseRules 安装基础规则
func (fw *Firewall) installBaseRules() error {
    // Rule order is critical:
    // 1. Predefined blacklist (private IPs) → drop
    // 2. User deny list → drop
    // 3. All TCP (except proxy port) → redirect to TCPProxy

    // 1. 预定义黑名单规则 (私有IP)
    fw.conn.AddRule(&nftables.Rule{
        Table: fw.table,
        Chain: fw.outputChain,
        Expr: []nftables.Expr{
            // match ip daddr @predef_deny
            &nftables.Payload{
                Operation: nftables.PayloadOperationLoad,
                SourceRegister: true,
                DestRegister:   1,
            },
            &nftables.Lookup{
                SetName: "predef_deny",
                SetID:   fw.predefinedDenySet.GetID(),
            },
            // verdict drop
            &nftables.Verdict{
                Kind: nftables.VerdictDrop,
            },
        },
    })

    // 2. 用户拒绝集合规则 (优先级最高)
    fw.conn.AddRule(&nftables.Rule{
        Table: fw.table,
        Chain: fw.outputChain,
        Expr: []nftables.Expr{
            // match ip daddr @user_deny
            &nftables.Payload{
                Operation: nftables.PayloadOperationLoad,
                SourceRegister: true,
                DestRegister:   1,
            },
            &nftables.Lookup{
                SetName: "user_deny",
                SetID:   fw.userDenySet.GetID(),
            },
            // verdict drop
            &nftables.Verdict{
                Kind: nftables.VerdictDrop,
            },
        },
    })

    return fw.conn.Flush()
}

// UpdatePolicy 更新网络策略 (动态调用)
func (fw *Firewall) UpdatePolicy(policy *NetworkPolicy) error {
    // 1. 清空用户集合
    if err := fw.userAllowSet.Clear(fw.conn); err != nil {
        return err
    }
    if err := fw.userDenySet.Clear(fw.conn); err != nil {
        return err
    }

    // 2. 删除旧的 TCP 重定向规则
    if fw.tcpRedirectRule != nil {
        fw.conn.DelRule(fw.tcpRedirectRule)
        fw.tcpRedirectRule = nil
    }

    // 3. 根据模式配置规则
    switch policy.Mode {
    case NetworkModeAllowAll:
        // allow-all 模式: 不需要额外规则，直接放行

    case NetworkModeDenyAll:
        // deny-all 模式: 添加拒绝所有规则
        fw.conn.AddRule(&nftables.Rule{
            Table: fw.table,
            Chain: fw.outputChain,
            Expr: []nftables.Expr{
                &nftables.Verdict{Kind: nftables.VerdictDrop},
            },
        })

    case NetworkModeWhitelist:
        // whitelist 模式: 所有 TCP 流量重定向到 TCPProxy
        // TCPProxy 内部检查 IP 白名单和域名白名单
        if policy.Egress != nil && policy.Egress.TCPProxyPort > 0 {
            // Store allowed CIDRs in userAllowSet for TCPProxy to check
            for _, cidr := range policy.Egress.AllowCIDRs {
                if err := fw.userAllowSet.AddElement(fw.conn, cidr); err != nil {
                    return err
                }
            }

            // Store allowed domains in TCPProxy's DomainMatcher
            // Add TCP redirect rule: all TCP → TCPProxy
            fw.addTCPRedirectRule(policy.Egress.TCPProxyPort)
        }
    }

    // 4. 配置拒绝集合 (优先级高于允许，在 installBaseRules 中已添加规则)
    if policy.Egress != nil {
        for _, cidr := range policy.Egress.DenyCIDRs {
            if err := fw.userDenySet.AddElement(fw.conn, cidr); err != nil {
                return err
            }
        }
    }

    return fw.conn.Flush()
}

// addTCPRedirectRule 添加 TCP 重定向到代理的规则
func (fw *Firewall) addTCPRedirectRule(proxyPort int32) {
    fw.tcpRedirectRule = &nftables.Rule{
        Table: fw.table,
        Chain: fw.outputChain,
        Expr: []nftables.Expr{
            // meta l4proto tcp
            &nftables.Meta{Key: "l4proto", Register: 1},
            &nftables.Cmp{Op: nftables.CmpOpEq, Register: 1, Data: []byte{6}}, // IPPROTO_TCP

            // tcp dport != {proxyPort}
            &nftables.Payload{
                SourceRegister: 1,
                DestRegister:   2,
                Operation:      nftables.PayloadOperationLoad,
                Field:          "tcp dport",
            },
            &nftables.Cmp{
                Op:       nftables.CmpOpNeq,
                Register: 2,
                Data:     []byte{byte(proxyPort >> 8), byte(proxyPort)},
            },

            // redirect to 127.0.0.1:{proxyPort}
            &nftables.Redirect{
                Register: 1,
                Address:  "127.0.0.1",
                Port:     proxyPort,
            },
        },
    }
    fw.conn.AddRule(fw.tcpRedirectRule)
}
```

### 4.3 TCPProxy 实现（域名过滤）

```go
// TCPProxy TCP代理 (SOCKS5协议)
type TCPProxy struct {
    listenAddr   string
    dnsResolver  *DNSResolver
    allowDomains *DomainMatcher  // 允许的域名列表
    allowIPs     *ipnet.IPNetSet // 允许的IP/CIDR集合

    // 连接跟踪
    connections  sync.Map
}

// DomainMatcher 域名匹配器
type DomainMatcher struct {
    mu       sync.RWMutex
    exact    map[string]bool      // 精确匹配: "example.com"
    wildcard []*glob.Glob         // 通配符: "*.example.com"
    allowAll bool                 // "*"
}

// NewDomainMatcher 创建域名匹配器
func NewDomainMatcher(domains []string) *DomainMatcher {
    dm := &DomainMatcher{
        exact: make(map[string]bool),
    }

    for _, d := range domains {
        d = strings.ToLower(strings.TrimSpace(d))

        if d == "*" {
            dm.allowAll = true
            continue
        }

        if strings.HasPrefix(d, "*.") {
            // 通配符域名
            g, _ := glob.Compile(d)
            dm.wildcard = append(dm.wildcard, g)
        } else {
            // 精确域名
            dm.exact[d] = true
        }
    }

    return dm
}

// TCP代理处理流程
func (p *TCPProxy) handleConnection(clientConn net.Conn) {
    // 1. 解析SOCKS5握手，获取目标地址
    targetAddr, err := p.parseSocks5Handshake(clientConn)
    if err != nil {
        clientConn.Close()
        return
    }

    // 2. 分离host和port
    host, port, _ := net.SplitHostPort(targetAddr)

    // 3. DNS欺骗防护：代理自己解析域名
    var targetIPs []string
    if ip := net.ParseIP(host); ip != nil {
        // 直接IP连接
        targetIPs = []string{host}
    } else {
        // 域名连接：代理自己DNS解析
        resolvedIPs, err := p.dnsResolver.Resolve(host)
        if err != nil {
            clientConn.Close()
            return
        }

        // 4. 域名白名单检查
        if !p.allowDomains.Match(host) {
            // 域名不在白名单，拒绝连接
            clientConn.Close()
            return
        }

        targetIPs = resolvedIPs
    }

    // 5. IP白名单检查 (检查 userAllowSet)
    for _, ip := range targetIPs {
        if p.isIPAllowed(ip) {
            // 6. 建立到目标服务器的连接
            targetConn, err := net.Dial("tcp", net.JoinHostPort(ip, port))
            if err != nil {
                continue
            }

            // 7. 双向转发数据
            go p.relay(clientConn, targetConn)
            return
        }
    }

    // 所有IP都被拒绝
    clientConn.Close()
}

// isIPAllowed 检查IP是否被允许
func (p *TCPProxy) isIPAllowed(ipStr string) bool {
    ip := net.ParseIP(ipStr)

    // 1. 检查是否是私有IP (默认拒绝)
    if isPrivateIP(ip) {
        return false
    }

    // 2. 检查用户白名单 IP 集合
    if p.allowIPs != nil {
        return p.allowIPs.Contains(ip)
    }

    // 3. 如果没有配置白名单，拒绝所有 (whitelist 模式)
    return false
}

// isPrivateIP 检查是否是私有IP
func isPrivateIP(ip net.IP) bool {
    privateRanges := []string{
        "10.0.0.0/8",
        "127.0.0.0/8",
        "169.254.0.0/16",
        "172.16.0.0/12",
        "192.168.0.0/16",
    }

    for _, cidr := range privateRanges {
        _, network, _ := net.ParseCIDR(cidr)
        if network.Contains(ip) {
            return true
        }
    }

    return false
}
```

### 4.4 DNSResolver 实现

```go
// DNSResolver 独立DNS解析器 (防止沙箱内DNS欺骗)
type DNSResolver struct {
    servers    []string
    cache      sync.Map  // 域名 -> []IP (带过期)
    cacheTTL   time.Duration
}

// NewDNSResolver 创建DNS解析器
func NewDNSResolver(servers []string) *DNSResolver {
    return &DNSResolver{
        servers:  servers,
        cacheTTL: 5 * time.Minute,
    }
}

// Resolve 解析域名 (独立于沙箱内DNS配置)
func (r *DNSResolver) Resolve(domain string) ([]string, error) {
    domain = strings.ToLower(domain)

    // 检查缓存
    if cached, ok := r.cache.Load(domain); ok {
        entry := cached.(*cacheEntry)
        if time.Since(entry.expiredAt) < r.cacheTTL {
            return entry.ips, nil
        }
        r.cache.Delete(domain)
    }

    // 执行DNS查询 (使用外部DNS服务器)
    var ips []string
    var lastErr error

    for _, server := range r.servers {
        resolver := &net.Resolver{
            PreferGo: true,
            Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
                d := net.Dialer{
                    Timeout: time.Second * 5,
                }
                return d.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
            },
        }

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        result, err := resolver.LookupIPAddr(ctx, domain)
        cancel()

        if err == nil {
            for _, ipAddr := range result {
                ips = append(ips, ipAddr.IP.String())
            }
            break
        }
        lastErr = err
    }

    if len(ips) == 0 {
        return nil, lastErr
    }

    // 更新缓存
    r.cache.Store(domain, &cacheEntry{
        ips:       ips,
        expiredAt: time.Now().Add(r.cacheTTL),
    })

    return ips, nil
}

type cacheEntry struct {
    ips       []string
    expiredAt time.Time
}
```

---

## 五、HTTP API

### 5.1 获取当前网络策略

```http
GET /api/v1/network/policy

Response: 200 OK
{
    "mode": "whitelist",
    "egress": {
        "allow_cidrs": ["8.8.8.8", "1.1.1.0/24"],
        "allow_domains": ["google.com", "*.github.com"],
        "deny_cidrs": ["10.0.0.0/8"],
        "tcp_proxy_port": 1080
    },
    "updated_at": "2024-01-01T00:00:00Z"
}
```

### 5.2 更新网络策略

```http
PUT /api/v1/network/policy
Content-Type: application/json

{
    "mode": "whitelist",
    "egress": {
        "allow_cidrs": ["8.8.8.8"],
        "allow_domains": ["google.com", "*.github.com"]
    }
}

Response: 200 OK
{
    "mode": "whitelist",
    "egress": { ... },
    "updated_at": "2024-01-01T00:01:00Z"
}
```

### 5.3 重置为默认策略

```http
POST /api/v1/network/policy/reset

Response: 200 OK
{
    "mode": "allow-all",
    "egress": null,
    "updated_at": "2024-01-01T00:02:00Z"
}
```

### 5.4 添加/删除规则

```http
# 添加允许的CIDR
POST /api/v1/network/policy/allow/cidr
{
    "cidr": "8.8.8.8"
}

# 添加允许的域名
POST /api/v1/network/policy/allow/domain
{
    "domain": "google.com"
}

# 添加拒绝的CIDR
POST /api/v1/network/policy/deny/cidr
{
    "cidr": "10.0.0.0/8"
}
```

---

## 六、与 Manager 的交互

### 6.1 沙箱认领时更新策略

```go
// Manager认领沙箱时调用Procd API
func (s *SandboxService) claimIdlePod(ctx context.Context, template *crd.SandboxTemplate, pod *corev1.Pod, req *ClaimSandboxRequest) (*ClaimSandboxResponse, error) {
    // ... 更新Pod labels ...

    // 获取Procd地址
    procdAddr := s.buildProcdAddress(pod)

    // 如果有网络策略配置，调用Procd API更新
    if req.Config != nil && req.Config.Network != nil {
        if err := s.updateNetworkPolicy(ctx, procdAddr, req.Config.Network); err != nil {
            // 回滚...
            return nil, err
        }
    }

    return &ClaimSandboxResponse{...}, nil
}

func (s *SandboxService) updateNetworkPolicy(ctx context.Context, procdAddr string, network *NetworkOverride) error {
    // 调用Procd API更新网络策略
    url := fmt.Sprintf("http://%s/api/v1/network/policy", procdAddr)

    policy := &NetworkPolicy{
        Mode: NetworkModeWhitelist,
        Egress: &NetworkEgressPolicy{
            AllowCIDRs:   network.AllowedCIDRs,
            AllowDomains: network.AllowedDomains,
        },
    }

    resp, err := http.Put(url, policy)
    return err
}
```

---

## 七、安全与性能

### 7.1 特权容器要求

由于需要配置 nftables，Procd 容器需要以下权限：

```yaml
securityContext:
  privileged: false
  capabilities:
    add:
      - NET_ADMIN    # Configure nftables firewall rules

# Note: SYS_ADMIN capability is only required for volume management (OverlayFS mounting),
# not for network isolation. See volume.md for details.
```

### 7.2 推荐使用 Kata 运行时

为了降低特权容器的安全风险，强烈推荐使用 Kata Containers：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: sandbox-pod
spec:
  runtimeClassName: kata  # 使用 Kata 运行时
  containers:
  - name: procd
    securityContext:
      privileged: true     # 在 VM 内安全
```

**Kata 的优势**：
- 特权容器限制在轻量级 VM 内
- VM 有独立的内核和网络栈
- 容器逃逸只影响 VM，不影响主机
- 网络策略更新速度相同 (~10ms)

### 7.3 冷启动性能对比

| 方案 | 冷启动延迟 | 安全性 |
|------|-----------|--------|
| K8s NetworkPolicy | ~100-200ms | 高 |
| Procd + 特权容器 | ~10-20ms | 低 |
| **Procd + Kata** | **~10-20ms** | **高** |

---

## 八、与 E2B 功能对比

| 功能 | E2B | Sandbox0 (Procd层面) |
|------|-----|---------------------|
| **IP/CIDR过滤** | ✅ nftables | ✅ nftables |
| **域名过滤** | ✅ TCP代理 | ✅ TCP代理 |
| **通配符域名** | ✅ *.domain.com | ✅ *.domain.com |
| **DNS欺骗防护** | ✅ 独立DNS解析 | ✅ 独立DNS解析 |
| **私有IP黑名单** | ✅ 预定义拒绝集合 | ✅ 预定义拒绝集合 |
| **动态配置** | ✅ 运行时修改 | ✅ HTTP API |
| **暂停/恢复持久化** | ✅ | ✅ (策略保存在Procd内存) |
| **协议分离** | ✅ TCP代理/其他直接 | ✅ TCP代理/其他直接 |
| **冷启动延迟** | ⚠️ 需要分配网络slot | ✅ Pod预启动，策略动态更新 |

---

## 九、优势总结

1. **零冷启动延迟**：网络策略在Procd层面配置，Pod认领时无需与K8s交互
2. **动态配置**：通过HTTP API随时修改网络策略
3. **完整功能**：实现E2B所有网络隔离功能
4. **简单部署**：无需复杂的网络slot池管理
5. **K8s原生**：与ReplicaSet + Idle Pool完美配合
6. **Kata友好**：配合Kata运行时实现高安全性
