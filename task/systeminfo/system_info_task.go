package systeminfo

import (
	"github.com/mensylisir/xmcores/step"
	"github.com/mensylisir/xmcores/step/runcmd" // Import the new runcmd package
	"github.com/mensylisir/xmcores/task"
)

// SystemInfoTask defines a task to gather basic system information.
type SystemInfoTask struct {
	task.BaseTask
}

// NewSystemInfoTask creates a new SystemInfoTask.
func NewSystemInfoTask() *SystemInfoTask {
	t := &SystemInfoTask{
		BaseTask: *task.NewBaseTask("system-info", "Gathers basic system information from the target host."),
	}

	// Define the steps for this task
	steps := []step.Step{
		runcmd.NewRunCommandStep("GetHostname", "Retrieves the system hostname", "hostname"),
		runcmd.NewRunCommandStep("GetOSVersion", "Retrieves OS kernel information", "uname -a"),
		runcmd.NewRunCommandStep("GetUptime", "Retrieves system uptime", "uptime"),
		runcmd.NewRunCommandStep("GetMemoryUsage", "Retrieves memory usage (in MB)", "free -m"),
		runcmd.NewRunCommandStep("GetDiskUsageRoot", "Retrieves disk usage for /", "df -h /"),
	}

	// Set the steps for the task
	// Assuming BaseTask has a method like SetSteps or steps are managed via its internal list.
	// If BaseTask.Steps is a public field:
	// t.Steps = steps
	// If there's a setter:
	t.SetSteps(steps) // This assumes BaseTask has a SetSteps method.

	return t
}

// Ensure SystemInfoTask implements the Task interface (implicitly via BaseTask)
// var _ task.Task = (*SystemInfoTask)(nil) // This check is usually for specific interface methods
                                         // not covered by embedding, but BaseTask should handle task.Task.
                                         // If BaseTask itself doesn't explicitly satisfy task.Task for some reason,
                                         // or if SystemInfoTask needs to override something, this might be needed.
                                         // For now, assuming BaseTask handles it.
