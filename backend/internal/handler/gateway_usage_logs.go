package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type keyUsageLogDTO struct {
	CreatedAt             time.Time `json:"created_at"`
	Model                 string    `json:"model"`
	ServiceTier           *string   `json:"service_tier,omitempty"`
	ReasoningEffort       *string   `json:"reasoning_effort,omitempty"`
	InboundEndpoint       *string   `json:"inbound_endpoint,omitempty"`
	UpstreamEndpoint      *string   `json:"upstream_endpoint,omitempty"`
	RequestType           string    `json:"request_type"`
	InputTokens           int       `json:"input_tokens"`
	OutputTokens          int       `json:"output_tokens"`
	CacheCreationTokens   int       `json:"cache_creation_tokens"`
	CacheReadTokens       int       `json:"cache_read_tokens"`
	CacheCreation5mTokens int       `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int       `json:"cache_creation_1h_tokens"`
	ImageOutputTokens     int       `json:"image_output_tokens"`
	ImageOutputCost       float64   `json:"image_output_cost"`
	TotalTokens           int       `json:"total_tokens"`
	ActualCost            float64   `json:"actual_cost"`
	DurationMs            *int      `json:"duration_ms"`
	FirstTokenMs          *int      `json:"first_token_ms"`
	UserAgent             *string   `json:"user_agent,omitempty"`
	BillingMode           *string   `json:"billing_mode,omitempty"`
}

type keyUsageLogsResponse struct {
	Items    []keyUsageLogDTO `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Pages    int              `json:"pages"`
}

type keyUsageStatsResponse struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalInputTokens  int64   `json:"total_input_tokens"`
	TotalOutputTokens int64   `json:"total_output_tokens"`
	TotalCacheTokens  int64   `json:"total_cache_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalActualCost   float64 `json:"total_actual_cost"`
	AverageDurationMs float64 `json:"average_duration_ms"`
}

// UsageLogs returns read-only usage log details for the authenticated API key.
// GET /v1/usage/logs
func (h *GatewayHandler) UsageLogs(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if h.usageService == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Usage service unavailable")
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    "created_at",
		SortOrder: c.DefaultQuery("sort_order", pagination.SortOrderDesc),
	}
	if sortBy := strings.TrimSpace(c.Query("sort_by")); sortBy != "" && sortBy != "created_at" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "sort_by only supports created_at")
		return
	}

	filters, ok := h.parseKeyUsageLogFilters(c, apiKey.ID)
	if !ok {
		return
	}
	logs, pageResult, err := h.usageService.ListWithFilters(c.Request.Context(), params, filters)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to list usage logs")
		return
	}

	items := make([]keyUsageLogDTO, 0, len(logs))
	for i := range logs {
		items = append(items, keyUsageLogFromService(&logs[i]))
	}

	resp := keyUsageLogsResponse{Items: items, Total: 0, Page: page, PageSize: pageSize, Pages: 1}
	if pageResult != nil {
		resp.Total = pageResult.Total
		resp.Page = pageResult.Page
		resp.PageSize = pageResult.PageSize
		resp.Pages = pageResult.Pages
	}
	c.JSON(http.StatusOK, resp)
}

// UsageLogStats returns read-only usage stats for the authenticated API key.
// GET /v1/usage/logs/stats
func (h *GatewayHandler) UsageLogStats(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if h.usageService == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Usage service unavailable")
		return
	}

	filters, ok := h.parseKeyUsageLogFilters(c, apiKey.ID)
	if !ok {
		return
	}
	stats, err := h.usageService.GetStatsWithFilters(c.Request.Context(), filters)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to get usage stats")
		return
	}
	if stats == nil {
		stats = &usagestats.UsageStats{}
	}

	c.JSON(http.StatusOK, keyUsageStatsResponse{
		TotalRequests:     stats.TotalRequests,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalCacheTokens:  stats.TotalCacheTokens,
		TotalTokens:       stats.TotalTokens,
		TotalActualCost:   stats.TotalActualCost,
		AverageDurationMs: stats.AverageDurationMs,
	})
}

func (h *GatewayHandler) parseKeyUsageLogFilters(c *gin.Context, apiKeyID int64) (usagestats.UsageLogFilters, bool) {
	filters := usagestats.UsageLogFilters{APIKeyID: apiKeyID}

	if model := strings.TrimSpace(c.Query("model")); model != "" {
		filters.Model = model
	}
	if requestTypeStr := strings.TrimSpace(c.Query("request_type")); requestTypeStr != "" {
		parsed, err := service.ParseUsageRequestType(requestTypeStr)
		if err != nil {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
			return filters, false
		}
		value := int16(parsed)
		filters.RequestType = &value
	}

	startTime, endTime, ok := parseKeyUsageDateRange(c)
	if !ok {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Invalid date format, use YYYY-MM-DD")
		return filters, false
	}
	filters.StartTime = startTime
	filters.EndTime = endTime
	filters.ExactTotal = true
	return filters, true
}

func parseKeyUsageDateRange(c *gin.Context) (*time.Time, *time.Time, bool) {
	var startTime, endTime *time.Time
	userTZ := c.Query("timezone")

	if startDateStr := strings.TrimSpace(c.Query("start_date")); startDateStr != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", startDateStr, userTZ)
		if err != nil {
			return nil, nil, false
		}
		startTime = &t
	}
	if endDateStr := strings.TrimSpace(c.Query("end_date")); endDateStr != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", endDateStr, userTZ)
		if err != nil {
			return nil, nil, false
		}
		t = t.AddDate(0, 0, 1)
		endTime = &t
	}
	return startTime, endTime, true
}

func keyUsageLogFromService(l *service.UsageLog) keyUsageLogDTO {
	if l == nil {
		return keyUsageLogDTO{}
	}
	requestedModel := l.RequestedModel
	if requestedModel == "" {
		requestedModel = l.Model
	}
	return keyUsageLogDTO{
		CreatedAt:             l.CreatedAt,
		Model:                 requestedModel,
		ServiceTier:           l.ServiceTier,
		ReasoningEffort:       l.ReasoningEffort,
		InboundEndpoint:       l.InboundEndpoint,
		UpstreamEndpoint:      l.UpstreamEndpoint,
		RequestType:           l.EffectiveRequestType().String(),
		InputTokens:           l.InputTokens,
		OutputTokens:          l.OutputTokens,
		CacheCreationTokens:   l.CacheCreationTokens,
		CacheReadTokens:       l.CacheReadTokens,
		CacheCreation5mTokens: l.CacheCreation5mTokens,
		CacheCreation1hTokens: l.CacheCreation1hTokens,
		ImageOutputTokens:     l.ImageOutputTokens,
		ImageOutputCost:       l.ImageOutputCost,
		TotalTokens:           l.TotalTokens() + l.ImageOutputTokens,
		ActualCost:            l.ActualCost,
		DurationMs:            l.DurationMs,
		FirstTokenMs:          l.FirstTokenMs,
		UserAgent:             l.UserAgent,
		BillingMode:           l.BillingMode,
	}
}
