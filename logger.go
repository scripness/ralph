package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EventType represents the type of log event
type EventType string

const (
	EventRunStart       EventType = "run_start"
	EventRunEnd         EventType = "run_end"
	EventIterationStart EventType = "iteration_start"
	EventIterationEnd   EventType = "iteration_end"
	EventStoryStart     EventType = "story_start"
	EventStoryEnd       EventType = "story_end"
	EventProviderStart  EventType = "provider_start"
	EventProviderEnd    EventType = "provider_end"
	EventProviderOutput EventType = "provider_output"
	EventMarkerDetected EventType = "marker_detected"
	EventVerifyStart    EventType = "verify_start"
	EventVerifyEnd      EventType = "verify_end"
	EventVerifyCmdStart EventType = "verify_cmd_start"
	EventVerifyCmdEnd   EventType = "verify_cmd_end"
	EventBrowserStart   EventType = "browser_start"
	EventBrowserEnd     EventType = "browser_end"
	EventBrowserStep    EventType = "browser_step"
	EventServiceStart   EventType = "service_start"
	EventServiceReady   EventType = "service_ready"
	EventServiceRestart EventType = "service_restart"
	EventServiceHealth  EventType = "service_health"
	EventStateChange    EventType = "state_change"
	EventLearning       EventType = "learning"
	EventProviderLine   EventType = "provider_line"
	EventWarning        EventType = "warning"
	EventError          EventType = "error"
)

// Event represents a single log event
type Event struct {
	Timestamp time.Time              `json:"ts"`
	Type      EventType              `json:"type"`
	Iteration int                    `json:"iter,omitempty"`
	StoryID   string                 `json:"story,omitempty"`
	Duration  *int64                 `json:"duration,omitempty"` // nanoseconds
	Success   *bool                  `json:"success,omitempty"`
	Message   string                 `json:"msg,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// RunLogger handles logging for a single Ralph run
type RunLogger struct {
	file         *os.File
	encoder      *json.Encoder
	mu           sync.Mutex
	runNumber    int
	iteration    int
	currentStory string
	startTime    time.Time
	featureDir   string
	enabled      bool
	config       *LoggingConfig

	// Duration tracking
	iterationStart time.Time
	providerStart  time.Time
	verifyStart    time.Time
}

// LoggingConfig configures the logging system
type LoggingConfig struct {
	Enabled           bool `json:"enabled"`
	MaxRuns           int  `json:"maxRuns"`
	ConsoleTimestamps bool `json:"consoleTimestamps"`
	ConsoleDurations  bool `json:"consoleDurations"`
}

// DefaultLoggingConfig returns sensible defaults
func DefaultLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Enabled:           true,
		MaxRuns:           10,
		ConsoleTimestamps: true,
		ConsoleDurations:  true,
	}
}

// NewRunLogger creates a new logger for a run
func NewRunLogger(featureDir string, config *LoggingConfig) (*RunLogger, error) {
	if config == nil {
		config = DefaultLoggingConfig()
	}

	logger := &RunLogger{
		featureDir: featureDir,
		startTime:  time.Now(),
		enabled:    config.Enabled,
		config:     config,
	}

	if !config.Enabled {
		return logger, nil
	}

	// Create logs directory
	logsDir := filepath.Join(featureDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Determine run number
	runNumber := nextRunNumber(logsDir)
	logger.runNumber = runNumber

	// Rotate old runs
	if config.MaxRuns > 0 {
		rotateOldRuns(logsDir, config.MaxRuns)
	}

	// Create log file
	logPath := filepath.Join(logsDir, fmt.Sprintf("run-%03d.jsonl", runNumber))
	file, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	logger.file = file
	logger.encoder = json.NewEncoder(file)

	return logger, nil
}

// Close closes the log file
func (l *RunLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// RunNumber returns the current run number
func (l *RunLogger) RunNumber() int {
	return l.runNumber
}

// LogPath returns the path to the current log file
func (l *RunLogger) LogPath() string {
	if l.file != nil {
		return l.file.Name()
	}
	return ""
}

// SetIteration sets the current iteration number
func (l *RunLogger) SetIteration(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.iteration = n
}

// SetCurrentStory sets the current story ID
func (l *RunLogger) SetCurrentStory(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.currentStory = id
}

// Log writes a generic event
func (l *RunLogger) Log(eventType EventType, data map[string]interface{}) {
	l.LogWithStory(eventType, "", data)
}

// LogWithStory writes an event with story context
func (l *RunLogger) LogWithStory(eventType EventType, storyID string, data map[string]interface{}) {
	if !l.enabled || l.file == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	event := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Iteration: l.iteration,
		StoryID:   storyID,
		Data:      data,
	}

	if storyID == "" && l.currentStory != "" {
		event.StoryID = l.currentStory
	}

	l.encoder.Encode(event)
}

// logEvent is an internal helper that writes an event with all fields
func (l *RunLogger) logEvent(event Event) {
	if !l.enabled || l.file == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Iteration == 0 {
		event.Iteration = l.iteration
	}
	if event.StoryID == "" {
		event.StoryID = l.currentStory
	}

	l.encoder.Encode(event)
}

// Convenience methods for specific event types

// RunStart logs the start of a run
func (l *RunLogger) RunStart(feature, branch string, storyCount int) {
	l.logEvent(Event{
		Type: EventRunStart,
		Data: map[string]interface{}{
			"feature":    feature,
			"branch":     branch,
			"stories":    storyCount,
			"run_number": l.runNumber,
		},
	})
}

// RunEnd logs the end of a run
func (l *RunLogger) RunEnd(success bool, summary string) {
	duration := time.Since(l.startTime).Nanoseconds()
	l.logEvent(Event{
		Type:     EventRunEnd,
		Duration: &duration,
		Success:  &success,
		Message:  summary,
	})
}

// IterationStart logs the start of an iteration
func (l *RunLogger) IterationStart(storyID, title string, retries int) {
	l.iterationStart = time.Now()
	l.logEvent(Event{
		Type:    EventIterationStart,
		StoryID: storyID,
		Data: map[string]interface{}{
			"title":   title,
			"retries": retries,
		},
	})
}

// StoryStart logs the start of a story (alias for more granular tracking)
func (l *RunLogger) StoryStart(storyID, title string) {
	l.logEvent(Event{
		Type:    EventStoryStart,
		StoryID: storyID,
		Data: map[string]interface{}{
			"title": title,
		},
	})
}

// StoryEnd logs the end of a story
func (l *RunLogger) StoryEnd(storyID string, success bool) {
	l.logEvent(Event{
		Type:    EventStoryEnd,
		StoryID: storyID,
		Success: &success,
	})
}

// IterationEnd logs the end of an iteration
func (l *RunLogger) IterationEnd(success bool) {
	duration := time.Since(l.iterationStart).Nanoseconds()
	l.logEvent(Event{
		Type:     EventIterationEnd,
		Duration: &duration,
		Success:  &success,
	})
}

// ProviderStart logs the start of provider execution
func (l *RunLogger) ProviderStart() {
	l.providerStart = time.Now()
	l.logEvent(Event{
		Type: EventProviderStart,
	})
}

// ProviderEnd logs the end of provider execution
func (l *RunLogger) ProviderEnd(exitCode int, timedOut bool, markers []string) {
	duration := time.Since(l.providerStart).Nanoseconds()
	success := exitCode == 0 && !timedOut
	l.logEvent(Event{
		Type:     EventProviderEnd,
		Duration: &duration,
		Success:  &success,
		Data: map[string]interface{}{
			"exit_code": exitCode,
			"timed_out": timedOut,
			"markers":   markers,
		},
	})
}

// ProviderOutput logs the full provider output
func (l *RunLogger) ProviderOutput(stdout, stderr string) {
	l.logEvent(Event{
		Type: EventProviderOutput,
		Data: map[string]interface{}{
			"stdout": stdout,
			"stderr": stderr,
		},
	})
}

// ProviderLine logs a single line of provider output in real-time
func (l *RunLogger) ProviderLine(stream, line string) {
	l.logEvent(Event{
		Type: EventProviderLine,
		Data: map[string]interface{}{
			"stream": stream,
			"line":   line,
		},
	})
}

// MarkerDetected logs a detected marker
func (l *RunLogger) MarkerDetected(marker, value string) {
	l.logEvent(Event{
		Type: EventMarkerDetected,
		Data: map[string]interface{}{
			"marker": marker,
			"value":  value,
		},
	})
}

// VerifyStart logs the start of verification
func (l *RunLogger) VerifyStart() {
	l.verifyStart = time.Now()
	l.logEvent(Event{
		Type: EventVerifyStart,
	})
}

// VerifyEnd logs the end of verification
func (l *RunLogger) VerifyEnd(success bool) {
	duration := time.Since(l.verifyStart).Nanoseconds()
	l.logEvent(Event{
		Type:     EventVerifyEnd,
		Duration: &duration,
		Success:  &success,
	})
}

// VerifyCmdStart logs the start of a verification command
func (l *RunLogger) VerifyCmdStart(cmd string) {
	l.logEvent(Event{
		Type: EventVerifyCmdStart,
		Data: map[string]interface{}{
			"cmd": cmd,
		},
	})
}

// VerifyCmdEnd logs the end of a verification command
func (l *RunLogger) VerifyCmdEnd(cmd string, success bool, output string, durationNs int64) {
	l.logEvent(Event{
		Type:     EventVerifyCmdEnd,
		Duration: &durationNs,
		Success:  &success,
		Data: map[string]interface{}{
			"cmd":    cmd,
			"output": output,
		},
	})
}

// BrowserStart logs the start of browser verification
func (l *RunLogger) BrowserStart() {
	l.logEvent(Event{
		Type: EventBrowserStart,
	})
}

// BrowserEnd logs the end of browser verification
func (l *RunLogger) BrowserEnd(success bool, consoleErrors int) {
	l.logEvent(Event{
		Type:    EventBrowserEnd,
		Success: &success,
		Data: map[string]interface{}{
			"console_errors": consoleErrors,
		},
	})
}

// BrowserStep logs a browser step execution
func (l *RunLogger) BrowserStep(action string, success bool, details map[string]interface{}) {
	l.logEvent(Event{
		Type:    EventBrowserStep,
		Success: &success,
		Data: map[string]interface{}{
			"action":  action,
			"details": details,
		},
	})
}

// ServiceStart logs a service start
func (l *RunLogger) ServiceStart(name, cmd string) {
	l.logEvent(Event{
		Type: EventServiceStart,
		Data: map[string]interface{}{
			"name": name,
			"cmd":  cmd,
		},
	})
}

// ServiceReady logs a service becoming ready
func (l *RunLogger) ServiceReady(name, url string, durationNs int64) {
	l.logEvent(Event{
		Type:     EventServiceReady,
		Duration: &durationNs,
		Data: map[string]interface{}{
			"name": name,
			"url":  url,
		},
	})
}

// ServiceRestart logs a service restart
func (l *RunLogger) ServiceRestart(name string, success bool) {
	l.logEvent(Event{
		Type:    EventServiceRestart,
		Success: &success,
		Data: map[string]interface{}{
			"name": name,
		},
	})
}

// ServiceHealth logs a service health check result
func (l *RunLogger) ServiceHealth(name string, healthy bool, issue string) {
	l.logEvent(Event{
		Type:    EventServiceHealth,
		Success: &healthy,
		Data: map[string]interface{}{
			"name":  name,
			"issue": issue,
		},
	})
}

// StateChange logs a story state change
func (l *RunLogger) StateChange(storyID, from, to string, details map[string]interface{}) {
	data := map[string]interface{}{
		"from": from,
		"to":   to,
	}
	for k, v := range details {
		data[k] = v
	}
	l.logEvent(Event{
		Type:    EventStateChange,
		StoryID: storyID,
		Data:    data,
	})
}

// Learning logs a captured learning
func (l *RunLogger) Learning(text string) {
	l.logEvent(Event{
		Type:    EventLearning,
		Message: text,
	})
}

// Warning logs a warning message
func (l *RunLogger) Warning(msg string) {
	l.logEvent(Event{
		Type:    EventWarning,
		Message: msg,
	})
}

// Error logs an error message
func (l *RunLogger) Error(msg string, err error) {
	data := make(map[string]interface{})
	if err != nil {
		data["error"] = err.Error()
	}
	l.logEvent(Event{
		Type:    EventError,
		Message: msg,
		Data:    data,
	})
}

// Console output helpers with timestamps

// LogPrint prints a timestamped message to stdout
func (l *RunLogger) LogPrint(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if l.config != nil && l.config.ConsoleTimestamps {
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] %s", timestamp, msg)
	} else {
		fmt.Print(msg)
	}
}

// LogPrintln prints a timestamped message with newline to stdout
func (l *RunLogger) LogPrintln(args ...interface{}) {
	msg := fmt.Sprint(args...)
	if l.config != nil && l.config.ConsoleTimestamps {
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] %s\n", timestamp, msg)
	} else {
		fmt.Println(msg)
	}
}

// FormatDuration formats a duration for display
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// Helper functions

// nextRunNumber determines the next run number based on existing logs
func nextRunNumber(logsDir string) int {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return 1
	}

	maxRun := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "run-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		// Extract number from run-XXX.jsonl
		numStr := strings.TrimPrefix(name, "run-")
		numStr = strings.TrimSuffix(numStr, ".jsonl")
		if num, err := strconv.Atoi(numStr); err == nil && num > maxRun {
			maxRun = num
		}
	}

	return maxRun + 1
}

// rotateOldRuns deletes runs beyond maxRuns (keeps most recent)
func rotateOldRuns(logsDir string, maxRuns int) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return
	}

	var runFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "run-") && strings.HasSuffix(name, ".jsonl") {
			runFiles = append(runFiles, name)
		}
	}

	if len(runFiles) <= maxRuns {
		return
	}

	// Sort by run number (ascending)
	sort.Slice(runFiles, func(i, j int) bool {
		numI := extractRunNumber(runFiles[i])
		numJ := extractRunNumber(runFiles[j])
		return numI < numJ
	})

	// Delete oldest files
	toDelete := len(runFiles) - maxRuns
	for i := 0; i < toDelete; i++ {
		os.Remove(filepath.Join(logsDir, runFiles[i]))
	}
}

// extractRunNumber extracts the run number from a filename like "run-001.jsonl"
func extractRunNumber(filename string) int {
	numStr := strings.TrimPrefix(filename, "run-")
	numStr = strings.TrimSuffix(numStr, ".jsonl")
	num, _ := strconv.Atoi(numStr)
	return num
}

// LogsDir returns the path to the logs directory for a feature
func LogsDir(featureDir string) string {
	return filepath.Join(featureDir, "logs")
}

// ListRuns returns all run log files in a feature directory
func ListRuns(featureDir string) ([]RunSummary, error) {
	logsDir := LogsDir(featureDir)
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var runs []RunSummary
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "run-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		runNum := extractRunNumber(name)
		logPath := filepath.Join(logsDir, name)

		info, err := entry.Info()
		if err != nil {
			continue
		}

		summary := RunSummary{
			RunNumber: runNum,
			LogPath:   logPath,
			FileSize:  info.Size(),
			ModTime:   info.ModTime(),
		}

		// Try to read first and last events for summary
		if first, last := readFirstLastEvents(logPath); first != nil {
			summary.StartTime = first.Timestamp
			if last != nil && last.Type == EventRunEnd {
				summary.EndTime = &last.Timestamp
				summary.Success = last.Success
				summary.Summary = last.Message
			}
		}

		runs = append(runs, summary)
	}

	// Sort by run number descending (most recent first)
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].RunNumber > runs[j].RunNumber
	})

	return runs, nil
}

// RunSummary contains summary info about a run
type RunSummary struct {
	RunNumber int
	LogPath   string
	FileSize  int64
	ModTime   time.Time
	StartTime time.Time
	EndTime   *time.Time
	Success   *bool
	Summary   string
}

// readFirstLastEvents reads the first and last events from a log file
func readFirstLastEvents(logPath string) (*Event, *Event) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, nil
	}
	defer file.Close()

	var first, last *Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if first == nil {
			first = &event
		}
		last = &event
	}

	return first, last
}

// ReadEvents reads events from a log file with optional filtering
func ReadEvents(logPath string, filter *EventFilter) ([]Event, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return ReadEventsFromReader(file, filter)
}

// ReadEventsFromReader reads events from an io.Reader with optional filtering
func ReadEventsFromReader(r io.Reader, filter *EventFilter) ([]Event, error) {
	var events []Event
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if filter != nil && !filter.Match(&event) {
			continue
		}

		events = append(events, event)
	}

	return events, scanner.Err()
}

// EventFilter filters events when reading logs
type EventFilter struct {
	EventType EventType
	StoryID   string
	Offset    int
	Limit     int
}

// Match returns true if the event matches the filter
func (f *EventFilter) Match(event *Event) bool {
	if f.EventType != "" && event.Type != f.EventType {
		return false
	}
	if f.StoryID != "" && event.StoryID != f.StoryID {
		return false
	}
	return true
}

// GetRunSummary generates a detailed summary of a run
func GetRunSummary(logPath string) (*DetailedRunSummary, error) {
	events, err := ReadEvents(logPath, nil)
	if err != nil {
		return nil, err
	}

	summary := &DetailedRunSummary{
		Stories: make(map[string]*StorySummary),
	}

	var currentStory string
	storyStarts := make(map[string]time.Time)

	for _, event := range events {
		switch event.Type {
		case EventRunStart:
			summary.StartTime = event.Timestamp
			if data := event.Data; data != nil {
				if f, ok := data["feature"].(string); ok {
					summary.Feature = f
				}
				if b, ok := data["branch"].(string); ok {
					summary.Branch = b
				}
				if n, ok := data["run_number"].(float64); ok {
					summary.RunNumber = int(n)
				}
			}

		case EventRunEnd:
			summary.EndTime = &event.Timestamp
			summary.Success = event.Success
			summary.Result = event.Message

		case EventIterationStart:
			currentStory = event.StoryID
			storyStarts[currentStory] = event.Timestamp

			if _, exists := summary.Stories[currentStory]; !exists {
				summary.Stories[currentStory] = &StorySummary{
					ID: currentStory,
				}
			}
			if event.Data != nil {
				if t, ok := event.Data["title"].(string); ok {
					summary.Stories[currentStory].Title = t
				}
				if r, ok := event.Data["retries"].(float64); ok {
					summary.Stories[currentStory].Retries = int(r)
				}
			}

		case EventIterationEnd:
			if s, exists := summary.Stories[currentStory]; exists {
				if start, ok := storyStarts[currentStory]; ok {
					d := event.Timestamp.Sub(start)
					s.Duration = &d
				}
				s.Success = event.Success
			}

		case EventVerifyCmdEnd:
			if event.Data != nil {
				cmd, _ := event.Data["cmd"].(string)
				output, _ := event.Data["output"].(string)
				var durationNs int64
				if event.Duration != nil {
					durationNs = *event.Duration
				}
				summary.VerifyResults = append(summary.VerifyResults, VerifyResult{
					Command:  cmd,
					Success:  event.Success != nil && *event.Success,
					Duration: time.Duration(durationNs),
					Output:   output,
				})
			}

		case EventLearning:
			summary.Learnings = append(summary.Learnings, event.Message)

		case EventWarning:
			summary.Warnings++

		case EventError:
			summary.Errors++
		}
	}

	// Calculate total duration
	if summary.EndTime != nil {
		d := summary.EndTime.Sub(summary.StartTime)
		summary.Duration = &d
	}

	return summary, nil
}

// DetailedRunSummary contains detailed information about a run
type DetailedRunSummary struct {
	RunNumber     int
	Feature       string
	Branch        string
	StartTime     time.Time
	EndTime       *time.Time
	Duration      *time.Duration
	Success       *bool
	Result        string
	Stories       map[string]*StorySummary
	VerifyResults []VerifyResult
	Learnings     []string
	Warnings      int
	Errors        int
}

// StorySummary contains summary info about a story's execution
type StorySummary struct {
	ID       string
	Title    string
	Duration *time.Duration
	Retries  int
	Success  *bool
}

// VerifyResult contains info about a verification command result
type VerifyResult struct {
	Command  string
	Success  bool
	Duration time.Duration
	Output   string
}
