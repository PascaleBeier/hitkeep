package cluster

import (
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"

	"hitkeep/internal/config"
	"hitkeep/internal/hklog"
)

type Manager struct {
	list   *memberlist.Memberlist
	logger *slog.Logger
	lock   sync.RWMutex
	self   string
	leader string
	peers  map[string]string
}

type eventDelegate struct {
	m *Manager
}

func (d *eventDelegate) NotifyJoin(node *memberlist.Node) {
	d.m.logger.Debug("Node joined cluster", "name", node.Name, "addr", node.Address())
	d.m.peers[node.Name] = node.Address()
	d.m.reElectLeader()
}

func (d *eventDelegate) NotifyLeave(node *memberlist.Node) {
	d.m.logger.Warn("Node left cluster", "name", node.Name)
	delete(d.m.peers, node.Name)
	d.m.reElectLeader()
}

func (d *eventDelegate) NotifyUpdate(node *memberlist.Node) {}

func NewManager(conf *config.Config, logger *slog.Logger) (*Manager, error) {
	if logger == nil {
		return nil, fmt.Errorf("cluster: logger is required")
	}
	m := &Manager{
		self:   conf.NodeName,
		logger: logger,
		peers:  make(map[string]string),
	}

	mlConfig := memberlist.DefaultWANConfig()
	mlConfig.Name = conf.NodeName
	mlConfig.Logger = hklog.MemberlistLogger(logger)
	bindAddr := conf.BindAddr
	bindPort := mlConfig.BindPort

	if host, port, err := net.SplitHostPort(conf.BindAddr); err == nil {
		if host != "" {
			bindAddr = host
		}
		if parsed, err := strconv.Atoi(port); err == nil && parsed > 0 {
			bindPort = parsed
		}
	} else if strings.Contains(conf.BindAddr, ":") {
		// Likely an IPv6 address without a port; keep as-is.
		bindAddr = conf.BindAddr
	}

	mlConfig.BindAddr = bindAddr
	mlConfig.BindPort = bindPort
	if bindAddr != "" && bindAddr != "0.0.0.0" {
		mlConfig.AdvertiseAddr = bindAddr
		mlConfig.AdvertisePort = bindPort
	}
	mlConfig.Events = &eventDelegate{m: m}

	list, err := memberlist.Create(mlConfig)
	if err != nil {
		return nil, err
	}
	m.list = list

	if conf.JoinAddr != "" {
		if _, err := list.Join([]string{conf.JoinAddr}); err != nil {
			return nil, err
		}
	}

	m.peers[conf.NodeName] = list.LocalNode().Address()
	m.reElectLeader()
	return m, nil
}

func (m *Manager) IsLeader() bool {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.leader == m.self
}

func (m *Manager) GetLeaderAddr() string {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.peers[m.leader]
}

func (m *Manager) HasPeers() bool {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.list.NumMembers() > 1
}

func (m *Manager) Shutdown() error {
	return m.list.Leave(1 * time.Second)
}

func (m *Manager) reElectLeader() {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Pre-allocate slice to avoid unnecessary allocations (prealloc)
	members := make([]string, 0, len(m.peers))
	for name := range m.peers {
		members = append(members, name)
	}
	sort.Strings(members)

	if len(members) > 0 && m.leader != members[0] {
		m.leader = members[0]
		m.logger.Debug("New leader elected", "leader", m.leader)
	}

	if m.leader == m.self {
		m.logger.Debug("This node is now the LEADER.")
	} else {
		m.logger.Debug("This node is a FOLLOWER.", "current_leader", m.leader)
	}
}
