package etcd

import (
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
	"fmt"
)

type EtcdModule struct{}

func (m *EtcdModule) Name() string { return "etcd" }
func (m *EtcdModule) Description() string { return "Manages Etcd cluster setup and configuration." }

func (m *EtcdModule) Init(pipelineRuntime runtime.Runtime, moduleSpec interface{}, logger *logrus.Entry) error {
	logger.Info("EtcdModule Init (placeholder)")

	_, ok := moduleSpec.(*config.EtcdSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.EtcdSpec, got %T", m.Name(), moduleSpec)
	}
	// Store spec if needed: m.spec = spec
	// Store runtime if needed: m.runtime = pipelineRuntime
	return nil
}

func (m *EtcdModule) Execute(logger *logrus.Entry) error {
	logger.Info("EtcdModule Execute (placeholder)")
	// Access stored spec and runtime here
	return nil
}
