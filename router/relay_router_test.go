package router

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListModelsSupportsOpenAIAndGeminiAuthentication(t *testing.T) {
	setupRelayRouterTestDB(t)

	user := model.User{
		Username: "models-user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "modelstestkey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	engine := gin.New()
	SetRelayRouter(engine)

	tests := []struct {
		name           string
		path           string
		headerName     string
		expectedObject string
		expectedField  string
	}{
		{
			name:           "OpenAI bearer token",
			path:           "/v1/models",
			headerName:     "Authorization",
			expectedObject: "list",
			expectedField:  "data",
		},
		{
			name:          "Gemini API key header",
			path:          "/v1/models",
			headerName:    "x-goog-api-key",
			expectedField: "models",
		},
		{
			name:          "Gemini API key query",
			path:          "/v1/models?key=modelstestkey",
			expectedField: "models",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.headerName != "" {
				value := "modelstestkey"
				if test.headerName == "Authorization" {
					value = "Bearer " + value
				}
				request.Header.Set(test.headerName, value)
			}

			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Contains(t, payload, test.expectedField)
			assert.NotContains(t, payload, "error")
			if test.expectedObject != "" {
				assert.Equal(t, test.expectedObject, payload["object"])
			}
		})
	}
}

func setupRelayRouterTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalIsMasterNode := common.IsMasterNode
	originalRedisEnabled := common.RedisEnabled
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}))

	t.Cleanup(func() {
		if sqlDB, err := model.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		common.IsMasterNode = originalIsMasterNode
		common.RedisEnabled = originalRedisEnabled
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
}

// TestRelayBodyRecordingPersistsStreamedExchange drives the full relay chain —
// router, token auth, channel selection, upstream call and log/body persistence
// — with body recording enabled globally and on the token. It pins the opt-in
// contract of ADR-0003: with both switches on, a streamed exchange must end up
// in request_bodies keyed to its usage log.
func TestRelayBodyRecordingPersistsStreamedExchange(t *testing.T) {
	database := setupRelayRouterTestBodyDB(t, constant.ChannelTypeOpenAI, "mock-model", "", openAIStreamSSE)

	engine := gin.New()
	engine.Use(middleware.RequestId())
	SetRelayRouter(engine)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mock-model","stream":true,"messages":[{"role":"user","content":"say hi"}]}`))
	req.Header.Set("Authorization", "Bearer bodyrecordtestkey")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	t.Logf("relay response: %d %s", recorder.Code, recorder.Body.String())
	require.Equal(t, http.StatusOK, recorder.Code, "the streamed relay must succeed")

	requestId := recorder.Header().Get(common.RequestIdKey)
	var stored model.RequestBody
	require.NoError(t, database.Where("request_id = ?", requestId).First(&stored).Error,
		"a streamed exchange must be recorded in request_bodies")
	assert.Contains(t, stored.RequestBody, "say hi")
	assert.Contains(t, stored.ResponseBody, "Hello")
}

// TestRelayBodyRecordingPersistsClaudeProtocolExchange reproduces the exact
// production shape of the deployment under investigation: an Advanced Custom
// (type 58) channel serving a Claude Code client through /v1/messages,
// streaming. If this fails while the OpenAI-protocol test above passes, the
// body-recording gap is protocol/channel-type specific, not configuration.
func TestRelayBodyRecordingPersistsClaudeProtocolExchange(t *testing.T) {
	database := setupRelayRouterTestBodyDB(t, constant.ChannelTypeAdvancedCustom, "mock-claude-model",
		`{"advanced_custom":{"advanced_routes":[{"incoming_path":"/v1/messages","upstream_path":"/v1/messages","converter":"none"}]}}`, claudeStreamSSE)

	engine := gin.New()
	engine.Use(middleware.RequestId())
	SetRelayRouter(engine)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"mock-claude-model","stream":true,"max_tokens":128,"messages":[{"role":"user","content":"say hi"}]}`))
	req.Header.Set("Authorization", "Bearer bodyrecordtestkey")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	t.Logf("relay response: %d %s", recorder.Code, recorder.Body.String())
	require.Equal(t, http.StatusOK, recorder.Code, "the streamed Claude relay must succeed")

	requestId := recorder.Header().Get(common.RequestIdKey)
	var stored model.RequestBody
	require.NoError(t, database.Where("request_id = ?", requestId).First(&stored).Error,
		"a streamed Claude exchange must be recorded in request_bodies")
	assert.Contains(t, stored.RequestBody, "say hi")
	assert.Contains(t, stored.ResponseBody, "Hello")
}

// openAIStreamSSE writes an OpenAI chat-completion stream fragment plus DONE.
func openAIStreamSSE(w http.ResponseWriter, r *http.Request) {
	_, _ = io.WriteString(w, `data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"mock-model","choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n\n")
	_, _ = io.WriteString(w, `data: {"id":"chatcmpl-mock","object":"chat.completion.chunk","created":1700000000,"model":"mock-model","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`+"\n\n")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

// claudeStreamSSE writes an Anthropic messages-stream: message_start, a text
// delta and message_stop.
func claudeStreamSSE(w http.ResponseWriter, r *http.Request) {
	_, _ = io.WriteString(w, `event: message_start`+"\n"+`data: {"type":"message_start","message":{"id":"msg_mock","type":"message","role":"assistant","model":"mock-claude-model","content":[],"usage":{"input_tokens":5,"output_tokens":5}}}`+"\n\n")
	_, _ = io.WriteString(w, `event: content_block_start`+"\n"+`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`+"\n\n")
	_, _ = io.WriteString(w, `event: content_block_delta`+"\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`+"\n\n")
	_, _ = io.WriteString(w, `event: content_block_stop`+"\n"+`data: {"type":"content_block_stop","index":0}`+"\n\n")
	_, _ = io.WriteString(w, `event: message_delta`+"\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`+"\n\n")
	_, _ = io.WriteString(w, `event: message_stop`+"\n"+`data: {"type":"message_stop"}`+"\n\n")
}

// setupRelayRouterTestBodyDB prepares the shared state of the body-recording
// tests: global master switch on, one token with per-token recording on, and
// an enabled channel of the given type serving the mocked streaming upstream.
func setupRelayRouterTestBodyDB(t *testing.T, channelType int, modelNames string, otherSettings string, streamBody func(http.ResponseWriter, *http.Request)) *gorm.DB {
	t.Helper()
	setupRelayRouterTestDB(t)
	database := model.DB

	require.NoError(t, i18n.Init())
	previousStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = previousStreamingTimeout })
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Log{}, &model.RequestBody{}, &model.UserSubscription{}))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"mock-model":37.5,"mock-claude-model":37.5}`))
	t.Cleanup(func() { _ = ratio_setting.UpdateModelRatioByJSONString("{}") })

	logSetting := operation_setting.GetLogSetting()
	previousBodyRecording := logSetting.RecordBodyEnabled
	logSetting.RecordBodyEnabled = true
	t.Cleanup(func() { logSetting.RecordBodyEnabled = previousBodyRecording })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		streamBody(w, r)
	}))
	t.Cleanup(upstream.Close)

	now := time.Now().Unix()
	user := model.User{Username: "body-user", Password: "x", Status: common.UserStatusEnabled, Group: "default", Quota: 500000}
	require.NoError(t, database.Create(&user).Error)
	token := model.Token{
		UserId: user.Id, Key: "bodyrecordtestkey", Status: common.TokenStatusEnabled,
		Name: "body-token", CreatedTime: now, AccessedTime: now,
		ExpiredTime: now + 86400, RemainQuota: 500000, UnlimitedQuota: true,
		Group: "default", RecordBodies: true,
	}
	require.NoError(t, database.Create(&token).Error)

	baseURL := upstream.URL
	priority := int64(0)
	channel := model.Channel{
		Type: channelType, Name: "mock-upstream", Status: common.ChannelStatusEnabled,
		Key: "sk-upstream", BaseURL: &baseURL, Models: modelNames, Group: "default", Priority: &priority,
	}
	if otherSettings != "" {
		channel.OtherSettings = otherSettings
	}
	require.NoError(t, database.Create(&channel).Error)
	for _, name := range strings.Split(modelNames, ",") {
		require.NoError(t, database.Create(&model.Ability{
			Group: "default", Model: name, ChannelId: channel.Id, Enabled: true, Priority: &priority,
		}).Error)
	}
	model.InitChannelCache()

	return database
}
