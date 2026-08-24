package trace

import (
	"encoding/json"
	"testing"

	"github.com/agent-ecosystem/agentminutes/session"
)

func text(s string) []session.ContentBlock {
	return []session.ContentBlock{{Kind: session.ContentText, Text: s}}
}

func userEvent(origin session.Origin, s string) session.Event {
	return session.Event{Kind: session.KindUserMessage, UserMessage: &session.UserMessage{Origin: origin, Content: text(s)}}
}

func systemEvent(subtype, s string) session.Event {
	return session.Event{Kind: session.KindSystem, System: &session.SystemEvent{Subtype: subtype, Text: s}}
}

func sessionOf(events ...session.Event) *session.Session {
	return &session.Session{Events: events}
}

func TestPhraseClassifiesLocations(t *testing.T) {
	echo := map[string]bool{"task_complete": true}
	tests := []struct {
		name   string
		event  session.Event
		loc    Location
		detail string
	}{
		{"harness-injected user message", userEvent(session.OriginHarness, "CANARY"), LocHarnessInjected, ""},
		{"human prompt", userEvent(session.OriginHuman, "CANARY"), LocHumanPrompt, ""},
		{"non-echo system event", systemEvent("attachment/skill_listing", "CANARY"), LocHarnessInjected, "attachment/skill_listing"},
		{"echo system event", systemEvent("task_complete", "CANARY"), LocEcho, "task_complete"},
		{"tool result", session.Event{Kind: session.KindToolResult, ToolResult: &session.ToolResult{ToolCallID: "t1", ToolName: "Read", Content: text("CANARY")}}, LocToolResult, "Read"},
		{"tool call input", session.Event{Kind: session.KindToolCall, ToolCall: &session.ToolCall{ToolCallID: "t1", Name: "Read", Input: json.RawMessage(`{"path":"CANARY"}`)}}, LocModelOutput, "Read"},
		{"assistant message", session.Event{Kind: session.KindAssistantMessage, AssistantMessage: &session.AssistantMessage{Content: text("CANARY")}}, LocModelOutput, ""},
		{"thinking", session.Event{Kind: session.KindThinking, Thinking: &session.Thinking{Text: "CANARY"}}, LocModelOutput, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occs := Phrase(sessionOf(tt.event), "CANARY", echo)
			if len(occs) != 1 {
				t.Fatalf("got %d occurrences, want 1: %+v", len(occs), occs)
			}
			if occs[0].Location != tt.loc || occs[0].Detail != tt.detail {
				t.Errorf("got location %q detail %q, want %q %q", occs[0].Location, occs[0].Detail, tt.loc, tt.detail)
			}
			if occs[0].EventIndex != 0 || occs[0].Kind != tt.event.Kind {
				t.Errorf("got index %d kind %q, want 0 %q", occs[0].EventIndex, occs[0].Kind, tt.event.Kind)
			}
		})
	}
}

// Meta embeds raw source records and unknown events are unclassifiable, so
// neither may ever count as a phrase sighting.
func TestPhraseIgnoresMetaAndUnknown(t *testing.T) {
	s := sessionOf(
		session.Event{Kind: session.KindSessionMeta, SessionMeta: &session.Meta{Harness: "claude-code", SessionID: "CANARY"}},
		session.Event{Kind: session.KindUnknown, Unknown: &session.Unknown{}},
	)
	if occs := Phrase(s, "CANARY", nil); len(occs) != 0 {
		t.Errorf("got %d occurrences in meta/unknown events, want 0: %+v", len(occs), occs)
	}
}

func TestPhraseAbsent(t *testing.T) {
	s := sessionOf(userEvent(session.OriginHuman, "nothing to see"))
	if occs := Phrase(s, "CANARY", nil); occs != nil {
		t.Errorf("got %+v, want nil", occs)
	}
}

func TestPhraseCarriesProvenanceLine(t *testing.T) {
	ev := userEvent(session.OriginHuman, "CANARY")
	ev.Provenance = &session.Provenance{Line: 7}
	occs := Phrase(sessionOf(ev), "CANARY", nil)
	if len(occs) != 1 || occs[0].Line != 7 {
		t.Fatalf("got %+v, want one occurrence at line 7", occs)
	}
}

func TestPhraseMultipleOccurrences(t *testing.T) {
	s := sessionOf(
		userEvent(session.OriginHuman, "say CANARY"),
		session.Event{Kind: session.KindAssistantMessage, AssistantMessage: &session.AssistantMessage{Content: text("CANARY it is")}},
	)
	occs := Phrase(s, "CANARY", nil)
	if len(occs) != 2 {
		t.Fatalf("got %d occurrences, want 2", len(occs))
	}
	if occs[0].EventIndex != 0 || occs[1].EventIndex != 1 {
		t.Errorf("indices = %d, %d, want 0, 1", occs[0].EventIndex, occs[1].EventIndex)
	}
}

func TestAt(t *testing.T) {
	occs := []Occurrence{
		{EventIndex: 0, Location: LocEcho},
		{EventIndex: 1, Location: LocToolResult},
		{EventIndex: 2, Location: LocEcho},
	}
	echoes := At(occs, LocEcho)
	if len(echoes) != 2 || echoes[0].EventIndex != 0 || echoes[1].EventIndex != 2 {
		t.Errorf("got %+v, want the two echo occurrences", echoes)
	}
	if out := At(occs, LocHumanPrompt); out != nil {
		t.Errorf("got %+v, want nil", out)
	}
}

func TestSkillListing(t *testing.T) {
	s := sessionOf(
		systemEvent("turn_duration", "my-skill mentioned in the wrong subtype"),
		systemEvent("attachment/skill_listing", "available skills: other-skill"),
		systemEvent("attachment/skill_listing", "available skills: my-skill"),
		systemEvent("attachment/skill_listing", "available skills: my-skill (again)"),
	)
	subtypes := []string{"attachment/skill_listing"}
	if got := SkillListing(s, subtypes, "my-skill"); got != 2 {
		t.Errorf("got index %d, want 2 (first matching listing)", got)
	}
	if got := SkillListing(s, subtypes, "absent-skill"); got != -1 {
		t.Errorf("got %d, want -1 for absent skill", got)
	}
	if got := SkillListing(s, nil, "my-skill"); got != -1 {
		t.Errorf("got %d, want -1 with no listing subtypes", got)
	}
}

func TestToolReadsOf(t *testing.T) {
	s := sessionOf(
		session.Event{Kind: session.KindToolCall, ToolCall: &session.ToolCall{ToolCallID: "t1", Name: "Read", Input: json.RawMessage(`{"path":"/p/.claude/skills/my-skill/SKILL.md"}`)}},
		session.Event{Kind: session.KindToolCall, ToolCall: &session.ToolCall{ToolCallID: "t2", Name: "Read", Input: json.RawMessage(`{"path":"/p/README.md"}`)}},
		// A tool RESULT mentioning the path is not a model-initiated read.
		session.Event{Kind: session.KindToolResult, ToolResult: &session.ToolResult{ToolCallID: "t1", ToolName: "Read", Content: text("contents of my-skill/SKILL.md")}},
		session.Event{Kind: session.KindToolCall, ToolCall: &session.ToolCall{ToolCallID: "t3", Name: "Bash", Input: json.RawMessage(`{"command":"cat my-skill/reference.md"}`)}},
	)
	got := ToolReadsOf(s, "my-skill")
	if len(got) != 2 || got[0] != 0 || got[1] != 3 {
		t.Errorf("got %v, want [0 3]", got)
	}
	if got := ToolReadsOf(s, "no-such-path"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestModel(t *testing.T) {
	s := sessionOf(
		userEvent(session.OriginHuman, "hi"),
		session.Event{Kind: session.KindAssistantMessage, AssistantMessage: &session.AssistantMessage{Content: text("no model recorded")}},
		session.Event{Kind: session.KindAssistantMessage, AssistantMessage: &session.AssistantMessage{Model: "claude-sonnet-5", Content: text("hello")}},
		session.Event{Kind: session.KindAssistantMessage, AssistantMessage: &session.AssistantMessage{Model: "claude-haiku-4-5", Content: text("later")}},
	)
	if got := Model(s); got != "claude-sonnet-5" {
		t.Errorf("got %q, want first non-empty model", got)
	}
	if got := Model(sessionOf(userEvent(session.OriginHuman, "hi"))); got != "" {
		t.Errorf("got %q, want empty for session with no assistant messages", got)
	}
}
