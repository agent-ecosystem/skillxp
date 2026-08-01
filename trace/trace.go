// Package trace answers how content moved through a normalized session:
// given any phrase, it finds every occurrence in model-visible text and
// classifies how it got there. The discipline comes from the spike that
// shaped this runner: only model-visible text counts (never raw provenance
// or harness sidecar enrichment), echo locations that replay conversation
// content are separated from genuine loading evidence, and the model
// reading a skill's own files is a distinct signal from the harness
// injecting them.
package trace

import (
	"strings"

	"github.com/agent-ecosystem/agentminutes/session"
)

// Location classifies where in a session a traced phrase was found.
type Location string

// Location values.
const (
	// LocHarnessInjected is content the harness placed in front of the
	// model: a harness-origin user message or a non-echo system event.
	// This is the harness-push loading signal.
	LocHarnessInjected Location = "harness-injected"

	// LocToolResult is content the model retrieved through a tool call.
	// Together with the call's input path this is the model-pull signal.
	LocToolResult Location = "tool-result"

	// LocHumanPrompt is the run's own prompt. Benchmark prompts are
	// designed not to contain canaries, so this normally flags a check
	// design bug.
	LocHumanPrompt Location = "human-prompt"

	// LocModelOutput is text the model produced (assistant message,
	// thinking, or tool-call input). The model knowing a phrase it was
	// never visibly given is itself a finding.
	LocModelOutput Location = "model-output"

	// LocEcho is a system event that merely replays conversation content
	// (per the harness profile's echo list). Never loading evidence.
	LocEcho Location = "echo"
)

// Occurrence is one sighting of a traced phrase.
type Occurrence struct {
	EventIndex int               `json:"event_index"`
	Kind       session.EventKind `json:"kind"`
	Location   Location          `json:"location"`
	// Detail carries the system subtype or tool name, when relevant.
	Detail string `json:"detail,omitempty"`
	// Line is the native transcript line, for citable evidence.
	Line int `json:"line,omitempty"`
}

// Phrase reports every occurrence of phrase in the session's
// model-visible text, classified by location. echoSubtypes marks system
// subtypes that replay conversation content for this harness.
func Phrase(s *session.Session, phrase string, echoSubtypes map[string]bool) []Occurrence {
	var occs []Occurrence
	for i := range s.Events {
		ev := &s.Events[i]
		text, loc, detail := visibleText(ev, echoSubtypes)
		if text == "" || !strings.Contains(text, phrase) {
			continue
		}
		occ := Occurrence{EventIndex: i, Kind: ev.Kind, Location: loc, Detail: detail}
		if ev.Provenance != nil {
			occ.Line = ev.Provenance.Line
		}
		occs = append(occs, occ)
	}
	return occs
}

// visibleText extracts the model-visible text of one event and its
// evidence classification. session_meta and unknown events yield nothing:
// meta embeds raw source records, and unknowns are unclassifiable by
// definition.
func visibleText(ev *session.Event, echoSubtypes map[string]bool) (string, Location, string) {
	switch ev.Kind {
	case session.KindUserMessage:
		if ev.UserMessage.Origin == session.OriginHarness {
			return ev.UserMessage.Text(), LocHarnessInjected, ""
		}
		return ev.UserMessage.Text(), LocHumanPrompt, ""
	case session.KindSystem:
		if echoSubtypes[ev.System.Subtype] {
			return ev.System.Text, LocEcho, ev.System.Subtype
		}
		return ev.System.Text, LocHarnessInjected, ev.System.Subtype
	case session.KindToolResult:
		return ev.ToolResult.Text(), LocToolResult, ev.ToolResult.ToolName
	case session.KindToolCall:
		return string(ev.ToolCall.Input), LocModelOutput, ev.ToolCall.Name
	case session.KindAssistantMessage:
		return ev.AssistantMessage.Text(), LocModelOutput, ""
	case session.KindThinking:
		return ev.Thinking.Text, LocModelOutput, ""
	}
	return "", "", ""
}

// At filters occurrences to one location.
func At(occs []Occurrence, loc Location) []Occurrence {
	var out []Occurrence
	for _, o := range occs {
		if o.Location == loc {
			out = append(out, o)
		}
	}
	return out
}

// SkillListing looks for the harness's skill discovery listing: a system
// event with one of the given subtypes whose text mentions the skill name.
// It returns the event index, or -1.
func SkillListing(s *session.Session, subtypes []string, skillName string) int {
	want := make(map[string]bool, len(subtypes))
	for _, st := range subtypes {
		want[st] = true
	}
	for i := range s.Events {
		ev := &s.Events[i]
		if ev.Kind != session.KindSystem || !want[ev.System.Subtype] {
			continue
		}
		if strings.Contains(ev.System.Text, skillName) {
			return i
		}
	}
	return -1
}

// ToolReadsOf returns the indices of tool calls whose input references the
// given path fragment (e.g. a skill directory name or SKILL.md path). This
// is the model-pull detector and, for harnesses whose transcripts omit
// injected context, the direct-path-navigation signal that a discovery
// listing existed.
func ToolReadsOf(s *session.Session, pathFragment string) []int {
	var out []int
	for i := range s.Events {
		ev := &s.Events[i]
		if ev.Kind != session.KindToolCall {
			continue
		}
		if strings.Contains(string(ev.ToolCall.Input), pathFragment) {
			out = append(out, i)
		}
	}
	return out
}

// Model returns the first model observed on an assistant message.
func Model(s *session.Session) string {
	for i := range s.Events {
		if s.Events[i].Kind == session.KindAssistantMessage {
			if m := s.Events[i].AssistantMessage.Model; m != "" {
				return m
			}
		}
	}
	return ""
}
