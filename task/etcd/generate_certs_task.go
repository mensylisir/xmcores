package etcd

import (
	"fmt"

	"github.com/mensylisir/xmcores/config"
	// "github.com/mensylisir/xmcores/pipeline/ending" // Not directly used by this task's methods
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step/runcmd"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// GenerateCertsTask is responsible for generating etcd certificates.
type GenerateCertsTask struct {
	*task.BaseTask // Embed pointer to BaseTask
	// TaskSpec is inherited from BaseTask and will be asserted to *config.EtcdSpec in Default/Init
}

// NewGenerateCertsTask creates a new GenerateCertsTask.
func NewGenerateCertsTask() task.Task {
	base := task.NewBaseTask("etcd-generate-certs", "Generates etcd CA and node certificates.")
	return &GenerateCertsTask{
		BaseTask: base,
	}
}

// Default stores runtime, logger, and the taskSpec.
func (t *GenerateCertsTask) Default(runtime *krt.ClusterRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error {
	if err := t.BaseTask.Default(runtime, taskSpec, moduleCache, taskCache); err != nil {
		return err
	}
	t.Logger = runtime.Log.WithFields(logrus.Fields{"task": t.NameField, "type": "GenerateCertsTask"})

	if _, ok := t.TaskSpec.(*config.EtcdSpec); !ok {
		return fmt.Errorf("invalid taskSpec type for %s: expected *config.EtcdSpec, got %T", t.NameField, t.TaskSpec)
	}

	t.Logger.Info("GenerateCertsTask Default completed.")
	return nil
}


// Init initializes the GenerateCertsTask by creating necessary steps.
func (t *GenerateCertsTask) Init() error {
	if err := t.BaseTask.Init(); err != nil {
		return err
	}
	t.Logger.Info("GenerateCertsTask Init called - assembling steps.")

	if t.TaskSpec == nil {
        return fmt.Errorf("taskSpec not set for %s (must be set in Default method)", t.NameField)
    }
	etcdSpec, _ := t.TaskSpec.(*config.EtcdSpec) // Already validated in Default

	if etcdSpec.Type == "external" {
		t.Logger.Info("Etcd type is external; certificate generation is assumed to be handled by the user. No steps added.")
		return nil
	}

	t.Logger.Infof("Initializing certificate generation steps for etcd type: %s", etcdSpec.Type)

	caCmdStep := runcmd.NewRunCommandStep("GenerateEtcdCA", "Generates ETCD CA", "echo TODO: Generate ETCD CA certificate and key")
	stepLoggerCA := t.Logger.WithField("step", caCmdStep.Name()) // Use task's logger
	if err := caCmdStep.Init(t.Runtime, stepLoggerCA); err != nil {
		return fmt.Errorf("failed to init step '%s': %w", caCmdStep.Name(), err)
	}
	t.AddStep(caCmdStep)
	t.Logger.Debugf("Added step: %s", caCmdStep.Name())

	etcdHosts := t.Runtime.RoleHosts()["etcd"]
	if len(etcdHosts) == 0 && (etcdSpec.Type == "kubeadm" || etcdSpec.Type == "kubexm") {
		// For kubeadm, etcd nodes are control-plane nodes.
		// For kubexm, etcd nodes are from 'etcd' role.
		// This check might need adjustment based on how roles are assigned for stacked kubeadm.
		// If etcdSpec.Type == "kubeadm", it might use control-plane hosts.
		roleToUse := connector.RoleEtcd
		if etcdSpec.Type == "kubeadm" { // Kubeadm often co-locates etcd with control plane
			etcdHosts = t.Runtime.RoleHosts()[connector.RoleControlPlane]
			roleToUse = connector.RoleControlPlane
		}
		if len(etcdHosts) == 0 {
			return fmt.Errorf("no hosts found in role '%s' for etcd certificate generation (etcd type: %s)", roleToUse, etcdSpec.Type)
		}
	}


	for _, host := range etcdHosts {
		nodeCertCmd := fmt.Sprintf("echo TODO: Generate ETCD certificate for node %s using CA", host.GetName())
		nodeCertStep := runcmd.NewRunCommandStep(
			fmt.Sprintf("GenerateEtcdCert-%s", host.GetName()),
			fmt.Sprintf("Generates ETCD certificate for node %s", host.GetName()),
			nodeCertCmd,
		)
		stepLoggerNode := t.Logger.WithField("step", nodeCertStep.Name())
		if err := nodeCertStep.Init(t.Runtime, stepLoggerNode); err != nil {
			return fmt.Errorf("failed to init step '%s': %w", nodeCertStep.Name(), err)
		}
		t.AddStep(nodeCertStep)
		t.Logger.Debugf("Added step: %s", nodeCertStep.Name())
	}

	t.Logger.Info("GenerateCertsTask initialized with certificate generation steps.")
	return nil
}

// Slogan provides a specific slogan for GenerateCertsTask.
func (t *GenerateCertsTask) Slogan() string {
	spec, ok := t.TaskSpec.(*config.EtcdSpec)
	if !ok || spec == nil { // Should not happen if Default ran correctly
		return fmt.Sprintf("Generating ETCD certificates for task: %s...", t.NameField)
	}
	return fmt.Sprintf("Generating ETCD certificates (type: %s) for task: %s...", spec.Type, t.NameField)
}

var _ task.Task = (*GenerateCertsTask)(nil)
