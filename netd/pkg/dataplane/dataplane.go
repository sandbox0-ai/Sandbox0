// Package dataplane implements the network data plane for netd.
// It uses iptables/nftables for packet filtering and tc for traffic shaping.
// Optionally uses eBPF for more efficient bandwidth control.
package dataplane

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	"github.com/sandbox0-ai/infra/netd/pkg/ebpf"
	"github.com/sandbox0-ai/infra/netd/pkg/netdiscovery"
	"github.com/sandbox0-ai/infra/netd/pkg/watcher"
	"go.uber.org/zap"
)

// DataPlane manages network rules for sandboxes
type DataPlane struct {
	logger                  *zap.Logger
	proxyHTTPPort           int
	proxyHTTPSPort          int
	procdPort               int
	dnsPort                 int
	failClosed              bool
	clusterDNSCIDR          string
	vethPrefix              string
	burstRatio              float64
	iptablesBinary          string
	systemIngressIPProvider func() []string
	systemEgressIPProvider func() []string

	// eBPF manager for bandwidth control
	ebpfMgr    *ebpf.Manager
	useEBPF    bool
	bpfFSPath  string
	bpfPinPath string
	edtHorizon time.Duration

	// Track applied rules per sandbox
	mu           sync.RWMutex
	sandboxRules map[string]*SandboxRules
}

// SandboxRules tracks rules applied for a sandbox
type SandboxRules struct {
	SandboxID    string
	PodIP        string
	VethName     string
	EgressRules  []string
	IngressRules []string
	TCClass      string
	IPSets       []string
	Applied      bool
}

// Config contains configuration for the DataPlane
type Config struct {
	ProxyHTTPPort  int
	ProxyHTTPSPort int
	ProcdPort      int
	DNSPort        int
	FailClosed     bool
	ClusterDNSCIDR string
	UseEBPF        bool
	BPFFSPath      string
	BPFPinPath     string
	UseEDT         bool // Use Earliest Departure Time for eBPF pacing
	EDTHorizon     time.Duration
	VethPrefix     string
	BurstRatio     float64
	PreferNFT      bool
}

// NewDataPlane creates a new DataPlane
func NewDataPlane(
	logger *zap.Logger,
	proxyHTTPPort int,
	proxyHTTPSPort int,
	procdPort int,
	dnsPort int,
	failClosed bool,
	clusterDNSCIDR string,
	vethPrefix string,
	burstRatio float64,
) *DataPlane {
	return &DataPlane{
		logger:         logger,
		proxyHTTPPort:  proxyHTTPPort,
		proxyHTTPSPort: proxyHTTPSPort,
		procdPort:      procdPort,
		dnsPort:        dnsPort,
		failClosed:     failClosed,
		clusterDNSCIDR: clusterDNSCIDR,
		vethPrefix:     vethPrefix,
		burstRatio:     burstRatio,
		useEBPF:        false,
		sandboxRules:   make(map[string]*SandboxRules),
	}
}

// NewDataPlaneWithEBPF creates a new DataPlane with eBPF support
func NewDataPlaneWithEBPF(logger *zap.Logger, cfg *Config) (*DataPlane, error) {
	dp := &DataPlane{
		logger:         logger,
		proxyHTTPPort:  cfg.ProxyHTTPPort,
		proxyHTTPSPort: cfg.ProxyHTTPSPort,
		procdPort:      cfg.ProcdPort,
		dnsPort:        cfg.DNSPort,
		failClosed:     cfg.FailClosed,
		clusterDNSCIDR: cfg.ClusterDNSCIDR,
		vethPrefix:     cfg.VethPrefix,
		burstRatio:     cfg.BurstRatio,
		iptablesBinary: resolveIptablesBinary(cfg.PreferNFT, logger),
		useEBPF:        cfg.UseEBPF,
		bpfFSPath:      cfg.BPFFSPath,
		bpfPinPath:     cfg.BPFPinPath,
		edtHorizon:     cfg.EDTHorizon,
		sandboxRules:   make(map[string]*SandboxRules),
	}

	if cfg.UseEBPF {
		ebpfMgr, err := ebpf.NewManager(logger, cfg.BPFFSPath, cfg.BPFPinPath, cfg.UseEDT, cfg.EDTHorizon)
		if err != nil {
			logger.Warn("Failed to create eBPF manager, falling back to iptables/tc",
				zap.Error(err),
			)
			dp.useEBPF = false
		} else {
			dp.ebpfMgr = ebpfMgr
			dp.useEBPF = ebpfMgr.IsAvailable()
			if dp.useEBPF {
				logger.Info("eBPF support enabled for bandwidth control")
			} else {
				logger.Info("eBPF not available, using traditional tc for bandwidth control")
			}
		}
	}

	return dp, nil
}

// Initialize sets up base iptables chains for netd
func (dp *DataPlane) Initialize(ctx context.Context) error {
	dp.logger.Info("Initializing dataplane",
		zap.Bool("useEBPF", dp.useEBPF),
	)

	// Initialize eBPF manager if enabled
	if dp.ebpfMgr != nil {
		if err := dp.ebpfMgr.Initialize(ctx); err != nil {
			dp.logger.Warn("Failed to initialize eBPF manager", zap.Error(err))
			dp.useEBPF = false
		}
	}

	// Create custom chains for netd
	chains := []struct {
		table string
		chain string
	}{
		{"filter", "NETD-EGRESS"},
		{"filter", "NETD-INGRESS"},
		{"nat", "NETD-PREROUTING"},
		{"nat", "NETD-OUTPUT"},
		{"mangle", "NETD-EGRESS"},
	}

	for _, c := range chains {
		// Create chain if not exists
		if err := dp.runIPTables("-t", c.table, "-N", c.chain); err != nil {
			// Chain might already exist, ignore error
			dp.logger.Debug("Chain creation (may already exist)",
				zap.String("table", c.table),
				zap.String("chain", c.chain),
			)
		}

		// Flush chain
		if err := dp.runIPTables("-t", c.table, "-F", c.chain); err != nil {
			return fmt.Errorf("flush chain %s/%s: %w", c.table, c.chain, err)
		}
	}

	// Insert jumps to custom chains from built-in chains
	// These will be inserted at the beginning
	jumpRules := []struct {
		table     string
		chain     string
		target    string
		condition string
	}{
		{"filter", "FORWARD", "NETD-EGRESS", "-m comment --comment netd-egress"},
		{"filter", "FORWARD", "NETD-INGRESS", "-m comment --comment netd-ingress"},
		{"nat", "PREROUTING", "NETD-PREROUTING", "-m comment --comment netd-prerouting"},
		{"nat", "OUTPUT", "NETD-OUTPUT", "-m comment --comment netd-output"},
	}

	for _, r := range jumpRules {
		// Check if rule exists
		checkArgs := []string{"-t", r.table, "-C", r.chain, "-j", r.target}
		if r.condition != "" {
			checkArgs = append(checkArgs, strings.Fields(r.condition)...)
		}
		if err := dp.runIPTables(checkArgs...); err != nil {
			// Rule doesn't exist, insert it
			insertArgs := []string{"-t", r.table, "-I", r.chain, "1", "-j", r.target}
			if r.condition != "" {
				insertArgs = append(insertArgs, strings.Fields(r.condition)...)
			}
			if err := dp.runIPTables(insertArgs...); err != nil {
				return fmt.Errorf("insert jump rule: %w", err)
			}
		}
	}

	dp.logger.Info("Dataplane initialized")
	return nil
}

// ApplyPodRules applies network rules for a sandbox pod
func (dp *DataPlane) ApplyPodRules(
	ctx context.Context,
	info *watcher.SandboxInfo,
	networkPolicy *v1alpha1.NetworkPolicySpec,
	bandwidthPolicy *v1alpha1.BandwidthPolicySpec,
) error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if info.PodIP == "" {
		return fmt.Errorf("pod IP is empty for sandbox %s", info.SandboxID)
	}

	dp.logger.Info("Applying rules for sandbox",
		zap.String("sandboxID", info.SandboxID),
		zap.String("podIP", info.PodIP),
	)

	rules := &SandboxRules{
		SandboxID: info.SandboxID,
		PodIP:     info.PodIP,
	}

	// Apply egress rules
	if err := dp.applyEgressRules(ctx, info, networkPolicy, rules); err != nil {
		return fmt.Errorf("apply egress rules: %w", err)
	}

	// Apply ingress rules
	if err := dp.applyIngressRules(ctx, info, networkPolicy, rules); err != nil {
		return fmt.Errorf("apply ingress rules: %w", err)
	}

	// Apply bandwidth rules
	if bandwidthPolicy != nil {
		if err := dp.applyBandwidthRules(ctx, info, bandwidthPolicy, rules); err != nil {
			return fmt.Errorf("apply bandwidth rules: %w", err)
		}
	}

	rules.Applied = true
	dp.sandboxRules[info.SandboxID] = rules

	dp.logger.Info("Rules applied for sandbox",
		zap.String("sandboxID", info.SandboxID),
	)

	return nil
}

// applyEgressRules applies egress (outbound) rules for a sandbox
func (dp *DataPlane) applyEgressRules(
	ctx context.Context,
	info *watcher.SandboxInfo,
	policy *v1alpha1.NetworkPolicySpec,
	rules *SandboxRules,
) error {
	podIP := info.PodIP
	sandboxID := info.SandboxID
	comment := fmt.Sprintf("sandbox:%s", sandboxID)

	// 1. Allow established connections
	if err := dp.runIPTables(
		"-t", "filter", "-A", "NETD-EGRESS",
		"-s", podIP,
		"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED",
		"-m", "comment", "--comment", comment,
		"-j", "ACCEPT",
	); err != nil {
		return err
	}

	// 2. Always allow storage-proxy
	storageProxyCIDRs := dp.resolveProviderCIDRs(podIP, dp.systemEgressIPProvider)
	if len(storageProxyCIDRs) > 0 {
		setName := dp.ipsetName("eg-storage-allow", sandboxID)
		if dp.applyIPSet(ctx, setName, storageProxyCIDRs, rules) {
			if err := dp.runIPTables(
				"-t", "filter", "-A", "NETD-EGRESS",
				"-s", podIP,
				"-m", "set", "--match-set", setName, "dst",
				"-m", "comment", "--comment", comment+":storage-proxy",
				"-j", "ACCEPT",
			); err != nil {
				return err
			}
		} else {
			for _, cidr := range storageProxyCIDRs {
				if err := dp.runIPTables(
					"-t", "filter", "-A", "NETD-EGRESS",
					"-s", podIP, "-d", cidr,
					"-m", "comment", "--comment", comment+":storage-proxy",
					"-j", "ACCEPT",
				); err != nil {
					return err
				}
			}
		}
	}

	// 3. Allow DNS to cluster DNS
	if dp.clusterDNSCIDR != "" && dp.cidrMatchesPodIPFamily(podIP, dp.clusterDNSCIDR) {
		if err := dp.runIPTables(
			"-t", "filter", "-A", "NETD-EGRESS",
			"-s", podIP, "-d", dp.clusterDNSCIDR,
			"-p", "udp", "--dport", fmt.Sprintf("%d", dp.dnsPort),
			"-m", "comment", "--comment", comment+":dns",
			"-j", "ACCEPT",
		); err != nil {
			return err
		}
	}

	// 4. Redirect HTTP/HTTPS to proxy (for domain-based filtering)
	enforceProxyPorts := []int{80, 443}

	for _, port := range enforceProxyPorts {
		var proxyPort int
		if port == 80 {
			proxyPort = dp.proxyHTTPPort
		} else if port == 443 {
			proxyPort = dp.proxyHTTPSPort
		} else {
			proxyPort = dp.proxyHTTPSPort // Default to HTTPS proxy
		}

		if err := dp.runIPTables(
			"-t", "nat", "-A", "NETD-PREROUTING",
			"-s", podIP,
			"-p", "tcp", "--dport", fmt.Sprintf("%d", port),
			"-m", "comment", "--comment", comment+":proxy-redirect",
			"-j", "REDIRECT", "--to-ports", fmt.Sprintf("%d", proxyPort),
		); err != nil {
			return err
		}
	}

	// 5. Block platform-denied CIDRs (RFC1918, metadata, etc.)
	deniedCIDRs := dp.filterCIDRsForPodIP(podIP, v1alpha1.PlatformDeniedCIDRs)

	if len(deniedCIDRs) > 0 {
		setName := dp.ipsetName("eg-platform-deny", sandboxID)
		if dp.applyIPSet(ctx, setName, deniedCIDRs, rules) {
			if err := dp.runIPTables(
				"-t", "filter", "-A", "NETD-EGRESS",
				"-s", podIP,
				"-m", "set", "--match-set", setName, "dst",
				"-m", "comment", "--comment", comment+":deny-internal",
				"-j", "DROP",
			); err != nil {
				return err
			}
		} else {
			for _, cidr := range deniedCIDRs {
				if err := dp.runIPTables(
					"-t", "filter", "-A", "NETD-EGRESS",
					"-s", podIP, "-d", cidr,
					"-m", "comment", "--comment", comment+":deny-internal",
					"-j", "DROP",
				); err != nil {
					return err
				}
			}
		}
	}

	// 6. Apply policy rules (deny before allow)
	if policy != nil && policy.Egress != nil {
		deniedCIDRs := dp.filterCIDRsForPodIP(podIP, policy.Egress.DeniedCIDRs)
		allowedCIDRs := dp.filterCIDRsForPodIP(podIP, policy.Egress.AllowedCIDRs)
		allowedPorts := policy.Egress.AllowedPorts
		allowedCIDRsPresent := len(policy.Egress.AllowedCIDRs) > 0

		if len(deniedCIDRs) > 0 {
			setName := dp.ipsetName("eg-deny", sandboxID)
			if dp.applyIPSet(ctx, setName, deniedCIDRs, rules) {
				if err := dp.runIPTables(
					"-t", "filter", "-A", "NETD-EGRESS",
					"-s", podIP,
					"-m", "set", "--match-set", setName, "dst",
					"-m", "comment", "--comment", comment+":deny-cidr",
					"-j", "DROP",
				); err != nil {
					return err
				}
			} else {
				for _, cidr := range deniedCIDRs {
					if err := dp.runIPTables(
						"-t", "filter", "-A", "NETD-EGRESS",
						"-s", podIP, "-d", cidr,
						"-m", "comment", "--comment", comment+":deny-cidr",
						"-j", "DROP",
					); err != nil {
						return err
					}
				}
			}
		}

		for _, portSpec := range policy.Egress.DeniedPorts {
			if err := dp.applyEgressPortRule(podIP, portSpec, "DROP", comment+":deny-port"); err != nil {
				return err
			}
		}

		if len(allowedCIDRs) > 0 && len(allowedPorts) > 0 {
			setName := dp.ipsetName("eg-allow", sandboxID)
			if dp.applyIPSet(ctx, setName, allowedCIDRs, rules) {
				for _, portSpec := range allowedPorts {
					if err := dp.applyEgressPortRuleWithSet(podIP, portSpec, setName, "ACCEPT", comment+":allow-cidr-port"); err != nil {
						return err
					}
				}
			} else {
				for _, cidr := range allowedCIDRs {
					for _, portSpec := range allowedPorts {
						if err := dp.applyEgressPortRuleWithCIDR(podIP, cidr, portSpec, "ACCEPT", comment+":allow-cidr-port"); err != nil {
							return err
						}
					}
				}
			}
		} else if len(allowedCIDRs) > 0 {
			setName := dp.ipsetName("eg-allow", sandboxID)
			if dp.applyIPSet(ctx, setName, allowedCIDRs, rules) {
				if err := dp.runIPTables(
					"-t", "filter", "-A", "NETD-EGRESS",
					"-s", podIP,
					"-m", "set", "--match-set", setName, "dst",
					"-m", "comment", "--comment", comment+":allow-cidr",
					"-j", "ACCEPT",
				); err != nil {
					return err
				}
			} else {
				for _, cidr := range allowedCIDRs {
					if err := dp.runIPTables(
						"-t", "filter", "-A", "NETD-EGRESS",
						"-s", podIP, "-d", cidr,
						"-m", "comment", "--comment", comment+":allow-cidr",
						"-j", "ACCEPT",
					); err != nil {
						return err
					}
				}
			}
		} else if len(allowedPorts) > 0 && !allowedCIDRsPresent {
			for _, portSpec := range allowedPorts {
				if err := dp.applyEgressPortRule(podIP, portSpec, "ACCEPT", comment+":allow-port"); err != nil {
					return err
				}
			}
		}
	}

	// 7. Default action (deny by default for enterprise security)
	defaultAction := "DROP"
	if policy == nil || policy.Egress == nil {
		if policy != nil && policy.Mode == v1alpha1.NetworkModeAllowAll {
			defaultAction = "ACCEPT"
		} else if !dp.failClosed {
			defaultAction = "ACCEPT"
		}
	} else if policy.Mode == v1alpha1.NetworkModeAllowAll {
		defaultAction = "ACCEPT"
	}
	if policy != nil && policy.Egress != nil && len(policy.Egress.AllowedPorts) > 0 && defaultAction == "ACCEPT" {
		dp.logger.Warn("AllowedPorts set with default allow; enforcing default deny",
			zap.String("sandboxID", sandboxID),
		)
		defaultAction = "DROP"
	}

	if err := dp.runIPTables(
		"-t", "filter", "-A", "NETD-EGRESS",
		"-s", podIP,
		"-m", "comment", "--comment", comment+":default",
		"-j", defaultAction,
	); err != nil {
		return err
	}

	return nil
}

// applyIngressRules applies ingress (inbound) rules for a sandbox
func (dp *DataPlane) applyIngressRules(
	ctx context.Context,
	info *watcher.SandboxInfo,
	policy *v1alpha1.NetworkPolicySpec,
	rules *SandboxRules,
) error {
	podIP := info.PodIP
	sandboxID := info.SandboxID
	comment := fmt.Sprintf("sandbox:%s", sandboxID)

	// 1. Allow established connections
	if err := dp.runIPTables(
		"-t", "filter", "-A", "NETD-INGRESS",
		"-d", podIP,
		"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED",
		"-m", "comment", "--comment", comment,
		"-j", "ACCEPT",
	); err != nil {
		return err
	}

	// 2. Allow system services to reach sandbox (procd + control APIs)
	systemServiceCIDRs := dp.resolveProviderCIDRs(podIP, dp.systemIngressIPProvider)
	if len(systemServiceCIDRs) > 0 {
		setName := dp.ipsetName("in-system-allow", sandboxID)
		if dp.applyIPSet(ctx, setName, systemServiceCIDRs, rules) {
			if err := dp.runIPTables(
				"-t", "filter", "-A", "NETD-INGRESS",
				"-m", "set", "--match-set", setName, "src",
				"-d", podIP,
				"-m", "comment", "--comment", comment+":system-allow",
				"-j", "ACCEPT",
			); err != nil {
				return err
			}
		} else {
			for _, cidr := range systemServiceCIDRs {
				if err := dp.runIPTables(
					"-t", "filter", "-A", "NETD-INGRESS",
					"-s", cidr, "-d", podIP,
					"-m", "comment", "--comment", comment+":system-allow",
					"-j", "ACCEPT",
				); err != nil {
					return err
				}
			}
		}
	}

	// 3. Apply allowed source CIDRs from policy
	if policy != nil && policy.Ingress != nil {
		deniedCIDRs := dp.filterCIDRsForPodIP(podIP, policy.Ingress.DeniedCIDRs)
		allowedCIDRs := dp.filterCIDRsForPodIP(podIP, policy.Ingress.AllowedCIDRs)

		if len(deniedCIDRs) > 0 {
			setName := dp.ipsetName("in-deny", sandboxID)
			if dp.applyIPSet(ctx, setName, deniedCIDRs, rules) {
				if err := dp.runIPTables(
					"-t", "filter", "-A", "NETD-INGRESS",
					"-m", "set", "--match-set", setName, "src",
					"-d", podIP,
					"-m", "comment", "--comment", comment+":deny-source",
					"-j", "DROP",
				); err != nil {
					return err
				}
			} else {
				for _, cidr := range deniedCIDRs {
					if err := dp.runIPTables(
						"-t", "filter", "-A", "NETD-INGRESS",
						"-s", cidr, "-d", podIP,
						"-m", "comment", "--comment", comment+":deny-source",
						"-j", "DROP",
					); err != nil {
						return err
					}
				}
			}
		}

		if len(allowedCIDRs) > 0 {
			setName := dp.ipsetName("in-allow", sandboxID)
			if dp.applyIPSet(ctx, setName, allowedCIDRs, rules) {
				if err := dp.runIPTables(
					"-t", "filter", "-A", "NETD-INGRESS",
					"-m", "set", "--match-set", setName, "src",
					"-d", podIP,
					"-m", "comment", "--comment", comment+":allow-source",
					"-j", "ACCEPT",
				); err != nil {
					return err
				}
			} else {
				for _, cidr := range allowedCIDRs {
					if err := dp.runIPTables(
						"-t", "filter", "-A", "NETD-INGRESS",
						"-s", cidr, "-d", podIP,
						"-m", "comment", "--comment", comment+":allow-source",
						"-j", "ACCEPT",
					); err != nil {
						return err
					}
				}
			}
		}

	}

	// 4. Default deny for ingress
	defaultAction := "DROP"
	if policy == nil || policy.Ingress == nil {
		if !dp.failClosed {
			defaultAction = "ACCEPT"
		}
	}

	if err := dp.runIPTables(
		"-t", "filter", "-A", "NETD-INGRESS",
		"-d", podIP,
		"-m", "comment", "--comment", comment+":default",
		"-j", defaultAction,
	); err != nil {
		return err
	}

	return nil
}

// applyBandwidthRules applies tc bandwidth shaping rules
func (dp *DataPlane) applyBandwidthRules(
	ctx context.Context,
	info *watcher.SandboxInfo,
	policy *v1alpha1.BandwidthPolicySpec,
	rules *SandboxRules,
) error {
	// Discover the network interface for this pod using routing table.
	// This supports both standard containers (veth) and Kata Containers (tap).
	deviceInfo, err := netdiscovery.FindDeviceBySandboxID(info.SandboxID, info.PodIP)
	if err != nil {
		dp.logger.Warn("Failed to discover network device, skipping bandwidth control",
			zap.String("sandboxID", info.SandboxID),
			zap.String("podIP", info.PodIP),
			zap.Error(err),
		)
		// Don't fail - just log and skip bandwidth control
		// Security policies (iptables) will still work
		return nil
	}

	rules.VethName = deviceInfo.Name
	dp.logger.Info("Network device discovered for sandbox",
		zap.String("sandboxID", info.SandboxID),
		zap.String("podIP", info.PodIP),
		zap.String("deviceName", deviceInfo.Name),
		zap.String("deviceType", deviceInfo.Type),
	)

	// Get rate limits
	var egressRateBps, ingressRateBps, burstBytes int64
	if policy.EgressRateLimit != nil {
		egressRateBps = policy.EgressRateLimit.RateBps
		burstBytes = policy.EgressRateLimit.BurstBytes
	}
	if policy.IngressRateLimit != nil {
		ingressRateBps = policy.IngressRateLimit.RateBps
		if burstBytes == 0 {
			burstBytes = policy.IngressRateLimit.BurstBytes
		}
	}
	if burstBytes == 0 && egressRateBps > 0 {
		burstBytes = int64(float64(egressRateBps) * dp.burstRatio) // Default burst based on burstRatio
	}

	// Use eBPF manager if available for more efficient rate limiting
	if dp.ebpfMgr != nil && dp.useEBPF {
		cfg := &ebpf.RateLimitConfig{
			SandboxID:      info.SandboxID,
			Iface:          deviceInfo.Name,
			EgressRateBps:  egressRateBps,
			IngressRateBps: ingressRateBps,
			BurstBytes:     burstBytes,
			UseBPF:         true,
		}

		if err := dp.ebpfMgr.ApplyRateLimit(ctx, cfg); err != nil {
			dp.logger.Warn("eBPF rate limit failed, falling back to tc",
				zap.String("sandboxID", info.SandboxID),
				zap.Error(err),
			)
			return dp.applyTCBandwidthRules(ctx, info, deviceInfo.Name, egressRateBps, burstBytes, rules)
		}

		rules.TCClass = "ebpf:fq"
		return nil
	}

	// Fall back to traditional tc htb
	return dp.applyTCBandwidthRules(ctx, info, deviceInfo.Name, egressRateBps, burstBytes, rules)
}

// applyTCBandwidthRules applies bandwidth rules using traditional tc htb
func (dp *DataPlane) applyTCBandwidthRules(
	ctx context.Context,
	info *watcher.SandboxInfo,
	vethName string,
	rateBps int64,
	burstBytes int64,
	rules *SandboxRules,
) error {
	if rateBps == 0 {
		return nil
	}

	// Create qdisc if not exists
	dp.runTC("qdisc", "add", "dev", vethName, "root", "handle", "1:", "htb", "default", "10")

	// Create class for this sandbox
	classID := dp.tcClassID(info.SandboxID)
	if err := dp.runTC(
		"class", "add", "dev", vethName,
		"parent", "1:", "classid", classID,
		"htb", "rate", fmt.Sprintf("%dbit", rateBps),
		"burst", fmt.Sprintf("%d", burstBytes),
	); err != nil {
		if !dp.tcIgnoreExists(err) {
			dp.logger.Warn("Failed to add tc class", zap.Error(err))
		}
	}

	// Add filter to match this pod's traffic
	if err := dp.runTC(
		"filter", "add", "dev", vethName,
		"protocol", "ip", "parent", "1:0",
		"prio", "1", "u32",
		"match", "ip", "src", info.PodIP+"/32",
		"flowid", classID,
	); err != nil {
		if !dp.tcIgnoreExists(err) {
			dp.logger.Warn("Failed to add tc filter", zap.Error(err))
		}
	}

	rules.TCClass = classID
	return nil
}

// RemovePodRules removes all rules for a sandbox
func (dp *DataPlane) RemovePodRules(ctx context.Context, sandboxID string) error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	rules, ok := dp.sandboxRules[sandboxID]
	if !ok {
		return nil
	}

	dp.logger.Info("Removing rules for sandbox",
		zap.String("sandboxID", sandboxID),
	)

	comment := fmt.Sprintf("sandbox:%s", sandboxID)

	// Remove iptables rules by comment
	tables := []string{"filter", "nat", "mangle"}
	chains := []string{"NETD-EGRESS", "NETD-INGRESS", "NETD-PREROUTING", "NETD-OUTPUT"}

	for _, table := range tables {
		for _, chain := range chains {
			// List rules and find ones with our comment
			output, err := exec.CommandContext(ctx, "iptables", "-t", table, "-S", chain).Output()
			if err != nil {
				continue // Chain might not exist in this table
			}

			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, comment) {
					// Extract rule and delete it
					// Rule format: -A CHAIN ... -> we need -D CHAIN ...
					if strings.HasPrefix(line, "-A ") {
						deleteRule := strings.Replace(line, "-A ", "-D ", 1)
						args := strings.Fields(deleteRule)
						dp.runIPTables(append([]string{"-t", table}, args...)...)
					}
				}
			}
		}
	}

	// Remove tc/eBPF rules
	if rules.VethName != "" && rules.TCClass != "" {
		if dp.ebpfMgr != nil && strings.HasPrefix(rules.TCClass, "ebpf:") {
			// Remove eBPF-based rate limiting
			if err := dp.ebpfMgr.RemoveRateLimit(ctx, rules.VethName); err != nil {
				dp.logger.Warn("Failed to remove eBPF rate limit",
					zap.String("vethName", rules.VethName),
					zap.Error(err),
				)
			}
		} else {
			// Remove traditional tc rules
			dp.runTC("filter", "del", "dev", rules.VethName, "parent", "1:0")
			dp.runTC("class", "del", "dev", rules.VethName, "classid", rules.TCClass)
		}
	}

	// Remove ipsets
	for _, setName := range rules.IPSets {
		dp.destroyIPSet(ctx, setName)
	}

	delete(dp.sandboxRules, sandboxID)

	dp.logger.Info("Rules removed for sandbox",
		zap.String("sandboxID", sandboxID),
	)

	return nil
}

// Cleanup removes all netd rules
func (dp *DataPlane) Cleanup(ctx context.Context) error {
	dp.logger.Info("Cleaning up dataplane")

	// Flush all netd chains
	chains := []struct {
		table string
		chain string
	}{
		{"filter", "NETD-EGRESS"},
		{"filter", "NETD-INGRESS"},
		{"nat", "NETD-PREROUTING"},
		{"nat", "NETD-OUTPUT"},
		{"mangle", "NETD-EGRESS"},
	}

	for _, c := range chains {
		dp.runIPTables("-t", c.table, "-F", c.chain)
	}

	// Cleanup eBPF manager
	if dp.ebpfMgr != nil {
		if err := dp.ebpfMgr.Cleanup(ctx); err != nil {
			dp.logger.Warn("Failed to cleanup eBPF manager", zap.Error(err))
		}
	}

	// Clear sandbox rules map
	dp.mu.Lock()
	dp.sandboxRules = make(map[string]*SandboxRules)
	dp.mu.Unlock()

	dp.logger.Info("Dataplane cleaned up")
	return nil
}

// UseEBPF returns whether eBPF is enabled and available
func (dp *DataPlane) UseEBPF() bool {
	return dp.useEBPF && dp.ebpfMgr != nil
}

func (dp *DataPlane) SetSystemIngressIPProvider(provider func() []string) {
	dp.systemIngressIPProvider = provider
}

func (dp *DataPlane) SetStorageEgressIPProvider(provider func() []string) {
	dp.systemEgressIPProvider = provider
}

// runIPTables executes an iptables command
func (dp *DataPlane) runIPTables(args ...string) error {
	bin := dp.iptablesBinary
	if bin == "" {
		bin = "iptables"
	}
	cmd := exec.Command(bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		dp.logger.Debug("iptables command failed",
			zap.Strings("args", args),
			zap.String("output", string(output)),
			zap.Error(err),
		)
		return fmt.Errorf("iptables %v: %w (%s)", args, err, string(output))
	}
	return nil
}

// runTC executes a tc command
func (dp *DataPlane) runTC(args ...string) error {
	cmd := exec.Command("tc", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		dp.logger.Debug("tc command failed",
			zap.Strings("args", args),
			zap.String("output", string(output)),
			zap.Error(err),
		)
		return fmt.Errorf("tc %v: %w (%s)", args, err, string(output))
	}
	return nil
}

func (dp *DataPlane) filterCIDRsForPodIP(podIP string, cidrs []string) []string {
	if len(cidrs) == 0 {
		return nil
	}
	pod := net.ParseIP(podIP)
	if pod == nil {
		return cidrs
	}
	podIsV4 := pod.To4() != nil
	filtered := make([]string, 0, len(cidrs))
	for _, cidr := range cidrs {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			dp.logger.Warn("Invalid CIDR, skipping",
				zap.String("cidr", cidr),
				zap.Error(err),
			)
			continue
		}
		isV4 := ip.To4() != nil
		if isV4 == podIsV4 {
			filtered = append(filtered, cidr)
			continue
		}
		dp.logger.Debug("Skipping CIDR due to IP family mismatch",
			zap.String("cidr", cidr),
			zap.String("podIP", podIP),
		)
	}
	return filtered
}

func (dp *DataPlane) cidrMatchesPodIPFamily(podIP, cidr string) bool {
	filtered := dp.filterCIDRsForPodIP(podIP, []string{cidr})
	return len(filtered) == 1
}

func (dp *DataPlane) tcClassID(sandboxID string) string {
	if sandboxID == "" {
		return "1:1"
	}
	sum := sha256.Sum256([]byte(sandboxID))
	id := binary.BigEndian.Uint16(sum[:2])
	if id == 0 {
		id = 1
	}
	return fmt.Sprintf("1:%x", id)
}

func (dp *DataPlane) tcIgnoreExists(err error) bool {
	if err == nil {
		return true
	}
	// tc returns "File exists" when class/filter already present.
	return strings.Contains(err.Error(), "File exists")
}

func (dp *DataPlane) resolveProviderCIDRs(podIP string, provider func() []string) []string {
	if provider == nil {
		return nil
	}
	return dp.filterCIDRsForPodIP(podIP, dp.ipListToCIDRs(provider()))
}

func (dp *DataPlane) ipListToCIDRs(ips []string) []string {
	if len(ips) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ips))
	result := make([]string, 0, len(ips))
	for _, raw := range ips {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil {
			dp.logger.Warn("Invalid IP, skipping",
				zap.String("ip", raw),
			)
			continue
		}
		var cidr string
		if ip.To4() != nil {
			cidr = ip.String() + "/32"
		} else {
			cidr = ip.String() + "/128"
		}
		if _, ok := seen[cidr]; ok {
			continue
		}
		seen[cidr] = struct{}{}
		result = append(result, cidr)
	}
	return result
}

// IsPrivateIP checks if an IP is in private/reserved ranges
func IsPrivateIP(ip net.IP) bool {
	for _, cidr := range v1alpha1.PlatformDeniedCIDRs {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
