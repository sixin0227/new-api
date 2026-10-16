package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}, &model.RequestBody{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 202)
	ctx.Set("channel_name", "mutable-context-channel")
	ctx.Set("channel_type", 9)
	ctx.Set("use_channel", []string{"101"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	channelSnapshot := types.ChannelError{
		ChannelId:   101,
		ChannelType: 1,
		ChannelName: "snapshot-channel",
		AutoBan:     false,
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	processChannelError(ctx, channelSnapshot, apiErr, nil)

	var stored model.Log
	require.NoError(t, database.First(&stored).Error)
	assert.Equal(t, channelSnapshot.ChannelId, stored.ChannelId)
	storedOther, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusBadGateway), storedOther["status_code"])
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, storedOther, key)
	}
	adminInfo, ok := storedOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"101"}, adminInfo["use_channel"])

	logs, total, err := model.GetUserLogs(7, model.LogFilter{Type: model.LogTypeError}, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, channelSnapshot.ChannelId, logs[0].ChannelId)
	assert.Empty(t, logs[0].ChannelName)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, userOther, key)
	}
}

// setupBodyRecordTestDB builds an in-memory SQLite database wired as both the
// main and log database, restoring global state afterwards.
func setupBodyRecordTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}, &model.RequestBody{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	return database
}

func newBodyRecordTestContext(t *testing.T, requestId string) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set(common.RequestIdKey, requestId)
	return ctx
}

// TestBodyRecordingPersistsSearchesAndDeletes covers the recorded-body path
// end to end: a streamed response is reassembled into one complete JSON
// object, bodies persist next to the consume log, the two body search filters
// find the row through request_bodies, and bulk deletion removes log and
// bodies together.
func TestBodyRecordingPersistsSearchesAndDeletes(t *testing.T) {
	database := setupBodyRecordTestDB(t)
	now := time.Now().Unix()
	require.NoError(t, database.Exec("CREATE TABLE channels (id integer primary key, name varchar(255))").Error)
	require.NoError(t, database.Exec("INSERT INTO channels (id, name) VALUES (202, 'test-channel')").Error)

	ctx := newBodyRecordTestContext(t, "req-rec-1")
	br := common.NewBodyRecord(1<<20, "openai")
	br.RecordRequest([]byte(`{"model":"gpt-test","messages":[{"role":"user","content":"say hi"}]}`))
	common.SetBodyRecord(ctx, br)

	sse := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"Hello"}}]}` + "\n\n" +
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":" there"}}]}` + "\n\n"
	tee := common.TeeResponseBody(ctx, io.NopCloser(strings.NewReader(sse)), "text/event-stream")
	relayed, err := io.ReadAll(tee)
	require.NoError(t, err)
	assert.Equal(t, sse, string(relayed), "teeing must not alter what the client receives")
	require.False(t, br.RespSkipped)

	model.RecordConsumeLog(ctx, 7, model.RecordConsumeLogParams{
		ChannelId:        202,
		PromptTokens:     5,
		CompletionTokens: 7,
		UseTimeSeconds:   3,
		TokenName:        "test-token",
		ModelName:        "gpt-test",
		TokenId:          11,
		IsStream:         true,
		Group:            "default",
	})

	var stored model.RequestBody
	require.NoError(t, database.Where("request_id = ?", "req-rec-1").First(&stored).Error)
	assert.False(t, stored.RequestSkipped)
	assert.False(t, stored.ResponseSkipped)
	assert.Contains(t, stored.RequestBody, "say hi")
	// Write-time summaries power the body browser list.
	assert.Equal(t, "gpt-test", stored.Model)
	assert.Equal(t, "say hi", stored.RequestPreview)
	assert.Equal(t, "Hello there", stored.ResponsePreview)
	var reassembled map[string]any
	require.NoError(t, common.UnmarshalJsonStr(stored.ResponseBody, &reassembled))
	choices, ok := reassembled["choices"].([]any)
	require.True(t, ok, "reassembled response must keep the choices array")
	message, ok := choices[0].(map[string]any)["message"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Hello there", message["content"])

	// A body above the configured cap is skipped whole, not truncated.
	ctxSkip := newBodyRecordTestContext(t, "req-rec-2")
	brSkip := common.NewBodyRecord(16, "openai")
	brSkip.RecordRequest([]byte(`{"model":"gpt-test","input":"` + string(make([]byte, 64)) + `"}`))
	require.True(t, brSkip.RequestSkipped)
	common.SetBodyRecord(ctxSkip, brSkip)
	teeSkip := common.TeeResponseBody(ctxSkip, io.NopCloser(strings.NewReader("data: "+string(make([]byte, 64))+"\n\n")), "text/event-stream")
	_, err = io.ReadAll(teeSkip)
	require.NoError(t, err)
	require.True(t, brSkip.RespSkipped)
	model.RecordConsumeLog(ctxSkip, 7, model.RecordConsumeLogParams{TokenName: "test-token", ModelName: "gpt-test", TokenId: 11, Group: "default"})
	var skipped model.RequestBody
	require.NoError(t, database.Where("request_id = ?", "req-rec-2").First(&skipped).Error)
	assert.True(t, skipped.RequestSkipped)
	assert.True(t, skipped.ResponseSkipped)

	// Body search matches through the request_bodies subquery…
	logs, total, err := model.GetAllLogs(model.LogFilter{
		StartTimestamp:  now - 60,
		EndTimestamp:    now + 60,
		ReqBodyContains: "say hi",
	}, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, "req-rec-1", logs[0].RequestId)

	// …and the response filter matches inside the reassembled content.
	logs, total, err = model.GetAllLogs(model.LogFilter{
		StartTimestamp:   now - 60,
		EndTimestamp:     now + 60,
		RespBodyContains: "Hello there",
	}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)

	// The body browser lists records with their summaries and the full
	// payloads, filtered by content and user.
	items, listTotal, err := model.GetRequestBodyList(model.RequestBodyFilter{}, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(2), listTotal)
	require.Len(t, items, 2)
	assert.Equal(t, "req-rec-2", items[0].RequestId)
	assert.Equal(t, "log-owner", items[0].Username)
	assert.Equal(t, "gpt-test", items[1].Model)
	assert.Contains(t, items[1].RequestBody, "say hi")
	assert.Contains(t, items[1].ResponseBody, "Hello there")

	// The usage-log metadata of each exchange (channel, token, stream flag,
	// tokens, duration) is merged into its listed record.
	assert.True(t, items[1].HasLogMetadata)
	assert.Equal(t, 202, items[1].ChannelId)
	assert.Equal(t, "test-channel", items[1].ChannelName)
	assert.Equal(t, "test-token", items[1].TokenName)
	assert.True(t, items[1].IsStream)
	assert.Equal(t, 5, items[1].PromptTokens)
	assert.Equal(t, 7, items[1].CompletionTokens)
	assert.Equal(t, 3, items[1].UseTime)
	assert.True(t, items[0].HasLogMetadata)
	assert.Zero(t, items[0].ChannelId)
	assert.Empty(t, items[0].ChannelName)

	// A record whose log row is gone reports no metadata instead of inventing
	// zero values, and keeps the summary model as its model.
	require.NoError(t, database.Create(&model.RequestBody{
		RequestId:   "req-orphan",
		UserId:      7,
		CreatedAt:   now + 5,
		Model:       "orphan-model",
		RequestBody: `{"model":"orphan-model","messages":[{"role":"user","content":"orphan"}]}`,
	}).Error)
	items, listTotal, err = model.GetRequestBodyList(model.RequestBodyFilter{}, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(3), listTotal)
	require.Len(t, items, 3)
	assert.Equal(t, "req-orphan", items[0].RequestId)
	assert.False(t, items[0].HasLogMetadata)
	assert.Equal(t, "orphan-model", items[0].Model)
	assert.Empty(t, items[0].ChannelName)
	assert.Zero(t, items[0].UseTime)

	items, listTotal, err = model.GetRequestBodyList(model.RequestBodyFilter{Contains: "say hi"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), listTotal)
	require.Len(t, items, 1)
	assert.Equal(t, "req-rec-1", items[0].RequestId)

	items, listTotal, err = model.GetRequestBodyList(model.RequestBodyFilter{UserId: 99}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), listTotal)
	assert.Empty(t, items)

	// A literal search must not treat wildcards as their own pattern.
	items, listTotal, err = model.GetRequestBodyList(model.RequestBodyFilter{Contains: "say%hi"}, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), listTotal)
	assert.Empty(t, items)

	// Export loads the full bodies for the selected exchanges.
	full, err := model.GetRequestBodiesByRequestIds([]string{"req-rec-1"})
	require.NoError(t, err)
	require.Len(t, full, 1)
	assert.Contains(t, full[0].RequestBody, "say hi")

	// Deleting by request ids removes the body row and its usage-log row,
	// leaving the other exchange untouched.
	deletedByRequestId, err := model.DeleteRequestBodiesByRequestIds([]string{"req-rec-2"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), deletedByRequestId)
	var remainingLogRows int64
	require.NoError(t, database.Model(&model.Log{}).Where("request_id = ?", "req-rec-2").Count(&remainingLogRows).Error)
	assert.Equal(t, int64(0), remainingLogRows)
	var survivorBodies int64
	require.NoError(t, database.Model(&model.RequestBody{}).Where("request_id = ?", "req-rec-1").Count(&survivorBodies).Error)
	assert.Equal(t, int64(1), survivorBodies)

	// Bulk deletion removes the log row and its bodies together.
	deleted, err := model.DeleteLogsAndBodies([]int{logs[0].Id})
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	var remainingBodies int64
	require.NoError(t, database.Model(&model.RequestBody{}).Where("request_id = ?", "req-rec-1").Count(&remainingBodies).Error)
	assert.Equal(t, int64(0), remainingBodies)
	var remainingLogs int64
	require.NoError(t, database.Model(&model.Log{}).Where("request_id = ?", "req-rec-1").Count(&remainingLogs).Error)
	assert.Equal(t, int64(0), remainingLogs)

	// Delete-all honors the browse filters: a filter with no match removes
	// nothing…
	deletedAll, err := model.DeleteAllRequestBodiesByFilter(model.RequestBodyFilter{UserId: 99})
	require.NoError(t, err)
	assert.Zero(t, deletedAll)
	var remainingOrphans int64
	require.NoError(t, database.Model(&model.RequestBody{}).Where("request_id = ?", "req-orphan").Count(&remainingOrphans).Error)
	assert.Equal(t, int64(1), remainingOrphans)

	// …while a matching filter removes every remaining record.
	deletedAll, err = model.DeleteAllRequestBodiesByFilter(model.RequestBodyFilter{UserId: 7})
	require.NoError(t, err)
	assert.Equal(t, int64(1), deletedAll)
	require.NoError(t, database.Model(&model.RequestBody{}).Count(&remainingOrphans).Error)
	assert.Zero(t, remainingOrphans)
}

// TestDeleteAllRequestBodiesSweepsInBatches covers the batched server-side
// sweep: a filtered set larger than one batch is fully removed together with
// the usage-log rows of its exchanges.
func TestDeleteAllRequestBodiesSweepsInBatches(t *testing.T) {
	database := setupBodyRecordTestDB(t)
	now := time.Now().Unix()

	// One row above the batch boundary proves the loop advances past the first
	// batch and sums the counts of both.
	const totalRows = 501
	bodies := make([]model.RequestBody, 0, totalRows)
	for i := range totalRows {
		bodies = append(bodies, model.RequestBody{
			RequestId: fmt.Sprintf("req-batch-%d", i),
			UserId:    7,
			CreatedAt: now,
		})
	}
	require.NoError(t, database.CreateInBatches(&bodies, 100).Error)
	require.NoError(t, database.Create(&model.Log{UserId: 7, Username: "log-owner", CreatedAt: now, Type: model.LogTypeConsume, RequestId: "req-batch-0"}).Error)
	require.NoError(t, database.Create(&model.Log{UserId: 8, Username: "other", CreatedAt: now, Type: model.LogTypeConsume, RequestId: "req-batch-1"}).Error)

	deleted, err := model.DeleteAllRequestBodiesByFilter(model.RequestBodyFilter{UserId: 7})
	require.NoError(t, err)
	assert.Equal(t, int64(totalRows), deleted)

	var remainingBodies int64
	require.NoError(t, database.Model(&model.RequestBody{}).Count(&remainingBodies).Error)
	assert.Zero(t, remainingBodies)
	// Each usage-log row goes with its exchange, whichever user wrote the log.
	var remainingLogs int64
	require.NoError(t, database.Model(&model.Log{}).Count(&remainingLogs).Error)
	assert.Zero(t, remainingLogs)
}

// TestBodyBrowserDatabaseMatrix runs the metadata merge and the batched
// delete-all against every configured database engine, mirroring the log
// export matrix: sqlite always runs, the others use their AUDIT_* DSN.
func TestBodyBrowserDatabaseMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = true
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	for _, tc := range []struct {
		kind, env string
		typ       common.DatabaseType
	}{
		{"sqlite", "", common.DatabaseTypeSQLite},
		{"mysql", "AUDIT_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "AUDIT_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
		{"clickhouse", "AUDIT_CLICKHOUSE_DSN", common.DatabaseTypeClickHouse},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if tc.env != "" && dsn == "" {
				t.Skip(tc.env + " is not configured")
			}
			logDB, isolatedDSN := newAuditTestDatabase(t, tc.kind, dsn)
			mainDB, mainType := logDB, tc.typ
			if tc.kind == "clickhouse" {
				mainDB, _ = newAuditTestDatabase(t, "sqlite", "")
				mainType = common.DatabaseTypeSQLite
			}
			useLogExportDatabases(t, mainDB, logDB, mainType, tc.typ)
			if tc.kind == "clickhouse" {
				t.Setenv("LOG_SQL_DSN", isolatedDSN)
				require.NoError(t, model.InitLogDB())
			} else {
				require.NoError(t, logDB.AutoMigrate(&model.Log{}, &model.RequestBody{}))
			}
			var version string
			versionSQL := "SELECT version()"
			if tc.kind == "sqlite" {
				versionSQL = "SELECT sqlite_version()"
			}
			require.NoError(t, logDB.Raw(versionSQL).Scan(&version).Error)
			t.Logf("database matrix: %s %s", tc.kind, version)

			now := time.Now().Unix()
			require.NoError(t, mainDB.AutoMigrate(&model.Channel{}))
			require.NoError(t, mainDB.Create(&model.Channel{Id: 202, Name: "matrix-channel"}).Error)

			// One exchange with its usage log proves the metadata merge, and one
			// row above the batch boundary proves the sweep loops across batches.
			require.NoError(t, logDB.Create(&model.Log{UserId: 7, Username: "log-owner", CreatedAt: now, Type: model.LogTypeConsume, ModelName: "matrix-model", TokenName: "matrix-token", ChannelId: 202, PromptTokens: 3, CompletionTokens: 4, UseTime: 2, IsStream: true, RequestId: "req-matrix-1"}).Error)
			const totalRows = 501
			bodies := make([]model.RequestBody, 0, totalRows)
			bodies = append(bodies, model.RequestBody{RequestId: "req-matrix-1", UserId: 7, CreatedAt: now, Model: "summary-model"})
			for i := range totalRows - 1 {
				bodies = append(bodies, model.RequestBody{RequestId: fmt.Sprintf("req-matrix-batch-%d", i), UserId: 7, CreatedAt: now})
			}
			require.NoError(t, logDB.CreateInBatches(&bodies, 100).Error)

			items, total, err := model.GetRequestBodyList(model.RequestBodyFilter{}, 0, totalRows)
			require.NoError(t, err)
			require.Equal(t, int64(totalRows), total)
			var merged *model.RequestBodyListItem
			for _, item := range items {
				if item.RequestId == "req-matrix-1" {
					merged = item
					break
				}
			}
			require.NotNil(t, merged, "the exchange with a usage log must be listed")
			assert.True(t, merged.HasLogMetadata)
			assert.Equal(t, "matrix-channel", merged.ChannelName)
			assert.Equal(t, "matrix-token", merged.TokenName)
			assert.True(t, merged.IsStream)
			assert.Equal(t, 3, merged.PromptTokens)
			assert.Equal(t, 4, merged.CompletionTokens)
			assert.Equal(t, 2, merged.UseTime)
			assert.Equal(t, "matrix-model", merged.Model)

			deleted, err := model.DeleteAllRequestBodiesByFilter(model.RequestBodyFilter{UserId: 7})
			require.NoError(t, err)
			assert.Equal(t, int64(totalRows), deleted)
			var remainingBodies int64
			require.NoError(t, logDB.Model(&model.RequestBody{}).Count(&remainingBodies).Error)
			assert.Zero(t, remainingBodies)
			var remainingLogs int64
			require.NoError(t, logDB.Model(&model.Log{}).Count(&remainingLogs).Error)
			assert.Zero(t, remainingLogs)
		})
	}
}

// TestSummarizeBodyRecord covers the write-time summary extraction that the
// body browser depends on: chat payloads of both protocol shapes, multimodal
// content, non-chat inputs, error responses, and long-text truncation.
func TestSummarizeBodyRecord(t *testing.T) {
	cases := []struct {
		name         string
		requestBody  string
		responseBody string
		wantModel    string
		wantRequest  string
		wantResponse string
	}{
		{
			name:         "openai chat keeps the last user message",
			requestBody:  `{"model":"gpt-4o","messages":[{"role":"system","content":"be nice"},{"role":"user","content":"first"},{"role":"user","content":"latest question"}]}`,
			responseBody: `{"object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"final answer"}}]}`,
			wantModel:    "gpt-4o",
			wantRequest:  "latest question",
			wantResponse: "final answer",
		},
		{
			name:         "multimodal user content falls back to its text block",
			requestBody:  `{"model":"claude-3","messages":[{"role":"user","content":[{"type":"image","source":{}},{"type":"text","text":"describe this"}]}]}`,
			responseBody: `{"type":"message","content":[{"type":"text","text":"a cat"}]}`,
			wantModel:    "claude-3",
			wantRequest:  "describe this",
			wantResponse: "a cat",
		},
		{
			name:         "embedding input is used as the preview",
			requestBody:  `{"model":"text-embedding-3","input":"embed me"}`,
			responseBody: `{}`,
			wantModel:    "text-embedding-3",
			wantRequest:  "embed me",
			wantResponse: "{}",
		},
		{
			name:         "error responses keep their message visible",
			requestBody:  `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`,
			responseBody: `{"error":{"message":"upstream overloaded","type":"server_error"}}`,
			wantModel:    "gpt-4o",
			wantRequest:  "hi",
			wantResponse: `{"error":{"message":"upstream overloaded","type":"server_error"}}`,
		},
		{
			name:         "non-json payloads fall back to a plain prefix",
			requestBody:  `not json at all`,
			responseBody: ``,
			wantModel:    "",
			wantRequest:  "not json at all",
			wantResponse: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			summaries := model.SummarizeBodyRecord(tc.requestBody, tc.responseBody)
			assert.Equal(t, tc.wantModel, summaries.Model)
			assert.Equal(t, tc.wantRequest, summaries.RequestPreview)
			assert.Equal(t, tc.wantResponse, summaries.ResponsePreview)
		})
	}

	// A rune boundary that does not line up with the byte limit must back off
	// instead of emitting a broken character.
	longText := "a" + strings.Repeat("字", 200)
	summaries := model.SummarizeBodyRecord(`{"model":"m","messages":[{"role":"user","content":"`+longText+`"}]}`, "")
	assert.True(t, strings.HasSuffix(summaries.RequestPreview, "…"))
	assert.LessOrEqual(t, len(summaries.RequestPreview), 300+3)
	assert.True(t, utf8.ValidString(summaries.RequestPreview), "truncation must not split a character")
}
