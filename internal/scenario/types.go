// Package scenario provides UI scenario testing for KALUA run-mode apps.
// Scenarios are JSON files describing a sequence of user interactions and assertions.
package scenario

import (
	"encoding/json"
	"time"
)

// ScenarioStepType represents the type of a scenario step.
type ScenarioStepType string

const (
	StepSetControl   ScenarioStepType = "set_control"
	StepClick        ScenarioStepType = "click"
	StepAssertValue  ScenarioStepType = "assert_value"
	StepAssertOutbox ScenarioStepType = "assert_outbox"
	StepAssertMsgbox ScenarioStepType = "assert_msgbox"
	StepWait         ScenarioStepType = "wait"
	StepTimer        ScenarioStepType = "timer"
	StepGetGlobal    ScenarioStepType = "get_global"
	StepAssertGlobal ScenarioStepType = "assert_global"
)

// ScenarioStep is a single step in a scenario.
type ScenarioStep struct {
	Type        ScenarioStepType `json:"type"`
	Form        string           `json:"form,omitempty"`        // form name
	Control     string           `json:"control,omitempty"`     // control name
	Value       interface{}      `json:"value,omitempty"`       // value to set
	Event       string           `json:"event,omitempty"`       // event name (default: "click")
	Expected    interface{}      `json:"expected,omitempty"`    // expected value
	OutboxType  string           `json:"outbox_type,omitempty"` // expected outbox type
	OutboxForm  string           `json:"outbox_form,omitempty"` // expected outbox form
	OutboxCtrl  string           `json:"outbox_ctrl,omitempty"` // expected outbox control
	OutboxCount int              `json:"outbox_count,omitempty"` // expected count
	Timeout     int              `json:"timeout,omitempty"`     // timeout in ms
	Global      string           `json:"global,omitempty"`      // global variable name
	Message     string           `json:"message,omitempty"`     // custom error message
	MsgboxID    string           `json:"msgbox_id,omitempty"`   // msgbox ID to answer
	MsgboxValue string           `json:"msgbox_value,omitempty"` // value to send for msgbox
	MsgboxChoice string          `json:"msgbox_choice,omitempty"` // choice to send for msgbox
}

// Scenario is a complete UI scenario test.
type Scenario struct {
	Name   string          `json:"name"`
	Steps  []ScenarioStep  `json:"steps"`
	Setup  string          `json:"setup,omitempty"`   // Lua code to run before scenario
	Teardown string         `json:"teardown,omitempty"` // Lua code to run after scenario
}

// ScenarioResult holds the result of running a scenario.
type ScenarioResult struct {
	OK        bool          `json:"ok"`
	Name      string        `json:"name"`
	Step      int           `json:"step"`       // 0-based step index where failure occurred
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Output    string        `json:"output,omitempty"`   // captured k.print output
	Errors    []string      `json:"errors,omitempty"`   // captured error output
}

// UnmarshalScenario loads a scenario from JSON bytes.
func UnmarshalScenario(data []byte) (*Scenario, error) {
	var s Scenario
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// MarshalScenario marshals a scenario to JSON bytes.
func MarshalScenario(s *Scenario) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// DefaultTimeout is the default step timeout in milliseconds.
const DefaultTimeout = 5000