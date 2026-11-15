// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package moduletelemetry

// Action is the module-instance lifecycle classification for one apply.
type Action string

const (
	ActionCreated Action = "created"
	ActionUpdated Action = "updated"
	ActionDeleted Action = "deleted"
)

// InstanceRecord is the metadata persisted (in state, in the proposal) for a
// logical module instance. The InstanceID is stable across plans and applies.
type InstanceRecord struct {
	InstanceID      string
	Source          string
	Version         string
	Endpoint        string
	LastAppliedData map[string]string
}

// Classify determines the lifecycle action for a successful apply.
//
//   - prev == nil and the instance is present: created (first successful apply).
//   - prev != nil and the instance is present: updated.
//   - prev != nil and the instance was removed by this apply: deleted.
//   - prev == nil and the instance was removed: no event. Core cannot identify
//     the module without prior metadata, so the proposal says to omit it.
//
// It returns ok=false when no event should be emitted.
func Classify(prev *InstanceRecord, removed bool) (Action, bool) {
	switch {
	case removed && prev == nil:
		return "", false
	case removed:
		return ActionDeleted, true
	case prev == nil:
		return ActionCreated, true
	default:
		return ActionUpdated, true
	}
}
