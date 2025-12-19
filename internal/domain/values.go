package domain

// TaskStatus represents the current state of a task.
// Value object - immutable string enum.
type TaskStatus string

const (
	TaskStatusTodo       TaskStatus = "TODO"
	TaskStatusInProgress TaskStatus = "IN_PROGRESS"
	TaskStatusBlocked    TaskStatus = "BLOCKED"
	TaskStatusDone       TaskStatus = "DONE"
	TaskStatusArchived   TaskStatus = "ARCHIVED"
	TaskStatusCancelled  TaskStatus = "CANCELLED"
)

// TaskPriority represents the priority level of a task.
// Value object - immutable string enum.
type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "LOW"
	TaskPriorityMedium TaskPriority = "MEDIUM"
	TaskPriorityHigh   TaskPriority = "HIGH"
	TaskPriorityUrgent TaskPriority = "URGENT"
)

// RecurrencePattern represents the type of recurrence for recurring tasks.
// Value object - immutable string enum.
type RecurrencePattern string

const (
	RecurrenceDaily     RecurrencePattern = "DAILY"
	RecurrenceWeekly    RecurrencePattern = "WEEKLY"
	RecurrenceBiweekly  RecurrencePattern = "BIWEEKLY"
	RecurrenceMonthly   RecurrencePattern = "MONTHLY"
	RecurrenceYearly    RecurrencePattern = "YEARLY"
	RecurrenceQuarterly RecurrencePattern = "QUARTERLY"
	RecurrenceWeekdays  RecurrencePattern = "WEEKDAYS"
)
