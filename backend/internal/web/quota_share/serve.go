package quotashare

import (
	"embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed index.html admin.html
var pages embed.FS

func RegisterRoutes(r *gin.Engine) {
	r.GET("/quota-share", servePage("index.html"))
	r.GET("/quota-share/admin", servePage("admin.html"))
}

func servePage(name string) gin.HandlerFunc {
	data, err := pages.ReadFile(name)
	if err != nil {
		panic("quota_share: missing embedded page " + name)
	}
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}
}
