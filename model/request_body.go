package model

import (
	"context"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RequestBody stores the recorded request and response bodies of one relay
// exchange, keyed to its usage/error log through RequestId. Bodies are
// stored on the log database only; see docs/adr/0001-request-bodies-separate-table.md.
// Model and the preview columns cache the write-time BodySummaries so the
// body browser can list records without loading the full bodies.
type RequestBody struct {
	Id              int64  `json:"id" gorm:"primaryKey"`
	RequestId       string `json:"request_id" gorm:"size:64;index"`
	UserId          int    `json:"user_id" gorm:"index"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index"`
	Model           string `json:"model" gorm:"size:64"`
	RequestPreview  string `json:"request_preview" gorm:"type:text"`
	ResponsePreview string `json:"response_preview" gorm:"type:text"`
	RequestBody     string `json:"request_body" gorm:"type:text"`
	ResponseBody    string `json:"response_body" gorm:"type:text"`
	RequestSkipped  bool   `json:"request_skipped"`
	ResponseSkipped bool   `json:"response_skipped"`
}

func (RequestBody) TableName() string {
	return "request_bodies"
}

// PersistRequestBodies writes the bodies accumulated on the request's
// BodyRecord next to the log row that was just created. It is a no-op when
// body recording is off, so the default deployment pays nothing for it.
func PersistRequestBodies(c *gin.Context, requestId string, userId int) {
	br := common.GetBodyRecord(c)
	if br == nil || !br.Enabled || requestId == "" {
		return
	}
	rb := &RequestBody{
		RequestId:       requestId,
		UserId:          userId,
		CreatedAt:       common.GetTimestamp(),
		RequestSkipped:  br.RequestSkipped,
		ResponseSkipped: br.RespSkipped,
	}
	rb.RequestBody = string(br.RequestBody)
	rb.ResponseBody = br.ResponseBody()
	if rb.RequestBody == "" && rb.ResponseBody == "" && !rb.RequestSkipped && !rb.ResponseSkipped {
		return
	}
	summaries := SummarizeBodyRecord(rb.RequestBody, rb.ResponseBody)
	rb.Model = summaries.Model
	rb.RequestPreview = summaries.RequestPreview
	rb.ResponsePreview = summaries.ResponsePreview
	if err := LOG_DB.Create(rb).Error; err != nil {
		common.SysLog(common.LogText("failed to record request bodies: %s", err.Error()))
	}
}

func GetRequestBodyByRequestId(requestId string) (*RequestBody, error) {
	var rb RequestBody
	err := LOG_DB.Where("request_id = ?", requestId).Order("id desc").First(&rb).Error
	if err != nil {
		return nil, err
	}
	return &rb, nil
}

// DeleteLogsAndBodies removes whole usage-log entries: the log rows and the
// body rows recorded for them, so no orphan bodies survive.
func DeleteLogsAndBodies(ids []int) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var logs []Log
	if err := LOG_DB.Where("id IN ?", ids).Find(&logs).Error; err != nil {
		return 0, err
	}
	if len(logs) == 0 {
		return 0, nil
	}
	requestIds := make([]string, 0, len(logs))
	for _, log := range logs {
		if log.RequestId != "" {
			requestIds = append(requestIds, log.RequestId)
		}
	}

	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse rejects transactions and GORM DELETE; run the same
		// mutation-style removals the log-cleanup task uses.
		if len(requestIds) > 0 {
			if err := LOG_DB.Exec("ALTER TABLE request_bodies DELETE WHERE request_id IN ?", requestIds).Error; err != nil {
				return 0, err
			}
		}
		if err := LOG_DB.Exec("ALTER TABLE logs DELETE WHERE id IN ?", ids).Error; err != nil {
			return 0, err
		}
		return int64(len(logs)), nil
	}

	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		if len(requestIds) > 0 {
			if err := tx.Where("request_id IN ?", requestIds).Delete(&RequestBody{}).Error; err != nil {
				return err
			}
		}
		return tx.Where("id IN ?", ids).Delete(&Log{}).Error
	})
	if err != nil {
		return 0, err
	}
	return int64(len(logs)), nil
}

// DeleteOldBodiesBatch removes bodies older than targetTimestamp, mirroring
// the batched log cleanup so the two tables never drift apart. The whole
// sweep runs unbounded per batch because DELETE ... LIMIT is not portable to
// SQLite/PostgreSQL; the caller loops on log rows, not body rows.
func DeleteOldBodiesBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		var total int64
		if err := LOG_DB.WithContext(ctx).Model(&RequestBody{}).Where("created_at < ?", targetTimestamp).Count(&total).Error; err != nil {
			return 0, err
		}
		if total == 0 {
			return 0, nil
		}
		err := LOG_DB.WithContext(ctx).Exec(
			"ALTER TABLE request_bodies DELETE WHERE created_at < ? SETTINGS mutations_sync = 1",
			targetTimestamp,
		).Error
		return total, err
	}
	result := LOG_DB.WithContext(ctx).Where("created_at < ?", targetTimestamp).Delete(&RequestBody{})
	return result.RowsAffected, result.Error
}

// RequestBodyFilter is the body browser search. Contains is a literal
// substring matched against both recorded bodies.
type RequestBodyFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
	Contains       string
	UserId         int
}

// RequestBodyListItem is one row of the body browser. It carries the
// write-time summary alongside the full bodies, because the browser renders
// the recorded content directly; the usage-log metadata (channel, token,
// stream, tokens, duration) is merged in from the log row of the exchange.
type RequestBodyListItem struct {
	Id               int64  `json:"id"`
	RequestId        string `json:"request_id"`
	UserId           int    `json:"user_id"`
	Username         string `json:"username"`
	CreatedAt        int64  `json:"created_at"`
	Model            string `json:"model"`
	HasLogMetadata   bool   `json:"has_log_metadata"`
	ChannelId        int    `json:"channel_id"`
	ChannelName      string `json:"channel_name"`
	TokenName        string `json:"token_name"`
	IsStream         bool   `json:"is_stream"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	UseTime          int    `json:"use_time"`
	RequestPreview   string `json:"request_preview"`
	ResponsePreview  string `json:"response_preview"`
	RequestBody      string `json:"request_body"`
	ResponseBody     string `json:"response_body"`
	RequestSkipped   bool   `json:"request_skipped"`
	ResponseSkipped  bool   `json:"response_skipped"`
}

func (f RequestBodyFilter) apply(tx *gorm.DB) *gorm.DB {
	if f.StartTimestamp != 0 {
		tx = tx.Where("created_at >= ?", f.StartTimestamp)
	}
	if f.EndTimestamp != 0 {
		tx = tx.Where("created_at <= ?", f.EndTimestamp)
	}
	if f.UserId != 0 {
		tx = tx.Where("user_id = ?", f.UserId)
	}
	if f.Contains != "" {
		requestCondition, requestPattern := bodyContainsCondition("request_body", f.Contains)
		responseCondition, responsePattern := bodyContainsCondition("response_body", f.Contains)
		tx = tx.Where("("+requestCondition+" OR "+responseCondition+")", requestPattern, responsePattern)
	}
	return tx
}

func GetRequestBodyList(filter RequestBodyFilter, startIdx int, num int) (items []*RequestBodyListItem, total int64, err error) {
	err = filter.apply(LOG_DB.Model(&RequestBody{})).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	order := "created_at desc, id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = "created_at desc, request_id desc"
	}
	var rows []*RequestBody
	err = filter.apply(LOG_DB.Model(&RequestBody{})).
		Order(order).Limit(num).Offset(startIdx).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	items = make([]*RequestBodyListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, &RequestBodyListItem{
			Id:              row.Id,
			RequestId:       row.RequestId,
			UserId:          row.UserId,
			CreatedAt:       row.CreatedAt,
			Model:           row.Model,
			RequestPreview:  row.RequestPreview,
			ResponsePreview: row.ResponsePreview,
			RequestBody:     row.RequestBody,
			ResponseBody:    row.ResponseBody,
			RequestSkipped:  row.RequestSkipped,
			ResponseSkipped: row.ResponseSkipped,
		})
	}
	appendRequestBodyUsernames(items)
	appendRequestBodyLogMetadata(items)
	return items, total, nil
}

// appendRequestBodyUsernames fills usernames from the primary database, the
// same cross-database batch lookup the log list uses for channel names. A
// missing user (deleted account) leaves the id visible with an empty name.
func appendRequestBodyUsernames(items []*RequestBodyListItem) {
	userIds := make([]int, 0, len(items))
	for _, item := range items {
		if item.UserId != 0 {
			userIds = append(userIds, item.UserId)
		}
	}
	if len(userIds) == 0 {
		return
	}
	var users []struct {
		Id       int
		Username string
	}
	if err := DB.Table("users").Select("id, username").Where("id IN ?", userIds).Find(&users).Error; err != nil {
		common.SysLog(common.LogText("failed to resolve body record usernames: %s", err.Error()))
		return
	}
	usernameById := make(map[int]string, len(users))
	for _, user := range users {
		usernameById[user.Id] = user.Username
	}
	for _, item := range items {
		item.Username = usernameById[item.UserId]
	}
}

// appendRequestBodyLogMetadata fills each listed body record's usage-log
// metadata — channel, token name, stream flag, tokens and duration — from the
// log row of its exchange, matched through the request id, the same
// cross-database batch lookup the list uses for usernames. The metadata lives
// in the logs table only; a record without a surviving log row keeps empty
// values and has HasLogMetadata false.
func appendRequestBodyLogMetadata(items []*RequestBodyListItem) {
	requestIds := make([]string, 0, len(items))
	for _, item := range items {
		if item.RequestId != "" {
			requestIds = append(requestIds, item.RequestId)
		}
	}
	if len(requestIds) == 0 {
		return
	}
	// A request id normally has one log row; an exchange that produced both an
	// error and a consume log leaves more, and the newest row carries the
	// metadata of the final attempt.
	var logs []Log
	order := "id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("")
	}
	if err := LOG_DB.Where("request_id IN ?", requestIds).Order(order).Find(&logs).Error; err != nil {
		common.SysLog(common.LogText("failed to resolve body record log metadata: %s", err.Error()))
		return
	}
	logByRequestId := make(map[string]*Log, len(logs))
	for i := range logs {
		if _, exists := logByRequestId[logs[i].RequestId]; !exists {
			logByRequestId[logs[i].RequestId] = &logs[i]
		}
	}

	channelIds := make([]int, 0, len(logByRequestId))
	for _, log := range logByRequestId {
		if log.ChannelId != 0 {
			channelIds = append(channelIds, log.ChannelId)
		}
	}
	channelNames := make(map[int]string, len(channelIds))
	if common.MemoryCacheEnabled {
		for _, channelId := range channelIds {
			if cacheChannel, err := CacheGetChannel(channelId); err == nil {
				channelNames[channelId] = cacheChannel.Name
			}
		}
	} else if len(channelIds) > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err := DB.Table("channels").Select("id, name").Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
			common.SysLog(common.LogText("failed to resolve body record channel names: %s", err.Error()))
		} else {
			for _, channel := range channels {
				channelNames[channel.Id] = channel.Name
			}
		}
	}

	for _, item := range items {
		log, exists := logByRequestId[item.RequestId]
		if !exists {
			continue
		}
		item.HasLogMetadata = true
		item.ChannelId = log.ChannelId
		item.ChannelName = channelNames[log.ChannelId]
		item.TokenName = log.TokenName
		item.IsStream = log.IsStream
		item.PromptTokens = log.PromptTokens
		item.CompletionTokens = log.CompletionTokens
		item.UseTime = log.UseTime
		if log.ModelName != "" {
			item.Model = log.ModelName
		}
	}
}

// GetRequestBodiesByRequestIds loads the full records for an export. Callers
// bound the id count so one export cannot pull the whole table.
func GetRequestBodiesByRequestIds(requestIds []string) ([]*RequestBody, error) {
	if len(requestIds) == 0 {
		return nil, nil
	}
	var items []*RequestBody
	err := LOG_DB.Where("request_id IN ?", requestIds).Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

// DeleteRequestBodiesByRequestIds removes the body rows and their usage-log
// rows together, so no orphan on either side survives the deletion.
func DeleteRequestBodiesByRequestIds(requestIds []string) (int64, error) {
	return deleteRequestBodiesAndLogs(requestIds, false)
}

// DeleteAllRequestBodiesByFilter removes every recorded exchange matching the
// filter — bodies and their usage-log rows together — sweeping the table
// server-side in batches so one request never loads it into memory.
func DeleteAllRequestBodiesByFilter(filter RequestBodyFilter) (int64, error) {
	const batchSize = 500
	var deleted int64
	for {
		var requestIds []string
		err := filter.apply(LOG_DB.Model(&RequestBody{})).
			Limit(batchSize).Pluck("request_id", &requestIds).Error
		if err != nil {
			return deleted, err
		}
		if len(requestIds) == 0 {
			return deleted, nil
		}
		batchDeleted, err := deleteRequestBodiesAndLogs(requestIds, true)
		if err != nil {
			return deleted, err
		}
		deleted += batchDeleted
	}
}

// deleteRequestBodiesAndLogs removes the body rows and their usage-log rows
// together. syncMutations makes ClickHouse wait for each mutation to finish,
// which the batched delete-all loop needs so its next batch sees the removals.
func deleteRequestBodiesAndLogs(requestIds []string, syncMutations bool) (int64, error) {
	if len(requestIds) == 0 {
		return 0, nil
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse rejects transactions and GORM DELETE; run the same
		// mutation-style removals the log-cleanup task uses.
		settings := ""
		if syncMutations {
			settings = " SETTINGS mutations_sync = 1"
		}
		if err := LOG_DB.Exec("ALTER TABLE request_bodies DELETE WHERE request_id IN ?"+settings, requestIds).Error; err != nil {
			return 0, err
		}
		if err := LOG_DB.Exec("ALTER TABLE logs DELETE WHERE request_id IN ?"+settings, requestIds).Error; err != nil {
			return 0, err
		}
		return int64(len(requestIds)), nil
	}
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("request_id IN ?", requestIds).Delete(&RequestBody{}).Error; err != nil {
			return err
		}
		return tx.Where("request_id IN ?", requestIds).Delete(&Log{}).Error
	})
	if err != nil {
		return 0, err
	}
	return int64(len(requestIds)), nil
}

// bodyContainsCondition builds the literal substring condition for one body
// column. The value is escaped as literal text (wildcards and the escape
// character included) before being wrapped in %…%, so user input can never
// widen its own match.
func bodyContainsCondition(column string, value string) (string, string) {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse LIKE treats backslash as the escape character and has no
		// ESCAPE clause.
		escaped := strings.ReplaceAll(value, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `_`, `\_`)
		escaped = strings.ReplaceAll(escaped, `%`, `\%`)
		return column + " LIKE ?", "%" + escaped + "%"
	}
	// SQLite/MySQL follow the project-wide LIKE convention with ! as escape.
	escaped := strings.ReplaceAll(value, "!", "!!")
	escaped = strings.ReplaceAll(escaped, "_", "!_")
	escaped = strings.ReplaceAll(escaped, "%", "!%")
	return column + " LIKE ? ESCAPE '!'", "%" + escaped + "%"
}

// RequestBodySearchSubquery builds the request_bodies subquery used by the
// log list body search.
func RequestBodySearchSubquery(column string, value string) (string, string) {
	condition, pattern := bodyContainsCondition(column, value)
	return "logs.request_id IN (SELECT request_id FROM request_bodies WHERE " + condition + ")", pattern
}
