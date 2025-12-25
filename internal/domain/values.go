package domain

// TaskStatus represents the current state of a task.
// Value object - immutable string enum.
type TaskStatus string

const (
	TaskStatusTodo       TaskStatus = "todo"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusDone       TaskStatus = "done"
	TaskStatusArchived   TaskStatus = "archived"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// UndoneStatuses returns the list of statuses that are considered "not done".
// Business rule: tasks that still require action (not completed, archived, or cancelled).
func UndoneStatuses() []TaskStatus {
	return []TaskStatus{TaskStatusTodo, TaskStatusInProgress, TaskStatusBlocked}
}

// DefaultExcludedStatuses returns statuses excluded from item listings by default.
// Business rule: archived and cancelled items are hidden unless explicitly requested.
// This allows users to focus on active work while still being able to retrieve
// historical data when needed via explicit status filter.
func DefaultExcludedStatuses() []TaskStatus {
	return []TaskStatus{TaskStatusArchived, TaskStatusCancelled}
}

// TaskPriority represents the priority level of a task.
// Value object - immutable string enum.
type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "low"
	TaskPriorityMedium TaskPriority = "medium"
	TaskPriorityHigh   TaskPriority = "high"
	TaskPriorityUrgent TaskPriority = "urgent"
)

// RecurrencePattern represents the type of recurrence for recurring tasks.
// Value object - immutable string enum.
type RecurrencePattern string

const (
	RecurrenceDaily     RecurrencePattern = "daily"
	RecurrenceWeekly    RecurrencePattern = "weekly"
	RecurrenceBiweekly  RecurrencePattern = "biweekly"
	RecurrenceMonthly   RecurrencePattern = "monthly"
	RecurrenceYearly    RecurrencePattern = "yearly"
	RecurrenceQuarterly RecurrencePattern = "quarterly"
	RecurrenceWeekdays  RecurrencePattern = "weekdays"
)
