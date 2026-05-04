package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService service.AdminService
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{
		adminService: adminService,
	}
}

// AdminUpdateAPIKeyGroupRequest represents the request to update an API key.
type AdminUpdateAPIKeyGroupRequest struct {
	GroupID                   *int64 `json:"group_id"`                      // nil=不修改, 0=解绑, >0=绑定到目标分组
	QuotaShareOverflowGroupID *int64 `json:"quota_share_overflow_group_id"` // nil=不修改, 0=清空, >0=设置 quota_share 超限兜底分组
	ResetRateLimitUsage       *bool  `json:"reset_rate_limit_usage"`        // true=重置 5h/1d/7d 限速用量
	QuotaWeight               *int   `json:"quota_weight"`                  // nil=不修改, >0=设置 quota_share 权重
}

// UpdateGroup handles updating an API key's admin-managed fields.
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) UpdateGroup(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	var req AdminUpdateAPIKeyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var resetKey *service.APIKey
	if req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage {
		resetKey, err = h.adminService.AdminResetAPIKeyRateLimitUsage(c.Request.Context(), keyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	if req.QuotaWeight != nil {
		wKey, wErr := h.adminService.AdminUpdateAPIKeyQuotaWeight(c.Request.Context(), keyID, *req.QuotaWeight)
		if wErr != nil {
			response.ErrorFrom(c, wErr)
			return
		}
		if resetKey == nil && req.GroupID == nil {
			resetKey = wKey
		}
	}

	result, err := h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, req.GroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if req.QuotaShareOverflowGroupID != nil {
		updatedKey, overflowErr := h.adminService.AdminUpdateAPIKeyQuotaShareOverflowGroupID(c.Request.Context(), keyID, req.QuotaShareOverflowGroupID)
		if overflowErr != nil {
			response.ErrorFrom(c, overflowErr)
			return
		}
		result.APIKey = updatedKey
	}
	if resetKey != nil && req.GroupID == nil && req.QuotaShareOverflowGroupID == nil {
		result.APIKey = resetKey
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey:                 dto.APIKeyFromService(result.APIKey),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.Success(c, resp)
}
