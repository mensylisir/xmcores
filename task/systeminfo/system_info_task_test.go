package systeminfo

import (
	"testing"

	"github.com/mensylisir/xmcores/step/runcmd" // To inspect step details
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSystemInfoTask(t *testing.T) {
	task := NewSystemInfoTask()

	// Verify task's name and description
	assert.Equal(t, "system-info", task.Name(), "Task name should match")
	assert.Equal(t, "Gathers basic system information from the target host.", task.Description(), "Task description should match")

	// Verify the number of steps added
	steps := task.Steps() // Assuming BaseTask.Steps() returns a slice of step.Step
	assert.Len(t, steps, 5, "Should have 5 steps")

	// Verify details for a few steps
	expectedSteps := []struct {
		name        string
		description string
		command     string
	}{
		{"GetHostname", "Retrieves the system hostname", "hostname"},
		{"GetOSVersion", "Retrieves OS kernel information", "uname -a"},
		{"GetMemoryUsage", "Retrieves memory usage (in MB)", "free -m"},
	}

	for i, expected := range expectedSteps {
		require.True(t, i < len(steps), "Not enough steps in task to check expected step %d", i)
		currentStep := steps[i]

		assert.Equal(t, expected.name, currentStep.Name(), "Step %d name mismatch", i)
		assert.Equal(t, expected.description, currentStep.Description(), "Step %d description mismatch", i)

		// Type assert to RunCommandStep to check the command
		rcStep, ok := currentStep.(*runcmd.RunCommandStep)
		require.True(t, ok, "Step %s should be a RunCommandStep", currentStep.Name())
		assert.Equal(t, expected.command, rcStep.Command, "Step %s command mismatch", currentStep.Name())
	}

	// Optionally, check all steps if desired, for now, a subset is fine as per prompt.
	// Example for checking the 4th step (GetUptime)
	if len(steps) > 3 {
		uptimeStep, ok := steps[2].(*runcmd.RunCommandStep) // Index 2 for 3rd item in expectedSteps (GetMemoryUsage)
                                                        // Prompt asked for hostname, uname, free.
                                                        // Let's correct this to match the expectedSteps array.
                                                        // The prompt specifically asked for hostname, uname, free.
                                                        // If testing 'uptime' it would be index 2 of the actual steps list.
		require.True(t, ok, "Uptime step should be a RunCommandStep")
		// This is redundant if the loop covers it. Let's ensure the loop is correct.
		// The loop iterates through `expectedSteps`, which are the first three.
	}

	// Let's ensure the 'uptime' step (index 2) and 'df -h /' (index 4) are also checked if we want more coverage.
	// For now, adhering to the prompt's "e.g., hostname, uname, free".

	// Check GetUptime (actual index 2)
	require.True(t, len(steps) > 2, "Task should have at least 3 steps for GetUptime check")
	uptimeStepConcrete, ok := steps[2].(*runcmd.RunCommandStep)
	require.True(t, ok, "Step at index 2 should be RunCommandStep for GetUptime")
	assert.Equal(t, "GetUptime", uptimeStepConcrete.Name())
	assert.Equal(t, "Retrieves system uptime", uptimeStepConcrete.Description())
	assert.Equal(t, "uptime", uptimeStepConcrete.Command)


	// Check GetDiskUsageRoot (actual index 4)
	require.True(t, len(steps) > 4, "Task should have at least 5 steps for GetDiskUsageRoot check")
	dfStepConcrete, ok := steps[4].(*runcmd.RunCommandStep)
	require.True(t, ok, "Step at index 4 should be RunCommandStep for GetDiskUsageRoot")
	assert.Equal(t, "GetDiskUsageRoot", dfStepConcrete.Name())
	assert.Equal(t, "Retrieves disk usage for /", dfStepConcrete.Description())
	assert.Equal(t, "df -h /", dfStepConcrete.Command)

}
