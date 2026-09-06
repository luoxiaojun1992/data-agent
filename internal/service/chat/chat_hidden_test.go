package chat

import (
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
)

func TestIsHiddenEvent(t *testing.T) {
	hidden := &session.Event{LLMResponse: model.LLMResponse{
		CustomMetadata: map[string]any{"hidden": true},
	}}
	explicitFalse := &session.Event{LLMResponse: model.LLMResponse{
		CustomMetadata: map[string]any{"hidden": false},
	}}
	otherKey := &session.Event{LLMResponse: model.LLMResponse{
		CustomMetadata: map[string]any{"compaction": true},
	}}
	noMeta := &session.Event{}

	if !IsHiddenEvent(hidden) {
		t.Errorf("IsHiddenEvent(hidden:true) = false, want true")
	}
	if IsHiddenEvent(explicitFalse) {
		t.Errorf("IsHiddenEvent(hidden:false) = true, want false")
	}
	if IsHiddenEvent(otherKey) {
		t.Errorf("IsHiddenEvent(other key) = true, want false")
	}
	if IsHiddenEvent(noMeta) {
		t.Errorf("IsHiddenEvent(no metadata) = true, want false")
	}
	if IsHiddenEvent(nil) {
		t.Errorf("IsHiddenEvent(nil) = true, want false")
	}
}
