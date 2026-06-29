package harness

// EventType identifies what operational decision occurred during a runtime step.
type EventType string

const (
	EventAgentStarted       EventType = "agent_started"
	EventPlanCreated        EventType = "plan_created"
	EventToolProposed       EventType = "tool_proposed"
	EventPermissionDecision EventType = "permission_decision"
	EventToolResult         EventType = "tool_result"
	EventValidationFailure  EventType = "validation_failure"
	EventBudgetExceeded     EventType = "budget_exceeded"
	EventAgentCompleted     EventType = "agent_completed"
)

// Event is an inspectable record of one operational decision. It does not expose
// hidden chain-of-thought; visible plans from ModelOutput are acceptable because
// they are explicit model output.
type Event struct {
	Type EventType
	Data map[string]any
}
