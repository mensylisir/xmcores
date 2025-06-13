package etcd

import (
	"fmt"

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/pipeline/ending" // Though Run is inherited, good to keep for context
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step/runcmd"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// GenerateCertsTask is responsible for generating etcd certificates.
type GenerateCertsTask struct {
	task.BaseTask
	// TaskSpec is inherited from BaseTask, will be asserted to *config.EtcdSpec
}

// NewGenerateCertsTask creates a new GenerateCertsTask.
func NewGenerateCertsTask() task.Task {
	t := &GenerateCertsTask{}
	t.NameField = "etcd-generate-certs"
	t.DescriptionField = "Generates etcd CA and node certificates."
	t.BaseTask = task.NewBaseTask(t.NameField, t.DescriptionField) // Initialize BaseTask parts
	return t
}

// Default stores runtime, logger, and the taskSpec.
func (t *GenerateCertsTask) Default(runtime *krt.KubeRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error {
	if err := t.BaseTask.Default(runtime, taskSpec, moduleCache, taskCache); err != nil {
		return err
	}
	t.Logger = runtime.Log.WithFields(logrus.Fields{"task": t.Name(), "type": "GenerateCertsTask"})

	// Type assert and store taskSpec for use in Init and other methods
	// The TaskSpec field in BaseTask is interface{}, so we store it there.
	// No need for a separate field in GenerateCertsTask unless we want to avoid re-assertion.
	if _, ok := t.TaskSpec.(*config.EtcdSpec); !ok {
		return fmt.Errorf("invalid taskSpec type for %s: expected *config.EtcdSpec, got %T", t.Name(), t.TaskSpec)
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
        return fmt.Errorf("taskSpec not set for %s (must be set in Default method)", t.Name())
    }
	etcdSpec, ok := t.TaskSpec.(*config.EtcdSpec)
	if !ok {
		return fmt.Errorf("taskSpec for %s is not of type *config.EtcdSpec (type: %T)", t.Name(), t.TaskSpec)
	}

	if etcdSpec.Type == "external" {
		t.Logger.Info("Etcd type is external, assuming certificates are user-provided. No cert generation steps will be added.")
		return nil
	}

	t.Logger.Infof("Initializing certificate generation steps for etcd type: %s", etcdSpec.Type)

	caCmdStep := runcmd.NewRunCommandStep("GenerateEtcdCA", "Generates ETCD CA", "echo TODO: Generate ETCD CA certificate and key")
	stepLoggerCA := t.Logger.WithField("step", caCmdStep.Name())
	if err := caCmdStep.Init(t.Runtime, stepLoggerCA); err != nil {
		return fmt.Errorf("failed to init step '%s': %w", caCmdStep.Name(), err)
	}
	t.AddStep(caCmdStep)
	t.Logger.Debugf("Added step: %s", caCmdStep.Name())

	etcdHosts := t.Runtime.RoleHosts()["etcd"]
	if len(etcdHosts) == 0 && (etcdSpec.Type == "kubeadm" || etcdSpec.Type == "kubexm") {
		return fmt.Errorf("no hosts found in 'etcd' role for certificate generation (etcd type: %s)", etcdSpec.Type)
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
	return fmt.Sprintf("Generating ETCD certificates for type: %s...", t.TaskSpec.(*config.EtcdSpec).Type)
}

// Run, IsSkip, AutoAssert, Until, Steps are inherited from BaseTask.
// Override if specific behavior is needed.
var _ task.Task = (*GenerateCertsTask)(nil)
