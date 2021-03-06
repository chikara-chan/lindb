package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"

	"go.uber.org/atomic"

	"github.com/lindb/lindb/constants"
	"github.com/lindb/lindb/coordinator/discovery"
	"github.com/lindb/lindb/models"
	"github.com/lindb/lindb/pkg/logger"
)

//go:generate mockgen -source=./node_state_machine.go -destination=./node_state_machine_mock.go -package=broker

// NodeStateMachine represents broker nodes state machine,
// listens node online/offline change event
type NodeStateMachine interface {
	discovery.Listener
	// GetCurrentNode returns the current broker node
	GetCurrentNode() models.Node
	// GetActiveNodes returns all active broker nodes
	GetActiveNodes() []models.ActiveNode
	// Close closes state machine, then releases resource
	Close() error

	// StartMonitoring starts monitoring broker nodes
	StartMonitoring()
	// StopMonitoring stops monitoring broker nodes
	StopMonitoring()
}

// nodeStateMachine implements node state machine interface,
// watches active node path.
type nodeStateMachine struct {
	currentNode models.Node
	discovery   discovery.Discovery

	monitoringSM MonitoringStateMachine
	monitoring   atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc

	mutex sync.RWMutex
	// brokers: broker node => replica list under this broker
	nodes map[string]models.ActiveNode

	log *logger.Logger
}

// NewNodeStateMachine creates a node state machine, and starts discovery for watching node state change event
func NewNodeStateMachine(ctx context.Context, currentNode models.Node,
	monitoringSM MonitoringStateMachine, discoveryFactory discovery.Factory,
) (NodeStateMachine, error) {
	c, cancel := context.WithCancel(ctx)
	stateMachine := &nodeStateMachine{
		ctx:          c,
		cancel:       cancel,
		monitoringSM: monitoringSM,
		currentNode:  currentNode,
		nodes:        make(map[string]models.ActiveNode),
		log:          logger.GetLogger("coordinator", "BrokerNodeStateMachine"),
	}
	// new replica status discovery
	stateMachine.discovery = discoveryFactory.CreateDiscovery(constants.ActiveNodesPath, stateMachine)
	if err := stateMachine.discovery.Discovery(); err != nil {
		return nil, fmt.Errorf("discovery broker node error:%s", err)
	}
	return stateMachine, nil
}

// GetCurrentNode returns the current broker node
func (s *nodeStateMachine) GetCurrentNode() models.Node {
	return s.currentNode
}

// GetActiveNodes returns all active broker nodes
func (s *nodeStateMachine) GetActiveNodes() []models.ActiveNode {
	var result []models.ActiveNode
	s.mutex.RLock()
	for _, node := range s.nodes {
		result = append(result, node)
	}
	s.mutex.RUnlock()
	return result
}

// OnCreate adds node into active node list when node online
func (s *nodeStateMachine) OnCreate(key string, resource []byte) {
	node := models.ActiveNode{}
	if err := json.Unmarshal(resource, &node); err != nil {
		s.log.Error("discovery node online but unmarshal error",
			logger.String("data", string(resource)), logger.Error(err))
		return
	}
	_, fileName := filepath.Split(key)
	nodeID := fileName
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.nodes[nodeID] = node

	if s.monitoring.Load() {
		s.monitoringSM.Start(fmt.Sprintf("http://%s:%d/metrics", node.Node.IP, node.Node.HTTPPort))
	}
}

// OnDelete removes node into active node list when node offline
func (s *nodeStateMachine) OnDelete(key string) {
	_, fileName := filepath.Split(key)
	nodeID := fileName
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.monitoring.Load() {
		node, ok := s.nodes[nodeID]
		if ok {
			s.monitoringSM.Stop(fmt.Sprintf("http://%s:%d/metrics", node.Node.IP, node.Node.HTTPPort))
		}
	}
	delete(s.nodes, nodeID)
}

// Close closes state machine, then releases resource
func (s *nodeStateMachine) Close() error {
	s.discovery.Close()
	s.mutex.Lock()
	s.nodes = make(map[string]models.ActiveNode)
	s.mutex.Unlock()
	s.cancel()
	return nil
}

// StartMonitoring starts monitoring broker nodes
func (s *nodeStateMachine) StartMonitoring() {
	if s.monitoring.CAS(false, true) {
		s.mutex.RLock()
		defer s.mutex.RUnlock()

		for _, node := range s.nodes {
			s.monitoringSM.Start(fmt.Sprintf("http://%s:%d/metrics", node.Node.IP, node.Node.HTTPPort))
		}
	}
}

// StopMonitoring stops monitoring broker nodes
func (s *nodeStateMachine) StopMonitoring() {
	if s.monitoring.CAS(true, false) {
		s.monitoringSM.StopAll()
	}
}
