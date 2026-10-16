package common

import (
	"strings"
)

// ssePayloads splits raw text/event-stream bytes into the JSON payloads of
// their "data:" lines, dropping pings, comments and the [DONE] sentinel.
func ssePayloads(raw string) []string {
	var payloads []string
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

type openAIStreamChunk struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content          *string `json:"content"`
			ReasoningContent *string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    *int   `json:"index"`
				Id       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage map[string]any `json:"usage"`
}

// reassembleOpenAIStream merges streamed chat-completion chunks into one
// complete chat.completion object: delta content and tool-call arguments are
// concatenated per choice, usage is taken from the last chunk that carries it.
func reassembleOpenAIStream(raw string) string {
	type toolCall struct {
		Id        string
		Name      string
		Arguments strings.Builder
	}
	type choiceState struct {
		Index        int
		Content      strings.Builder
		Reasoning    strings.Builder
		FinishReason string
		ToolCalls    map[int]*toolCall
	}

	var (
		id       string
		created  int64
		model    string
		usage    map[string]any
		choices  = map[int]*choiceState{}
		order    []int
		finished = map[int]string{}
	)

	for _, payload := range ssePayloads(raw) {
		var chunk openAIStreamChunk
		if err := UnmarshalJsonStr(payload, &chunk); err != nil {
			continue
		}
		if id == "" && chunk.Id != "" {
			id = chunk.Id
		}
		if created == 0 && chunk.Created != 0 {
			created = chunk.Created
		}
		if model == "" && chunk.Model != "" {
			model = chunk.Model
		}
		if len(chunk.Usage) > 0 {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			state, ok := choices[choice.Index]
			if !ok {
				state = &choiceState{Index: choice.Index, ToolCalls: map[int]*toolCall{}}
				choices[choice.Index] = state
				order = append(order, choice.Index)
			}
			if choice.Delta.Content != nil {
				state.Content.WriteString(*choice.Delta.Content)
			}
			if choice.Delta.ReasoningContent != nil {
				state.Reasoning.WriteString(*choice.Delta.ReasoningContent)
			}
			for _, call := range choice.Delta.ToolCalls {
				idx := 0
				if call.Index != nil {
					idx = *call.Index
				}
				call2, ok := state.ToolCalls[idx]
				if !ok {
					call2 = &toolCall{Id: call.Id, Name: call.Function.Name}
					state.ToolCalls[idx] = call2
				}
				if call2.Id == "" {
					call2.Id = call.Id
				}
				if call2.Name == "" {
					call2.Name = call.Function.Name
				}
				call2.Arguments.WriteString(call.Function.Arguments)
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finished[choice.Index] = *choice.FinishReason
			}
		}
	}

	out := map[string]any{
		"object":  "chat.completion",
		"choices": []any{},
	}
	if id != "" {
		out["id"] = id
	}
	if created != 0 {
		out["created"] = created
	}
	if model != "" {
		out["model"] = model
	}
	choiceList := make([]any, 0, len(order))
	for _, idx := range order {
		state := choices[idx]
		message := map[string]any{"role": "assistant"}
		if state.Content.Len() > 0 {
			message["content"] = state.Content.String()
		} else {
			message["content"] = ""
		}
		if state.Reasoning.Len() > 0 {
			message["reasoning_content"] = state.Reasoning.String()
		}
		if len(state.ToolCalls) > 0 {
			calls := make([]any, 0, len(state.ToolCalls))
			for i := range len(state.ToolCalls) {
				if call, ok := state.ToolCalls[i]; ok {
					calls = append(calls, map[string]any{
						"index": i,
						"id":    call.Id,
						"type":  "function",
						"function": map[string]any{
							"name":      call.Name,
							"arguments": call.Arguments.String(),
						},
					})
				}
			}
			message["tool_calls"] = calls
		}
		choice := map[string]any{"index": idx, "message": message}
		if reason, ok := finished[idx]; ok {
			choice["finish_reason"] = reason
		}
		choiceList = append(choiceList, choice)
	}
	out["choices"] = choiceList
	if usage != nil {
		out["usage"] = usage
	}
	body, err := Marshal(out)
	if err != nil {
		return raw
	}
	return string(body)
}

type claudeStreamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Id      string           `json:"id"`
		Model   string           `json:"model"`
		Role    string           `json:"role"`
		Content []map[string]any `json:"content"`
		Usage   map[string]any   `json:"usage"`
	} `json:"message"`
	Index        int `json:"index"`
	ContentBlock struct {
		Type  string `json:"type"`
		Id    string `json:"id"`
		Name  string `json:"name"`
		Input string `json:"input"`
		Text  string `json:"text"`
	} `json:"content_block"`
	Delta struct {
		Type        string         `json:"type"`
		Text        string         `json:"text"`
		PartialJson string         `json:"partial_json"`
		StopReason  string         `json:"stop_reason"`
		Usage       map[string]any `json:"usage"`
	} `json:"delta"`
}

// reassembleClaudeStream merges streamed messages events into one complete
// Anthropic messages object: text and tool-input deltas are concatenated into
// their content blocks, usage and stop_reason come from the final events.
func reassembleClaudeStream(raw string) string {
	type blockState struct {
		blockType string
		id        string
		name      string
		text      strings.Builder
		input     strings.Builder
	}

	var (
		id         string
		model      string
		role       = "assistant"
		usage      map[string]any
		stopReason string
		blocks     = map[int]*blockState{}
		order      []int
	)

	for _, payload := range ssePayloads(raw) {
		var event claudeStreamEvent
		if err := UnmarshalJsonStr(payload, &event); err != nil {
			continue
		}
		switch event.Type {
		case "message_start":
			if id == "" {
				id = event.Message.Id
			}
			if model == "" {
				model = event.Message.Model
			}
			if event.Message.Role != "" {
				role = event.Message.Role
			}
			if len(event.Message.Usage) > 0 {
				usage = event.Message.Usage
			}
			for i, block := range event.Message.Content {
				state := &blockState{blockType: stringAny(block["type"]), id: stringAny(block["id"]), name: stringAny(block["name"])}
				state.text.WriteString(stringAny(block["text"]))
				blocks[i] = state
				order = append(order, i)
			}
		case "content_block_start":
			if _, ok := blocks[event.Index]; !ok {
				blocks[event.Index] = &blockState{blockType: event.ContentBlock.Type, id: event.ContentBlock.Id, name: event.ContentBlock.Name}
				order = append(order, event.Index)
			}
		case "content_block_delta":
			state, ok := blocks[event.Index]
			if !ok {
				continue
			}
			switch event.Delta.Type {
			case "text_delta":
				state.text.WriteString(event.Delta.Text)
			case "input_json_delta":
				state.input.WriteString(event.Delta.PartialJson)
			}
		case "message_delta":
			if event.Delta.StopReason != "" {
				stopReason = event.Delta.StopReason
			}
			if len(event.Delta.Usage) > 0 {
				usage = event.Delta.Usage
			}
		}
	}

	content := make([]any, 0, len(order))
	for _, idx := range order {
		state := blocks[idx]
		switch state.blockType {
		case "tool_use":
			content = append(content, map[string]any{
				"type":  "tool_use",
				"id":    state.id,
				"name":  state.name,
				"input": state.input.String(),
			})
		case "thinking":
			content = append(content, map[string]any{
				"type":     "thinking",
				"thinking": state.text.String(),
			})
		default:
			content = append(content, map[string]any{
				"type": "text",
				"text": state.text.String(),
			})
		}
	}

	out := map[string]any{
		"type":    "message",
		"role":    role,
		"content": content,
	}
	if id != "" {
		out["id"] = id
	}
	if model != "" {
		out["model"] = model
	}
	if stopReason != "" {
		out["stop_reason"] = stopReason
	}
	if usage != nil {
		out["usage"] = usage
	}
	body, err := Marshal(out)
	if err != nil {
		return raw
	}
	return string(body)
}

func stringAny(v any) string {
	s, _ := v.(string)
	return s
}
