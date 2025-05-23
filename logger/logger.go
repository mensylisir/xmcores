// logger.go
package logger

import (
	"fmt"
	"io" // Required for io.Discard
	"os"
	"path/filepath"
	"runtime"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"

	"github.com/mensylisir/xmcores/common" // Your actual common package path
)

// Log is the global logger instance of XMLog.
var Log *XMLog

// XMLog wraps logrus.FieldLogger for application-specific logging.
type XMLog struct {
	*logrus.Logger // Embed *logrus.Logger directly for direct access to all its methods
}

// InitGlobalLogger initializes the global Log variable.
func InitGlobalLogger(outputPath string, verbose bool, defaultLevel logrus.Level) error {
	logger := logrus.New()

	currentLogLevel := defaultLevel
	if verbose {
		currentLogLevel = logrus.DebugLevel
	}
	logger.SetLevel(currentLogLevel)
	logger.SetReportCaller(true) // Enable caller reporting

	formatterDisplayLevelConfig := ShowAboveWarn
	if verbose {
		formatterDisplayLevelConfig = ShowAll
	}

	defaultFieldsOrder := []string{
		common.PipelineName, common.ModuleName, common.TaskName, common.StepName, common.NodeName,
	}

	if outputPath != "" {
		// Ensure output path exists
		if err := os.MkdirAll(outputPath, 0755); err != nil {
			return fmt.Errorf("failed to create log output directory %s: %w", outputPath, err)
		}
		logFilePath := filepath.Join(outputPath, "app.log")

		writer, err := rotatelogs.New(
			logFilePath+".%Y%m%d", // Daily rotation
			rotatelogs.WithLinkName(logFilePath),
			rotatelogs.WithMaxAge(7*24*time.Hour),
			rotatelogs.WithRotationTime(24*time.Hour),
		)
		if err != nil {
			return fmt.Errorf("failed to initialize rotatelogs for %s: %w", logFilePath, err)
		}

		fileFormatter := &Formatter{
			TimestampFormat:        "2006-01-02 15:04:05.000 MST",
			NoColors:               true,
			DisplayLevelName:       formatterDisplayLevelConfig,
			FieldsDisplayWithOrder: defaultFieldsOrder,
			FieldSeparator:         " | ",
			DisableCaller:          false,
			CustomCallerFormatter: func(frame *runtime.Frame) string {
				return fmt.Sprintf(" [%s:%d %s]", filepath.Base(frame.File), frame.Line, filepath.Base(frame.Function))
			},
		}
		logger.SetFormatter(fileFormatter) // Set formatter for the logger instance

		logWriters := lfshook.WriterMap{}
		for _, level := range logrus.AllLevels {
			if logger.IsLevelEnabled(level) {
				logWriters[level] = writer
			}
		}
		if len(logWriters) > 0 {
			logger.Hooks.Add(lfshook.NewHook(logWriters, fileFormatter))
			// **CRITICAL FIX for empty files in test:**
			// Discard logrus's default output when file logging is handled by a hook.
			// This prevents potential race conditions or issues where the default output stream
			// (e.g., os.Stderr) might not be flushed properly or quickly enough in tests,
			// especially if the hook is also writing to the same underlying file descriptor indirectly.
			logger.SetOutput(io.Discard)
		}
	} else {
		// Configure for console output
		consoleFormatter := &Formatter{
			TimestampFormat:        "15:04:05",
			NoColors:               false, // Enable colors for console
			DisplayLevelName:       formatterDisplayLevelConfig,
			DisableCaller:          true, // Caller info often too verbose for console
			FieldsDisplayWithOrder: defaultFieldsOrder,
		}
		logger.SetFormatter(consoleFormatter)
		logger.SetOutput(os.Stdout) // Default to Stdout for console logs
	}

	Log = &XMLog{
		Logger: logger,
	}
	return nil
}

// NewXMLog creates a new instance of XMLog.
func NewXMLog(outputPath string, verbose bool, defaultLevel logrus.Level, forConsole bool) (*XMLog, error) {
	logger := logrus.New()
	currentLogLevel := defaultLevel
	if verbose {
		currentLogLevel = logrus.DebugLevel
	}
	logger.SetLevel(currentLogLevel)
	logger.SetReportCaller(true)

	formatterDisplayLevelConfig := ShowAboveWarn
	if verbose {
		formatterDisplayLevelConfig = ShowAll
	}

	defaultFieldsOrder := []string{
		common.PipelineName, common.ModuleName, common.TaskName, common.StepName, common.NodeName,
	}

	var chosenFormatter *Formatter // Use pointer to share Formatter struct
	if forConsole {
		chosenFormatter = &Formatter{
			TimestampFormat:        "15:04:05",
			NoColors:               false,
			DisplayLevelName:       formatterDisplayLevelConfig,
			DisableCaller:          true,
			FieldsDisplayWithOrder: defaultFieldsOrder,
		}
		logger.SetOutput(os.Stdout)
		logger.SetFormatter(chosenFormatter)
	} else if outputPath != "" {
		if err := os.MkdirAll(outputPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log output directory %s: %w", outputPath, err)
		}
		logFilePath := filepath.Join(outputPath, "instance.log") // Different name for instance log
		writer, err := rotatelogs.New(
			logFilePath+".%Y%m%d",
			rotatelogs.WithLinkName(logFilePath),
			rotatelogs.WithRotationTime(24*time.Hour),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize rotatelogs for instance: %w", err)
		}
		fileFormatter := &Formatter{
			TimestampFormat:        "2006-01-02 15:04:05.000 MST",
			NoColors:               true,
			DisplayLevelName:       formatterDisplayLevelConfig,
			FieldsDisplayWithOrder: defaultFieldsOrder,
			DisableCaller:          false,
		}
		chosenFormatter = fileFormatter
		logger.SetFormatter(chosenFormatter) // Set formatter for the logger instance

		logWriters := lfshook.WriterMap{}
		for _, level := range logrus.AllLevels {
			if logger.IsLevelEnabled(level) {
				logWriters[level] = writer
			}
		}
		if len(logWriters) > 0 {
			logger.Hooks.Add(lfshook.NewHook(logWriters, fileFormatter))
			logger.SetOutput(io.Discard) // Discard default output, rely on hook
		}
	} else {
		// Default to a simple console logger if no output path and not explicitly for console
		chosenFormatter = &Formatter{
			NoColors:               false,
			DisplayLevelName:       ShowAll, // Show all levels for a basic default console
			FieldsDisplayWithOrder: defaultFieldsOrder,
		}
		logger.SetFormatter(chosenFormatter)
		logger.SetOutput(os.Stdout)
	}

	return &XMLog{Logger: logger}, nil
}

// --- Internal Helper Methods ---
func (xl *XMLog) logWithStandardFields(level logrus.Level, fixedFields logrus.Fields, message string, dynamicFields ...logrus.Fields) {
	entry := xl.Logger.WithFields(fixedFields)
	if len(dynamicFields) > 0 && dynamicFields[0] != nil {
		entry = entry.WithFields(dynamicFields[0])
	}
	switch level {
	case logrus.TraceLevel:
		entry.Trace(message)
	case logrus.DebugLevel:
		entry.Debug(message)
	case logrus.InfoLevel:
		entry.Info(message)
	case logrus.WarnLevel:
		entry.Warn(message)
	case logrus.ErrorLevel:
		entry.Error(message)
	case logrus.FatalLevel:
		entry.Fatal(message)
	case logrus.PanicLevel:
		entry.Panic(message)
	default:
		entry.Print(message)
	}
}

func (xl *XMLog) logfWithStandardFields(level logrus.Level, fixedFields logrus.Fields, format string, args []interface{}, dynamicFields ...logrus.Fields) {
	entry := xl.Logger.WithFields(fixedFields)
	if len(dynamicFields) > 0 && dynamicFields[0] != nil {
		entry = entry.WithFields(dynamicFields[0])
	}
	switch level {
	case logrus.TraceLevel:
		entry.Tracef(format, args...)
	case logrus.DebugLevel:
		entry.Debugf(format, args...)
	case logrus.InfoLevel:
		entry.Infof(format, args...)
	case logrus.WarnLevel:
		entry.Warnf(format, args...)
	case logrus.ErrorLevel:
		entry.Errorf(format, args...)
	case logrus.FatalLevel:
		entry.Fatalf(format, args...)
	case logrus.PanicLevel:
		entry.Panicf(format, args...)
	default:
		entry.Printf(format, args...)
	}
}

// --- Pipeline Context Logging ---
func (xl *XMLog) TracePipeline(pipelineName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.TraceLevel, logrus.Fields{common.PipelineName: pipelineName}, message, dynamicFields...)
}
func (xl *XMLog) TracefPipeline(pipelineName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.TraceLevel, logrus.Fields{common.PipelineName: pipelineName}, format, args)
}
func (xl *XMLog) DebugPipeline(pipelineName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.DebugLevel, logrus.Fields{common.PipelineName: pipelineName}, message, dynamicFields...)
}
func (xl *XMLog) DebugfPipeline(pipelineName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.DebugLevel, logrus.Fields{common.PipelineName: pipelineName}, format, args)
}
func (xl *XMLog) InfoPipeline(pipelineName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.InfoLevel, logrus.Fields{common.PipelineName: pipelineName}, message, dynamicFields...)
}
func (xl *XMLog) InfofPipeline(pipelineName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.InfoLevel, logrus.Fields{common.PipelineName: pipelineName}, format, args)
}
func (xl *XMLog) WarnPipeline(pipelineName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.WarnLevel, logrus.Fields{common.PipelineName: pipelineName}, message, dynamicFields...)
}
func (xl *XMLog) WarnfPipeline(pipelineName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.WarnLevel, logrus.Fields{common.PipelineName: pipelineName}, format, args)
}
func (xl *XMLog) ErrorPipeline(pipelineName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.PipelineName: pipelineName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.ErrorLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) ErrorfPipeline(pipelineName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.PipelineName: pipelineName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.ErrorLevel, fixedFields, format, args)
}
func (xl *XMLog) FatalPipeline(pipelineName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.PipelineName: pipelineName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.FatalLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) FatalfPipeline(pipelineName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.PipelineName: pipelineName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.FatalLevel, fixedFields, format, args)
}
func (xl *XMLog) PanicPipeline(pipelineName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.PipelineName: pipelineName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.PanicLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) PanicfPipeline(pipelineName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.PipelineName: pipelineName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.PanicLevel, fixedFields, format, args)
}

// --- Module Context Logging ---
func (xl *XMLog) TraceModule(moduleName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.TraceLevel, logrus.Fields{common.ModuleName: moduleName}, message, dynamicFields...)
}
func (xl *XMLog) TracefModule(moduleName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.TraceLevel, logrus.Fields{common.ModuleName: moduleName}, format, args)
}
func (xl *XMLog) DebugModule(moduleName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.DebugLevel, logrus.Fields{common.ModuleName: moduleName}, message, dynamicFields...)
}
func (xl *XMLog) DebugfModule(moduleName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.DebugLevel, logrus.Fields{common.ModuleName: moduleName}, format, args)
}
func (xl *XMLog) InfoModule(moduleName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.InfoLevel, logrus.Fields{common.ModuleName: moduleName}, message, dynamicFields...)
}
func (xl *XMLog) InfofModule(moduleName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.InfoLevel, logrus.Fields{common.ModuleName: moduleName}, format, args)
}
func (xl *XMLog) WarnModule(moduleName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.WarnLevel, logrus.Fields{common.ModuleName: moduleName}, message, dynamicFields...)
}
func (xl *XMLog) WarnfModule(moduleName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.WarnLevel, logrus.Fields{common.ModuleName: moduleName}, format, args)
}
func (xl *XMLog) ErrorModule(moduleName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.ModuleName: moduleName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.ErrorLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) ErrorfModule(moduleName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.ModuleName: moduleName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.ErrorLevel, fixedFields, format, args)
}
func (xl *XMLog) FatalModule(moduleName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.ModuleName: moduleName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.FatalLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) FatalfModule(moduleName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.ModuleName: moduleName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.FatalLevel, fixedFields, format, args)
}
func (xl *XMLog) PanicModule(moduleName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.ModuleName: moduleName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.PanicLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) PanicfModule(moduleName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.ModuleName: moduleName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.PanicLevel, fixedFields, format, args)
}

// --- Task Context Logging ---
func (xl *XMLog) TraceTask(taskName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.TraceLevel, logrus.Fields{common.TaskName: taskName}, message, dynamicFields...)
}
func (xl *XMLog) TracefTask(taskName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.TraceLevel, logrus.Fields{common.TaskName: taskName}, format, args)
}
func (xl *XMLog) DebugTask(taskName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.DebugLevel, logrus.Fields{common.TaskName: taskName}, message, dynamicFields...)
}
func (xl *XMLog) DebugfTask(taskName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.DebugLevel, logrus.Fields{common.TaskName: taskName}, format, args)
}
func (xl *XMLog) InfoTask(taskName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.InfoLevel, logrus.Fields{common.TaskName: taskName}, message, dynamicFields...)
}
func (xl *XMLog) InfofTask(taskName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.InfoLevel, logrus.Fields{common.TaskName: taskName}, format, args)
}
func (xl *XMLog) WarnTask(taskName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.WarnLevel, logrus.Fields{common.TaskName: taskName}, message, dynamicFields...)
}
func (xl *XMLog) WarnfTask(taskName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.WarnLevel, logrus.Fields{common.TaskName: taskName}, format, args)
}
func (xl *XMLog) ErrorTask(taskName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.TaskName: taskName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.ErrorLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) ErrorfTask(taskName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.TaskName: taskName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.ErrorLevel, fixedFields, format, args)
}
func (xl *XMLog) FatalTask(taskName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.TaskName: taskName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.FatalLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) FatalfTask(taskName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.TaskName: taskName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.FatalLevel, fixedFields, format, args)
}
func (xl *XMLog) PanicTask(taskName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.TaskName: taskName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.PanicLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) PanicfTask(taskName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.TaskName: taskName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.PanicLevel, fixedFields, format, args)
}

// --- Step Context Logging ---
func (xl *XMLog) TraceStep(stepName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.TraceLevel, logrus.Fields{common.StepName: stepName}, message, dynamicFields...)
}
func (xl *XMLog) TracefStep(stepName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.TraceLevel, logrus.Fields{common.StepName: stepName}, format, args)
}
func (xl *XMLog) DebugStep(stepName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.DebugLevel, logrus.Fields{common.StepName: stepName}, message, dynamicFields...)
}
func (xl *XMLog) DebugfStep(stepName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.DebugLevel, logrus.Fields{common.StepName: stepName}, format, args)
}
func (xl *XMLog) InfoStep(stepName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.InfoLevel, logrus.Fields{common.StepName: stepName}, message, dynamicFields...)
}
func (xl *XMLog) InfofStep(stepName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.InfoLevel, logrus.Fields{common.StepName: stepName}, format, args)
}
func (xl *XMLog) WarnStep(stepName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.WarnLevel, logrus.Fields{common.StepName: stepName}, message, dynamicFields...)
}
func (xl *XMLog) WarnfStep(stepName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.WarnLevel, logrus.Fields{common.StepName: stepName}, format, args)
}
func (xl *XMLog) ErrorStep(stepName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.StepName: stepName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.ErrorLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) ErrorfStep(stepName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.StepName: stepName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.ErrorLevel, fixedFields, format, args)
}
func (xl *XMLog) FatalStep(stepName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.StepName: stepName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.FatalLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) FatalfStep(stepName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.StepName: stepName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.FatalLevel, fixedFields, format, args)
}
func (xl *XMLog) PanicStep(stepName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.StepName: stepName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.PanicLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) PanicfStep(stepName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.StepName: stepName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.PanicLevel, fixedFields, format, args)
}

// --- Node Context Logging ---
func (xl *XMLog) TraceNode(nodeName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.TraceLevel, logrus.Fields{common.NodeName: nodeName}, message, dynamicFields...)
}
func (xl *XMLog) TracefNode(nodeName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.TraceLevel, logrus.Fields{common.NodeName: nodeName}, format, args)
}
func (xl *XMLog) DebugNode(nodeName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.DebugLevel, logrus.Fields{common.NodeName: nodeName}, message, dynamicFields...)
}
func (xl *XMLog) DebugfNode(nodeName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.DebugLevel, logrus.Fields{common.NodeName: nodeName}, format, args)
}
func (xl *XMLog) InfoNode(nodeName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.InfoLevel, logrus.Fields{common.NodeName: nodeName}, message, dynamicFields...)
}
func (xl *XMLog) InfofNode(nodeName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.InfoLevel, logrus.Fields{common.NodeName: nodeName}, format, args)
}
func (xl *XMLog) WarnNode(nodeName string, message string, dynamicFields ...logrus.Fields) {
	xl.logWithStandardFields(logrus.WarnLevel, logrus.Fields{common.NodeName: nodeName}, message, dynamicFields...)
}
func (xl *XMLog) WarnfNode(nodeName string, format string, args ...interface{}) {
	xl.logfWithStandardFields(logrus.WarnLevel, logrus.Fields{common.NodeName: nodeName}, format, args)
}
func (xl *XMLog) ErrorNode(nodeName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.NodeName: nodeName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.ErrorLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) ErrorfNode(nodeName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.NodeName: nodeName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.ErrorLevel, fixedFields, format, args)
}
func (xl *XMLog) FatalNode(nodeName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.NodeName: nodeName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.FatalLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) FatalfNode(nodeName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.NodeName: nodeName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.FatalLevel, fixedFields, format, args)
}
func (xl *XMLog) PanicNode(nodeName string, err error, message string, dynamicFields ...logrus.Fields) {
	fixedFields := logrus.Fields{common.NodeName: nodeName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logWithStandardFields(logrus.PanicLevel, fixedFields, message, dynamicFields...)
}
func (xl *XMLog) PanicfNode(nodeName string, err error, format string, args ...interface{}) {
	fixedFields := logrus.Fields{common.NodeName: nodeName}
	if err != nil {
		fixedFields["error"] = err
	}
	xl.logfWithStandardFields(logrus.PanicLevel, fixedFields, format, args)
}

// --- General Purpose Structured Log with Level ---
// Debug logs a message at level Debug on the standard logger.
func (xl *XMLog) Debug(args ...interface{}) {
	xl.Logger.Debug(args...)
}

// Debugf logs a formatted message at level Debug on the standard logger.
func (xl *XMLog) Debugf(format string, args ...interface{}) {
	xl.Logger.Debugf(format, args...)
}

// Info logs a message at level Info on the standard logger.
func (xl *XMLog) Info(args ...interface{}) {
	xl.Logger.Info(args...)
}

// Infof logs a formatted message at level Info on the standard logger.
func (xl *XMLog) Infof(format string, args ...interface{}) {
	xl.Logger.Infof(format, args...)
}

// Warn logs a message at level Warn on the standard logger.
func (xl *XMLog) Warn(args ...interface{}) {
	xl.Logger.Warn(args...)
}

// Warnf logs a formatted message at level Warn on the standard logger.
func (xl *XMLog) Warnf(format string, args ...interface{}) {
	xl.Logger.Warnf(format, args...)
}

// Error logs a message at level Error on the standard logger.
func (xl *XMLog) Error(args ...interface{}) {
	xl.Logger.Error(args...)
}

// Errorf logs a formatted message at level Error on the standard logger.
func (xl *XMLog) Errorf(format string, args ...interface{}) {
	xl.Logger.Errorf(format, args...)
}

// Fatal logs a message at level Fatal on the standard logger then the process will exit.
func (xl *XMLog) Fatal(args ...interface{}) {
	xl.Logger.Fatal(args...)
}

// Fatalf logs a formatted message at level Fatal on the standard logger then the process will exit.
func (xl *XMLog) Fatalf(format string, args ...interface{}) {
	xl.Logger.Fatalf(format, args...)
}

// Panic logs a message at level Panic on the standard logger then the process will panic.
func (xl *XMLog) Panic(args ...interface{}) {
	xl.Logger.Panic(args...)
}

// Panicf logs a formatted message at level Panic on the standard logger then the process will panic.
func (xl *XMLog) Panicf(format string, args ...interface{}) {
	xl.Logger.Panicf(format, args...)
}

// Trace logs a message at level Trace on the standard logger.
func (xl *XMLog) Trace(args ...interface{}) {
	xl.Logger.Trace(args...)
}

// Tracef logs a formatted message at level Trace on the standard logger.
func (xl *XMLog) Tracef(format string, args ...interface{}) {
	xl.Logger.Tracef(format, args...)
}

// Print logs a message at the Print level (typically Info).
func (xl *XMLog) Print(args ...interface{}) {
	xl.Logger.Print(args...)
}

// Printf logs a formatted message at the Print level.
func (xl *XMLog) Printf(format string, args ...interface{}) {
	xl.Logger.Printf(format, args...)
}

// Println logs a message at the Print level, adding a newline.
func (xl *XMLog) Println(args ...interface{}) {
	xl.Logger.Println(args...)
}

// LogAtLevel provides a general way to log with a specific level, message, and fields.
func (xl *XMLog) LogAtLevel(level logrus.Level, message string, fields logrus.Fields) {
	xl.WithFields(fields).Log(level, message)
}

// LogfAtLevel provides a general way to log a formatted message with a specific level and fields.
func (xl *XMLog) LogfAtLevel(level logrus.Level, fields logrus.Fields, format string, args ...interface{}) {
	xl.WithFields(fields).Logf(level, format, args...)
}

// --- Deprecated/Original Methods (can be removed if new ones are adopted) ---

// Message logs a structured message with a "node" field (Info level).
// Deprecated: Use InfoNode instead for clarity.
func (xl *XMLog) Message(node, str string) {
	xl.InfoNode(node, str) // Calls the new InfoNode
}

// Messagef logs a structured, formatted message with a "node" field (Info level).
// Deprecated: Use InfofNode instead for clarity.
func (xl *XMLog) Messagef(node, format string, args ...interface{}) {
	xl.InfofNode(node, format, args...) // Calls the new InfofNode
}

// InfoWithPipeline was an example, now superseded by full set of Pipeline methods.
// func (xl *XMLog) InfoWithPipeline(pipelineName string, message string, fields ...map[string]interface{}) {
// 	entry := xl.WithField(common.PipelineName, pipelineName)
// 	if len(fields) > 0 && fields[0] != nil {
// 		entry = entry.WithFields(fields[0]) // Note: logrus.Fields is map[string]interface{}
// 	}
// 	entry.Info(message)
// }

// ErrorWithTask was an example, now superseded by full set of Task methods.
// func (xl *XMLog) ErrorWithTask(taskName string, err error, message string, fields ...map[string]interface{}) {
// 	entry := xl.WithFields(logrus.Fields{
// 		common.TaskName: taskName,
// 		"error":         err,
// 	})
// 	if len(fields) > 0 && fields[0] != nil {
// 		entry = entry.WithFields(fields[0])
// 	}
// 	entry.Error(message)
// }
