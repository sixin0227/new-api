package model

import (
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// Body summaries are extracted once at write time so the body browser can
// list records without ever loading the full (potentially megabyte-sized)
// bodies. They are display-oriented: short, plain text, never used in
// billing or audit decisions.

const bodySummaryTextLimit = 300

// BodySummaries is the display summary of one recorded exchange.
type BodySummaries struct {
	Model           string
	RequestPreview  string
	ResponsePreview string
}

func SummarizeBodyRecord(requestBody string, responseBody string) BodySummaries {
	return BodySummaries{
		Model:           summarizeModel(requestBody),
		RequestPreview:  summarizeRequestPreview(requestBody),
		ResponsePreview: summarizeResponsePreview(responseBody),
	}
}

func summarizeModel(requestBody string) string {
	if requestBody == "" {
		return ""
	}
	return strings.TrimSpace(gjson.Get(requestBody, "model").String())
}

// summarizeRequestPreview shows what the caller asked for: the last user
// message when the payload is a chat request, otherwise the input text, the
// prompt, or a plain prefix of the raw JSON.
func summarizeRequestPreview(requestBody string) string {
	if requestBody == "" {
		return ""
	}
	if preview := lastUserMessageText(requestBody); preview != "" {
		return truncateBodySummary(preview)
	}
	if text := inputTextPreview(requestBody); text != "" {
		return truncateBodySummary(text)
	}
	return truncateBodySummary(requestBody)
}

func lastUserMessageText(requestBody string) string {
	messages := gjson.Get(requestBody, "messages")
	if !messages.IsArray() {
		return ""
	}
	var preview string
	for _, message := range messages.Array() {
		if message.Get("role").String() != "user" {
			continue
		}
		preview = messageContentText(message.Get("content"))
	}
	return preview
}

func messageContentText(content gjson.Result) string {
	if content.Type == gjson.String {
		return content.String()
	}
	if !content.IsArray() {
		return ""
	}
	for _, block := range content.Array() {
		if block.Get("type").String() == "text" {
			return block.Get("text").String()
		}
	}
	return ""
}

func inputTextPreview(requestBody string) string {
	input := gjson.Get(requestBody, "input")
	if input.Type == gjson.String {
		return input.String()
	}
	if input.IsArray() {
		for _, item := range input.Array() {
			if text := messageContentText(item.Get("content")); text != "" {
				return text
			}
		}
		return ""
	}
	return gjson.Get(requestBody, "prompt").String()
}

// summarizeResponsePreview shows what the upstream answered: the assistant
// text of a chat/completion response (OpenAI or Claude shape), otherwise a
// plain prefix of the raw payload (error responses keep their message text
// visible this way).
func summarizeResponsePreview(responseBody string) string {
	if responseBody == "" {
		return ""
	}
	if text := gjson.Get(responseBody, "choices.0.message.content").String(); text != "" {
		return truncateBodySummary(text)
	}
	if text := gjson.Get(responseBody, "content.0.text").String(); text != "" {
		return truncateBodySummary(text)
	}
	return truncateBodySummary(responseBody)
}

func truncateBodySummary(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= bodySummaryTextLimit {
		return text
	}
	trimmed := text[:bodySummaryTextLimit]
	// Back off to the last rune boundary so the preview never stores a broken
	// character.
	for !utf8.ValidString(trimmed) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed + "…"
}
