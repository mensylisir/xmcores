package etcd

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

type EtcdModule struct {
	NameField        string
	DescriptionField string
	KubeRuntime      *krt.KubeRuntime
	ModuleSpec       *config.EtcdSpec // Store the typed spec
	Logger           *logrus.Entry
	tasks            []task.Task
	postHooks        []module.HookFn
	// pipelineCache    interface{}
	// moduleCache      interface{}
}

// NewEtcdModule is a helper constructor.
// The pipeline factory will call this or instantiate the struct directly, then call Default.
func NewEtcdModule() module.Module {
	return &EtcdModule{
		NameField:        "etcd",
		DescriptionField: "Manages etcd cluster setup (installation or configuration for external).",
		tasks:            make([]task.Task, 0),
		postHooks:        make([]module.HookFn, 0),
	}
}

func (m *EtcdModule) Name() string { return m.NameField }
func (m *EtcdModule) Description() string { return m.DescriptionField }

func (m *EtcdModule) IsSkip(runtime *krt.KubeRuntime) (bool, error) {
	if m.Logger == nil { // Logger might not be set if Default wasn't called (e.g. direct IsSkip check)
		// This is a fallback, ideally logger is set in Default before IsSkip.
		m.Logger = logrus.NewEntry(logrus.New()).WithField("module_early", m.NameField)
	}
	m.Logger.Debug("IsSkip called.")
	if runtime == nil || runtime.Cluster == nil {
		return false, fmt.Errorf("runtime or cluster spec is nil for IsSkip check in %s", m.NameField)
	}
	// Example: if etcd type is external, and this module's primary purpose was *installing* etcd,
	// then it might be skipped. However, this module might also handle configuring nodes to *use* external etcd.
	// For now, let's assume it's not skipped by default.
	// Specific logic would depend on m.ModuleSpec, which is set in Default.
	// If Default hasn't run, ModuleSpec would be nil.
	if m.ModuleSpec != nil && m.ModuleSpec.Type == "external" {
		m.Logger.Info("Etcd type is external, module might have specific tasks or could be partially skipped if only for installation.")
	}
	return false, nil
}

func (m *EtcdModule) Default(runtime *krt.KubeRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error {
	m.KubeRuntime = runtime
	m.Logger = runtime.Log.WithField("module", m.NameField)
	m.Logger.Info("Default called: runtime and logger set.")

	spec, ok := moduleSpec.(*config.EtcdSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.EtcdSpec, got %T", m.NameField, moduleSpec)
	}
	m.ModuleSpec = spec
	m.Logger.Infof("Module spec configured: Etcd Type '%s'", m.ModuleSpec.Type)

	// m.pipelineCache = pipelineCache
	// m.moduleCache = moduleCache
	return nil
}

func (m *EtcdModule) AutoAssert() error {
	m.Logger.Info("AutoAssert called.")
	if m.KubeRuntime == nil {
		return fmt.Errorf("runtime not initialized for %s (AutoAssert)", m.NameField)
	}
	if m.ModuleSpec == nil {
		return fmt.Errorf("moduleSpec not initialized for %s (AutoAssert)", m.NameField)
	}
	if m.ModuleSpec.Type != "external" && (m.KubeRuntime.RoleHosts()["etcd"] == nil || len(m.KubeRuntime.RoleHosts()["etcd"]) == 0) {
		return fmt.Errorf("etcd hosts are required for etcd type '%s' but no hosts found in roleGroup 'etcd'", m.ModuleSpec.Type)
	}
	if m.ModuleSpec.Type == "external" && len(m.ModuleSpec.Endpoints) == 0 {
		return fmt.Errorf("external etcd type requires 'endpoints' to be specified in EtcdSpec")
	}
	m.Logger.Info("AutoAssert completed successfully.")
	return nil
}

func (m *EtcdModule) Init() error {
	m.Logger.Info("Init called (task assembly).")
	// Based on m.ModuleSpec.Type, assemble tasks.
	// For example, if m.ModuleSpec.Type == "kubexm":
	//  generateCertsTask := etcdtasks.NewGenerateCertsTask() // Assuming a subpackage for etcd tasks
	//  taskLogger := m.Logger.WithField("task", generateCertsTask.Name())
	//  // Tasks also need Default and Init called by the module
	//  if err := generateCertsTask.Default(m.KubeRuntime, m.moduleCache, nil /*task cache*/); err != nil {
	//    return fmt.Errorf("failed to Default task %s: %w", generateCertsTask.Name(), err)
	//  }
	//  if err := generateCertsTask.Init(m.KubeRuntime, m.ModuleSpec, taskLogger); err != nil {
	//    return fmt.Errorf("failed to Init task %s: %w", generateCertsTask.Name(), err)
	//  }
	//  m.tasks = append(m.tasks, generateCertsTask)
	//  m.Logger.Infof("Task %s added to etcd module.", generateCertsTask.Name())

	m.Logger.Info("No concrete tasks assembled in this skeleton Init for EtcdModule.")
	return nil
}

func (m *EtcdModule) Run(result *ending.ModuleResult) {
	m.Logger.Info("Run called.")
	if len(m.tasks) == 0 {
		m.Logger.Info("No tasks to execute for this module based on current configuration.")
		// If no tasks, module can be considered successful or skipped based on intent
		// For now, if module was meant to do something (e.g. install kubexm etcd) but has no tasks,
		// it might be an issue. But if it's e.g. "external" etcd and tasks are only for validation,
		// having no tasks might be fine.
		// Let's default to success if no tasks and no prior errors.
		if result.Status == ending.ModuleResultPending { // Only set if not already failed/skipped by pre-checks
			result.SetStatus(ending.ModuleResultSuccess)
			result.SetMessage(fmt.Sprintf("%s: No tasks to execute.", m.NameField))
		}
		return
	}

	for _, tsk := range m.tasks {
		taskLogger := m.Logger.WithField("task", tsk.Name())
		taskLogger.Info(tsk.Slogan())

		// Similar lifecycle for tasks if they also implement it fully
		// skipTask, errSkip := tsk.IsSkip(m.KubeRuntime) ...
		// tsk.Default(...)
		// tsk.AutoAssert(...)
		// tsk.Init(...) -> this would add steps to the task

		tsk.Run(result) // Task updates the *same* module result.

		if result.IsFailed() {
			taskLogger.Errorf("Task %s reported failure. Message: %s", tsk.Name(), result.Message)
			if !m.KubeRuntime.IgnoreError {
				m.Logger.Errorf("Task %s failed, and not ignoring errors. Halting module %s.", tsk.Name(), m.NameField)
				return // Stop processing further tasks in this module
			}
			m.Logger.Warnf("Task %s failed, but IgnoreError is true. Continuing module %s.", tsk.Name(), m.NameField)
		} else if result.Status == ending.ModuleResultSkipped {
			 taskLogger.Infof("Task %s was skipped.", tsk.Name())
		} else {
			taskLogger.Infof("Task %s completed successfully.", tsk.Name())
		}
	}

	if !result.IsFailed() && result.Status != ending.ModuleResultSkipped {
		result.SetStatus(ending.ModuleResultSuccess) // Mark module success if all tasks ok
		result.SetMessage(fmt.Sprintf("%s executed successfully.", m.NameField))
	}
	m.Logger.Info("Run completed.")
}

func (m *EtcdModule) Until(runtime *krt.KubeRuntime) (done bool, err error) {
	m.Logger.Info("Until called (placeholder, returning true).")
	return true, nil
}

func (m *EtcdModule) CallPostHook(res *ending.ModuleResult) error {
	m.Logger.Info("CallPostHook called.")
	var firstError error
	for i, hook := range m.postHooks {
		m.Logger.Debugf("Executing post-hook %d for module %s", i+1, m.NameField)
		if err := hook(res); err != nil {
			m.Logger.Errorf("Error executing post-hook %d for module %s: %v", i+1, m.NameField, err)
			if firstError == nil {
				firstError = fmt.Errorf("error executing post-hook %d for module %s: %w", i+1, m.NameField, err)
			}
		}
	}
	m.Logger.Info("All post-hooks executed for module %s.", m.NameField)
	return firstError // Return the first error encountered, if any
}

func (m *EtcdModule) Is() module.Type {
	return module.TaskModuleType
}

func (m *EtcdModule) Slogan() string {
	if m.ModuleSpec != nil {
		return fmt.Sprintf("Configuring Etcd (type: %s)...", m.ModuleSpec.Type)
	}
	return "Configuring Etcd..."
}

func (m *EtcdModule) AppendPostHook(hookFn module.HookFn) {
	m.postHooks = append(m.postHooks, hookFn)
}

var _ module.Module = (*EtcdModule)(nil)
