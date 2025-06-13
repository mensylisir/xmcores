package etcd

import (
	"fmt"

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step/runcmd" // For placeholder steps
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// GenerateCertsTask is responsible for generating etcd certificates.
type GenerateCertsTask struct {
	task.BaseTask
	// spec *config.EtcdSpec // Store the typed spec if needed by Execute directly
}

// NewGenerateCertsTask creates a new GenerateCertsTask.
func NewGenerateCertsTask() task.Task {
	return &GenerateCertsTask{
		BaseTask: task.NewBaseTask("etcd-generate-certs", "Generates etcd CA and node certificates."),
	}
}

// Init initializes the GenerateCertsTask.
// taskSpec is expected to be *config.EtcdSpec or a more specific cert generation config.
func (t *GenerateCertsTask) Init(moduleRuntime runtime.Runtime, taskSpec interface{}, logger *logrus.Entry) error {
	if err := t.BaseTask.Init(moduleRuntime, taskSpec, logger); err != nil {
		return err
	}

	etcdSpec, ok := taskSpec.(*config.EtcdSpec)
	if !ok {
		return fmt.Errorf("invalid taskSpec type for %s: expected *config.EtcdSpec, got %T", t.Name(), taskSpec)
	}
	// t.spec = etcdSpec // Store if Execute needs it directly

	t.BaseTask.logger.Infof("Initializing GenerateCertsTask with Etcd type: %s", etcdSpec.Type)
	if etcdSpec.Type == "external" {
		t.BaseTask.logger.Info("Etcd type is external, assuming certificates are user-provided. No cert generation steps will be added by this task.")
		// For external etcd, certs are provided by the user (caFile, certFile, keyFile).
		// This task might validate their existence if paths are absolute or resolvable.
		return nil
	}

	// Conceptual steps for generating certs (e.g., for kubeadm or xm type etcd)
	// These would typically not run on remote hosts directly but on a control/admin machine,
	// or use cfssl/easyrsa local tools.
	// For now, using placeholder RunCommandSteps that would echo their intent.

	// Placeholder for creating CA
	// In reality, this step would use a crypto library or call a local script.
	// The runtime for these steps might be a "localhost" runtime.
	// For simplicity, these echo commands don't need a specific runtime target for now.
	caCmdStep := runcmd.NewRunCommandStep("GenerateEtcdCA", "Generates ETCD CA", "echo TODO: Generate ETCD CA certificate and key")
	stepLoggerCA := t.BaseTask.logger.WithField("step_name", caCmdStep.Name())
	if err := caCmdStep.Init(t.BaseTask.runtime, stepLoggerCA); err != nil { // Using runtime from BaseTask
		return fmt.Errorf("failed to init step '%s': %w", caCmdStep.Name(), err)
	}
	t.AddStep(caCmdStep)
	t.BaseTask.logger.Debugf("Added step: %s", caCmdStep.Name())

	// Placeholder for creating node certificates for each etcd host
	// This would iterate over hosts in the "etcd" role.
	etcdHosts := t.BaseTask.runtime.RoleHosts()["etcd"]
	if len(etcdHosts) == 0 && etcdSpec.Type != "external" {
		 // If not external, we need etcd hosts to generate certs for.
		return fmt.Errorf("no hosts found in 'etcd' role for certificate generation (etcd type: %s)", etcdSpec.Type)
	}

	for _, host := range etcdHosts {
		nodeCertCmd := fmt.Sprintf("echo TODO: Generate ETCD certificate for node %s using CA", host.GetName())
		nodeCertStep := runcmd.NewRunCommandStep(
			fmt.Sprintf("GenerateEtcdCert-%s", host.GetName()),
			fmt.Sprintf("Generates ETCD certificate for node %s", host.GetName()),
			nodeCertCmd,
		)
		stepLoggerNode := t.BaseTask.logger.WithField("step_name", nodeCertStep.Name())
		if err := nodeCertStep.Init(t.BaseTask.runtime, stepLoggerNode); err != nil {
			return fmt.Errorf("failed to init step '%s': %w", nodeCertStep.Name(), err)
		}
		t.AddStep(nodeCertStep)
		t.BaseTask.logger.Debugf("Added step: %s", nodeCertStep.Name())
	}

	t.BaseTask.logger.Info("GenerateCertsTask initialized with certificate generation steps.")
	return nil
}

// Execute method is inherited from BaseTask.
// var _ task.Task = (*GenerateCertsTask)(nil) // Ensured by BaseTask embedding.
