package router

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// registerResponsesRoute 为规范的 /v1/responses 路由注册根路径别名；两者执行同一鉴权、分流和处理链。
// canonicalPath 必须是既有 Responses 路径，handlers 不包含 Engine 已安装的全局中间件。
func registerResponsesRoute(router *gin.Engine, method, canonicalPath string, handlers ...gin.HandlerFunc) {
	router.Handle(method, canonicalPath, handlers...)
	aliasHandlers := make([]gin.HandlerFunc, 0, len(handlers)+1)
	aliasHandlers = append(aliasHandlers, normalizeResponsesAlias)
	aliasHandlers = append(aliasHandlers, handlers...)
	router.Handle(method, strings.TrimPrefix(canonicalPath, "/v1"), aliasHandlers...)
}

// normalizeResponsesAlias 在鉴权与插件查找前补齐规范路径，保留请求方法、参数编码、请求体和已匹配的路径参数。
// 仅挂载到已注册的根路径别名，不重新派发请求，避免 POST 重定向或重复执行全局中间件。
func normalizeResponsesAlias(c *gin.Context) {
	c.Request.URL.Path = "/v1" + c.Request.URL.Path
	if c.Request.URL.RawPath != "" {
		c.Request.URL.RawPath = "/v1" + c.Request.URL.RawPath
	}
	c.Request.RequestURI = c.Request.URL.RequestURI()
	c.Next()
}
