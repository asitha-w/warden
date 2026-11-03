package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/asitha-w/warden/internal/domain"
	"github.com/asitha-w/warden/internal/store"
)

// Claude is the model-backed reasoner: a streaming manual tool loop so the
// console sees text as it is written and the approval gate can pause a run.
type Claude struct {
	client anthropic.Client
	model  string
	st     *store.Store
	log    *slog.Logger
}

// NewClaude returns a reasoner on the Anthropic API. The key comes from the environment.
func NewClaude(st *store.Store, model string, log *slog.Logger, opts ...option.RequestOption) *Claude {
	return &Claude{client: anthropic.NewClient(opts...), model: model, st: st, log: log}
}

// Name implements Reasoner.
func (c *Claude) Name() string { return "claude" }

const investigatorSystem = `You are Warden's investigator, a reliability operations agent working one finding in a customer tenant.

Work like a careful SRE. Read before you reason: metrics, logs, the activity log, resource state and files are all available through tools. Record every material claim with record_evidence and its source pointer before you rely on it. Record at least one alternative cause you rejected with record_hypothesis. Then propose exactly one remediation with propose_action; policy decides whether it runs, waits for approval, or is refused, and the tool result tells you which. If an action is denied, do not propose it again. Finish with conclude.

Write plainly for an operator: short sentences, no hedging, cite pointers. Keep text between tool calls to one or two sentences. Never invent facts that a tool did not return.`

func toolParams(defs []ToolDef) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		t := anthropic.ToolParam{
			Name:        d.Name,
			Description: anthropic.String(d.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: d.Schema, Required: d.Required},
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &t})
	}
	return out
}

// Investigate implements Reasoner.
func (c *Claude) Investigate(ctx context.Context, run *domain.Run, tb *Toolbox) error {
	var responder, finding, runbooks string
	c.st.View(func(s *domain.State) {
		runbooks = runbookText(s, run)
		if r := s.Responder(run.ResponderID); r != nil {
			var pol []string
			for _, tp := range r.Tools {
				pol = append(pol, tp.Tool+"="+tp.Policy)
			}
			responder = fmt.Sprintf("Responder %s (%s, %s): signal %q, scope %s, policy %s.", r.ID, r.Mode, r.Level, r.Signal, r.Scope, strings.Join(pol, ", "))
		}
		if f := s.Finding(run.FindingID); f != nil {
			finding = fmt.Sprintf("Finding %s: %s. Resource %s, service %s, severity %s.", f.ID, f.Title, f.Resource, f.Service, f.Severity)
		}
	})
	task := fmt.Sprintf("%s\n%s\nTask: %s\nInvestigate, record evidence and hypotheses, propose one action, then conclude.", finding, responder, run.Task)
	if runbooks != "" {
		task += "\n\nThe tenant wrote these runbooks for this resource. Follow their fix order and their Never rules, and say which runbook step you are following when you propose an action.\n" + runbooks
	}
	_, err := c.loop(ctx, loopRequest{
		system:  investigatorSystem,
		effort:  effortOf(c.st, run.Agent),
		tools:   InvestigatorTools(),
		call:    tb.Call,
		runID:   run.ID,
		agentID: run.Agent,
		say:     func() string { return tb.Say(domain.RoleAgent, "", "") },
		append:  tb.Append,
		max:     tb.maxCalls,
	}, []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(task))})
	return err
}

func effortOf(st *store.Store, agentID string) anthropic.OutputConfigEffort {
	e := "medium"
	st.View(func(s *domain.State) {
		if a := s.Agent(agentID); a != nil && a.Effort != "" {
			e = a.Effort
		}
	})
	return anthropic.OutputConfigEffort(e)
}

type loopRequest struct {
	system  string
	effort  anthropic.OutputConfigEffort
	tools   []ToolDef
	call    func(ctx context.Context, name string, in map[string]any) (string, error)
	runID   string
	agentID string
	say     func() string          // opens a new streamed message, returns its id
	append  func(id, delta string) // appends streamed text
	max     int
}

// loop is the manual tool loop. It returns the final text.
func (c *Claude) loop(ctx context.Context, req loopRequest, messages []anthropic.MessageParam) (string, error) {
	tools := toolParams(req.tools)
	final := ""
	for turn := 0; turn < req.max+2; turn++ {
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(c.model),
			MaxTokens: 16000,
			System:    []anthropic.TextBlockParam{{Text: req.system, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
			Messages:  messages,
			Tools:     tools,
		}
		if req.effort != "" {
			params.OutputConfig = anthropic.OutputConfigParam{Effort: req.effort}
		}
		stream := c.client.Messages.NewStreaming(ctx, params)
		msg := anthropic.Message{}
		msgID := ""
		for stream.Next() {
			ev := stream.Current()
			if err := msg.Accumulate(ev); err != nil {
				c.log.Warn("accumulate", "err", err)
			}
			if d, ok := ev.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				if td, ok := d.Delta.AsAny().(anthropic.TextDelta); ok && td.Text != "" {
					if msgID == "" && req.say != nil {
						msgID = req.say()
					}
					if req.append != nil {
						req.append(msgID, td.Text)
					}
				}
			}
		}
		if err := stream.Err(); err != nil {
			var apierr *anthropic.Error
			if errors.As(err, &apierr) {
				return "", fmt.Errorf("claude api %d: %s", apierr.StatusCode, firstLine(apierr.Error()))
			}
			return "", err
		}
		AddUsage(c.st, req.runID, req.agentID, c.model, int(msg.Usage.InputTokens+msg.Usage.CacheReadInputTokens+msg.Usage.CacheCreationInputTokens), int(msg.Usage.OutputTokens))
		messages = append(messages, msg.ToParam())
		var results []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			switch v := block.AsAny().(type) {
			case anthropic.TextBlock:
				final = v.Text
			case anthropic.ToolUseBlock:
				in := map[string]any{}
				if err := json.Unmarshal([]byte(v.JSON.Input.Raw()), &in); err != nil {
					results = append(results, anthropic.NewToolResultBlock(v.ID, "invalid tool input JSON: "+err.Error(), true))
					continue
				}
				out, err := req.call(ctx, v.Name, in)
				if err != nil {
					return final, err
				}
				results = append(results, anthropic.NewToolResultBlock(v.ID, out, false))
			}
		}
		switch msg.StopReason {
		case anthropic.StopReasonToolUse:
			messages = append(messages, anthropic.NewUserMessage(results...))
			continue
		case anthropic.StopReasonRefusal:
			return final, fmt.Errorf("the model declined this task (%s); escalate to a human", msg.StopDetails.Category)
		case anthropic.StopReasonMaxTokens:
			return final, errors.New("the model hit the output limit")
		}
		return final, nil
	}
	return final, errors.New("turn budget exhausted")
}

// Chat implements Reasoner.
func (c *Claude) Chat(ctx context.Context, req ChatRequest, onDelta func(string)) (string, error) {
	messages := []anthropic.MessageParam{}
	for _, h := range req.History {
		if h.Text == "" {
			continue
		}
		if h.Role == "user" {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(h.Text)))
		} else if h.Role == "orchestrator" {
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(h.Text)))
		}
	}
	q := req.Question
	if req.Context != "" {
		q = "Current context:\n" + req.Context + "\n\nQuestion: " + req.Question
	}
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(q)))
	call := req.Call
	if call == nil {
		call = func(ctx context.Context, name string, in map[string]any) (string, error) {
			return "no tools available", nil
		}
	}
	collected := ""
	return c.loop(ctx, loopRequest{
		system:  req.System,
		effort:  effortOf(c.st, req.AgentID),
		tools:   req.Tools,
		call:    call,
		runID:   req.RunID,
		agentID: req.AgentID,
		say:     func() string { collected = ""; return "chat" },
		append: func(_ string, d string) {
			collected += d
			if onDelta != nil {
				onDelta(d)
			}
		},
		max: max(req.MaxCalls, 1),
	}, messages)
}
