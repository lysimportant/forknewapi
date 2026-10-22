package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// WebAssets 保存嵌入式前端构建产物及站点 SEO 配置。
type WebAssets struct {
	// BuildFS 包含 web/dist 下的静态资源。
	BuildFS embed.FS
	// IndexPage 是 SPA 入口页模板。
	IndexPage []byte
	// SiteSEO 可在测试或非标准构建流程中覆盖嵌入式 SEO 清单。
	SiteSEO []byte
}

// SetWebRouter 注册嵌入式前端资源、SEO 文档与 SPA 回退路由。
// router 接收路由，assets 提供前端构建产物，pluginDispatcher 在静态资源前处理插件协议。
// 该函数保留 API 与静态资源缺失时的 JSON 404，并为未知页面返回带 SPA 外壳的 HTTP 404。
func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	manifest := loadSiteSEOManifest(assets)
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")
	compress := gzip.Gzip(gzip.DefaultCompression)

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		func(c *gin.Context) {
			// ServeFrontendFiles sends build files already compressed.
			if !frontendFS.Exists("/", c.Request.URL.Path) {
				compress(c)
			}
		},
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(frontendFS),
		middleware.Cache(),
		serveWebSEOFiles(manifest),
		serveKnownWebRoutes(manifest, assets.IndexPage),
		middleware.ServeFrontendFiles(frontendFS),
		func(c *gin.Context) {
			requestPath := c.Request.URL.Path
			if strings.HasPrefix(requestPath, "/v1") || strings.HasPrefix(requestPath, "/api") || strings.HasPrefix(requestPath, "/assets") {
				controller.RelayNotFound(c)
				return
			}
			if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
				controller.RelayNotFound(c)
				return
			}

			serveRenderedWebIndex(c, manifest, assets.IndexPage, http.StatusNotFound)
		},
	)
}
