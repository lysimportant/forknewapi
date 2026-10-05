package router

import (
	"bytes"
	"encoding/xml"
	"html/template"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
)

const (
	canonicalSiteOrigin  = "https://api.lolicon.beer"
	defaultSiteTitle     = "ManSuiAI - AI 聚合平台"
	seoHeadBlockStart    = "<!--site-seo-head:start-->"
	seoHeadBlockEnd      = "<!--site-seo-head:end-->"
	seoHeadMarker        = "<!--site-seo-head-->"
	seoContentMarker     = "<!--site-seo-content-->"
	publicSEOContentID   = "public-seo-content"
	structuredDataNodeID = "site-jsonld"
)

// siteSEOManifest 描述前后端共享的站点级 SEO 配置。
type siteSEOManifest struct {
	Origin      string                 `json:"origin"`
	SiteName    string                 `json:"siteName"`
	Description string                 `json:"description"`
	Image       string                 `json:"image"`
	Pages       map[string]siteSEOPage `json:"pages"`
}

// siteSEOPage 描述一个公开页面的搜索元数据和无脚本语义内容。
type siteSEOPage struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Heading     string        `json:"heading"`
	Paragraphs  []string      `json:"paragraphs"`
	Links       []siteSEOLink `json:"links"`
}

// siteSEOLink 描述无脚本语义导航中的站内链接。
type siteSEOLink struct {
	Href  string `json:"href"`
	Label string `json:"label"`
}

// resolvedSEOPage 保存单次请求最终使用的安全 SEO 输出。
type resolvedSEOPage struct {
	SiteName    string
	Title       string
	Description string
	Canonical   string
	Image       string
	Heading     string
	Paragraphs  []string
	Links       []siteSEOLink
	JSONLD      template.JS
	Index       bool
	ShowContent bool
}

// seoHeadTemplate 生成公开页完整元数据或私有页 noindex 标记。
var seoHeadTemplate = template.Must(template.New("site-seo-head").Parse(
	"<title>{{.Title}}</title>\n" +
		"<meta name=\"title\" content=\"{{.Title}}\" />\n" +
		"<meta name=\"description\" content=\"{{.Description}}\" />\n" +
		"{{if .Index}}<meta name=\"robots\" content=\"index, follow\" />\n" +
		"<link rel=\"canonical\" href=\"{{.Canonical}}\" />\n" +
		"<meta property=\"og:type\" content=\"website\" />\n" +
		"<meta property=\"og:site_name\" content=\"{{.SiteName}}\" />\n" +
		"<meta property=\"og:title\" content=\"{{.Title}}\" />\n" +
		"<meta property=\"og:description\" content=\"{{.Description}}\" />\n" +
		"<meta property=\"og:url\" content=\"{{.Canonical}}\" />\n" +
		"<meta property=\"og:image\" content=\"{{.Image}}\" />\n" +
		"<meta name=\"twitter:card\" content=\"summary_large_image\" />\n" +
		"<meta name=\"twitter:title\" content=\"{{.Title}}\" />\n" +
		"<meta name=\"twitter:description\" content=\"{{.Description}}\" />\n" +
		"<meta name=\"twitter:image\" content=\"{{.Image}}\" />\n" +
		"<script id=\"site-jsonld\" type=\"application/ld+json\">{{.JSONLD}}</script>" +
		"{{else}}<meta name=\"robots\" content=\"noindex, nofollow\" />{{end}}",
))

// seoContentTemplate 生成 React 根节点之外的可访问语义内容。
var seoContentTemplate = template.Must(template.New("site-seo-content").Parse(
	"{{if .ShowContent}}<section id=\"public-seo-content\" aria-label=\"{{.Heading}}\">\n" +
		"  <h1>{{.Heading}}</h1>\n" +
		"  {{range .Paragraphs}}<p>{{.}}</p>{{end}}\n" +
		"  {{if .Links}}<nav aria-label=\"相关页面\">{{range .Links}}<a href=\"{{.Href}}\">{{.Label}}</a>{{end}}</nav>{{end}}\n" +
		"</section>{{end}}",
))

// loadSiteSEOManifest 从嵌入资源读取前后端共享的站点元数据，并补齐安全默认值。
func loadSiteSEOManifest(assets WebAssets) siteSEOManifest {
	manifest := defaultSiteSEOManifest()
	data := assets.SiteSEO
	if len(data) == 0 {
		data, _ = assets.BuildFS.ReadFile("web/dist/site-seo.json")
	}
	if len(data) == 0 {
		return manifest
	}

	var configured siteSEOManifest
	if err := common.Unmarshal(data, &configured); err != nil {
		common.SysError("read embedded site-seo.json: " + err.Error())
		return manifest
	}
	if strings.TrimSpace(configured.SiteName) != "" {
		manifest.SiteName = configured.SiteName
	}
	if strings.TrimSpace(configured.Description) != "" {
		manifest.Description = configured.Description
	}
	if strings.TrimSpace(configured.Image) != "" {
		manifest.Image = configured.Image
	}
	for routePath, page := range configured.Pages {
		manifest.Pages[normalizeWebPath(routePath)] = page
	}
	// 规范域名属于部署契约，不能由请求 Host 或错误配置改写。
	manifest.Origin = canonicalSiteOrigin
	return manifest
}

// defaultSiteSEOManifest 返回清单缺失或无效时的保守默认配置。
func defaultSiteSEOManifest() siteSEOManifest {
	return siteSEOManifest{
		Origin:      canonicalSiteOrigin,
		SiteName:    "ManSuiAI",
		Description: "ManSuiAI 提供统一的 AI 模型聚合与 API 服务。",
		Image:       "/mansui-social.png",
		Pages: map[string]siteSEOPage{
			"/":        {Title: defaultSiteTitle, Description: "通过 ManSuiAI 统一访问多种 AI 模型与 API 服务。", Heading: defaultSiteTitle},
			"/about":   {Title: "关于 ManSuiAI - AI 聚合平台", Description: "了解 ManSuiAI AI 聚合平台及其统一 API 服务。", Heading: "关于 ManSuiAI"},
			"/pricing": {Title: "ManSuiAI 模型价格 - AI 聚合平台", Description: "查看 ManSuiAI 支持的 AI 模型与公开价格信息。", Heading: "ManSuiAI 模型价格"},
		},
	}
}

// sitemapURLSet 表示 sitemap.xml 的根节点。
type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// sitemapURL 表示 sitemap.xml 中的一个公开页面地址。
type sitemapURL struct {
	Location string `xml:"loc"`
}

// serveWebSEOFiles 提供规范域名固定的 robots.txt 与动态公开页面 sitemap。
func serveWebSEOFiles(manifest siteSEOManifest) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		switch c.Request.URL.Path {
		case "/robots.txt":
			c.Header("Cache-Control", "no-cache")
			writeSEOResponse(c, http.StatusOK, "text/plain; charset=utf-8", []byte("User-agent: *\nAllow: /\nDisallow: /api/\nDisallow: /v1/\nDisallow: /dashboard/\nDisallow: /system-settings/\nSitemap: "+canonicalSiteOrigin+"/sitemap.xml\n"))
		case "/sitemap.xml":
			paths := []string{"/", "/about"}
			if isPricingSEOEnabled() {
				paths = append(paths, "/pricing")
			}
			urls := make([]sitemapURL, 0, len(paths))
			for _, routePath := range paths {
				if _, exists := manifest.Pages[routePath]; exists {
					urls = append(urls, sitemapURL{Location: canonicalSiteOrigin + routePath})
				}
			}
			body, err := xml.MarshalIndent(sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls}, "", "  ")
			if err != nil {
				common.SysError("render sitemap.xml: " + err.Error())
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			c.Header("Cache-Control", "no-cache")
			writeSEOResponse(c, http.StatusOK, "application/xml; charset=utf-8", append([]byte(xml.Header), body...))
		default:
			c.Next()
		}
	}
}

// serveKnownWebRoutes 在静态文件处理前渲染有效 SPA 页面，避免入口文件绕过动态 SEO。
func serveKnownWebRoutes(manifest siteSEOManifest, indexPage []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		if requestPath == "/index.html" {
			if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
				c.Next()
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.Header("X-Robots-Tag", "noindex, nofollow")
			c.Header("Location", "/")
			c.Status(http.StatusMovedPermanently)
			c.Abort()
			return
		}
		if !isKnownWebRoute(requestPath) {
			c.Next()
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		status := http.StatusOK
		if normalizeWebPath(requestPath) == "/404" {
			status = http.StatusNotFound
		}
		serveRenderedWebIndex(c, manifest, indexPage, status)
	}
}

// serveRenderedWebIndex 写入动态 SEO 的 SPA 外壳，并为私有或错误页设置禁止收录响应头。
func serveRenderedWebIndex(c *gin.Context, manifest siteSEOManifest, indexPage []byte, status int) {
	page := resolveSEOPage(manifest, c.Request.URL.Path, status == http.StatusOK)
	c.Header("Cache-Control", "no-cache")
	if !page.Index {
		c.Header("X-Robots-Tag", "noindex, nofollow")
	}
	writeSEOResponse(c, status, "text/html; charset=utf-8", renderSEOIndex(indexPage, page))
}

// writeSEOResponse 保持 GET 与 HEAD 的状态和内容类型一致，同时保证 HEAD 不写响应体。
func writeSEOResponse(c *gin.Context, status int, contentType string, body []byte) {
	c.Header("Content-Type", contentType)
	if c.Request.Method == http.MethodHead {
		c.Status(status)
		c.Writer.WriteHeaderNow()
		c.Abort()
		return
	}
	c.Data(status, contentType, body)
	c.Abort()
}

// resolveSEOPage 根据请求路径、公开守卫和自定义内容计算最终页面元数据。
func resolveSEOPage(manifest siteSEOManifest, requestPath string, knownRoute bool) resolvedSEOPage {
	routePath := normalizeWebPath(requestPath)
	page := manifest.Pages[routePath]
	publicPage := isPublicSEOPath(routePath)
	if routePath == "/pricing" && !isPricingSEOEnabled() {
		publicPage = false
	}
	if !knownRoute {
		publicPage = false
	}

	resolved := resolvedSEOPage{
		SiteName:    manifest.SiteName,
		Title:       defaultSiteTitle,
		Description: manifest.Description,
		Image:       absoluteSiteURL(manifest.Image),
		Index:       publicPage,
	}
	if !publicPage {
		return resolved
	}

	customContent := hasCustomPublicContent(routePath)
	if customContent {
		resolved.Heading = manifest.SiteName
		resolved.Paragraphs = []string{manifest.Description}
		resolved.Links = []siteSEOLink{
			{Href: "/", Label: "首页"},
			{Href: "/about", Label: "关于"},
		}
		if isPricingSEOEnabled() {
			resolved.Links = append(resolved.Links, siteSEOLink{Href: "/pricing", Label: "模型与价格"})
		}
		resolved.ShowContent = true
	} else {
		resolved.Title = fallbackString(page.Title, manifest.SiteName)
		resolved.Description = fallbackString(page.Description, manifest.Description)
		resolved.Heading = page.Heading
		resolved.Paragraphs = page.Paragraphs
		resolved.Links = page.Links
		resolved.ShowContent = strings.TrimSpace(page.Heading) != ""
	}
	resolved.Canonical = canonicalSiteOrigin + canonicalRoutePath(routePath)
	jsonLD, err := common.Marshal(map[string]any{
		"@context":    "https://schema.org",
		"@type":       schemaTypeForPath(routePath),
		"name":        resolved.Title,
		"description": resolved.Description,
		"url":         resolved.Canonical,
		"image":       resolved.Image,
		"isPartOf": map[string]any{
			"@type": "WebSite",
			"name":  manifest.SiteName,
			"url":   canonicalSiteOrigin + "/",
		},
	})
	if err == nil {
		resolved.JSONLD = template.JS(jsonLD)
	}
	return resolved
}

// renderSEOIndex 通过显式模板标记向前端入口页注入已转义内容。
func renderSEOIndex(indexPage []byte, page resolvedSEOPage) []byte {
	if len(indexPage) == 0 {
		return indexPage
	}
	var head bytes.Buffer
	if err := seoHeadTemplate.Execute(&head, page); err != nil {
		common.SysError("render SEO head: " + err.Error())
		return indexPage
	}
	var content bytes.Buffer
	if err := seoContentTemplate.Execute(&content, page); err != nil {
		common.SysError("render SEO content: " + err.Error())
		return indexPage
	}

	rendered, headReplaced := replaceSEOMarkerBlock(indexPage, []byte(seoHeadBlockStart), []byte(seoHeadBlockEnd), head.Bytes())
	if !headReplaced && bytes.Contains(rendered, []byte(seoHeadMarker)) {
		rendered = bytes.Replace(rendered, []byte(seoHeadMarker), head.Bytes(), 1)
		headReplaced = true
	}
	contentReplaced := false
	if bytes.Contains(rendered, []byte(seoContentMarker)) {
		rendered = bytes.Replace(rendered, []byte(seoContentMarker), content.Bytes(), 1)
		contentReplaced = true
	}
	if headReplaced && contentReplaced {
		return rendered
	}
	lowerDocument := bytes.ToLower(rendered)
	if !bytes.Contains(lowerDocument, []byte("<html")) && !bytes.Contains(lowerDocument, []byte("<head")) && !bytes.Contains(lowerDocument, []byte("<body")) {
		return rendered
	}

	parsed, err := renderSEOIndexDOM(rendered, head.Bytes(), content.Bytes(), headReplaced, contentReplaced)
	if err != nil {
		common.SysError("render SEO fallback document: " + err.Error())
		return rendered
	}
	return parsed
}

// replaceSEOMarkerBlock 替换含默认静态元数据的完整标记块。
func replaceSEOMarkerBlock(document, startMarker, endMarker, replacement []byte) ([]byte, bool) {
	start := bytes.Index(document, startMarker)
	if start < 0 {
		return document, false
	}
	endOffset := bytes.Index(document[start+len(startMarker):], endMarker)
	if endOffset < 0 {
		return document, false
	}
	end := start + len(startMarker) + endOffset + len(endMarker)
	replaced := make([]byte, 0, len(document)-(end-start)+len(replacement))
	replaced = append(replaced, document[:start]...)
	replaced = append(replaced, replacement...)
	replaced = append(replaced, document[end:]...)
	return replaced, true
}

// renderSEOIndexDOM 在标记缺失时解析 HTML，移除旧 SEO 节点并安全插入动态内容。
func renderSEOIndexDOM(document, headContent, bodyContent []byte, headReplaced, contentReplaced bool) ([]byte, error) {
	doc, err := html.Parse(bytes.NewReader(document))
	if err != nil {
		return nil, err
	}

	if !headReplaced {
		head := findHTMLElement(doc, "head")
		removeStaticSEOHead(head)
		nodes, parseErr := html.ParseFragment(bytes.NewReader(headContent), head)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, node := range nodes {
			head.AppendChild(node)
		}
	}

	if !contentReplaced {
		removeHTMLElementByID(doc, publicSEOContentID)
		if len(bodyContent) > 0 {
			body := findHTMLElement(doc, "body")
			nodes, parseErr := html.ParseFragment(bytes.NewReader(bodyContent), body)
			if parseErr != nil {
				return nil, parseErr
			}
			root := findHTMLElementByID(body, "root")
			for _, node := range nodes {
				if root != nil && root.Parent == body {
					body.InsertBefore(node, root)
					continue
				}
				body.AppendChild(node)
			}
		}
	}

	var output bytes.Buffer
	if err := html.Render(&output, doc); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// findHTMLElement 查找文档中的首个指定元素；标准 HTML 解析会补齐 head 与 body。
func findHTMLElement(node *html.Node, element string) *html.Node {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, element) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLElement(child, element); found != nil {
			return found
		}
	}
	return nil
}

// findHTMLElementByID 查找指定子树中的首个 id 元素。
func findHTMLElementByID(node *html.Node, id string) *html.Node {
	if node.Type == html.ElementNode && htmlAttribute(node, "id") == id {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLElementByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

// removeHTMLElementByID 删除指定 id 的旧服务端内容，避免无标记入口页重复输出。
func removeHTMLElementByID(node *html.Node, id string) {
	if found := findHTMLElementByID(node, id); found != nil && found.Parent != nil {
		found.Parent.RemoveChild(found)
	}
}

// removeStaticSEOHead 移除入口页中会与动态页面元数据冲突的静态节点。
func removeStaticSEOHead(head *html.Node) {
	for child := head.FirstChild; child != nil; {
		next := child.NextSibling
		if isStaticSEOHeadNode(child) {
			head.RemoveChild(child)
		}
		child = next
	}
}

// isStaticSEOHeadNode 判断 head 子节点是否由服务端动态 SEO 取代。
func isStaticSEOHeadNode(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	switch strings.ToLower(node.Data) {
	case "title":
		return true
	case "meta":
		name := strings.ToLower(htmlAttribute(node, "name"))
		property := strings.ToLower(htmlAttribute(node, "property"))
		return name == "title" || name == "description" || name == "robots" || strings.HasPrefix(name, "twitter:") || strings.HasPrefix(property, "og:")
	case "link":
		for rel := range strings.FieldsSeq(strings.ToLower(htmlAttribute(node, "rel"))) {
			if rel == "canonical" {
				return true
			}
		}
	case "script":
		return htmlAttribute(node, "id") == structuredDataNodeID || strings.EqualFold(htmlAttribute(node, "type"), "application/ld+json")
	}
	return false
}

// htmlAttribute 返回元素属性值，属性名按 HTML 规则忽略大小写。
func htmlAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return attribute.Val
		}
	}
	return ""
}

// isKnownWebRoute 按生成路由树和旧版兼容映射识别有效 SPA 深层路径。
func isKnownWebRoute(requestPath string) bool {
	routePath := normalizeWebPath(requestPath)
	if _, exists := knownStaticWebRoutes[routePath]; exists {
		return true
	}
	if routePath == "/login" || routePath == "/forbidden" || routePath == "/console" || strings.HasPrefix(routePath, "/console/") {
		return true
	}
	segments := strings.Split(strings.Trim(routePath, "/"), "/")
	if len(segments) == 2 && segments[1] != "" {
		switch segments[0] {
		case "chat", "errors", "oauth", "dashboard", "models", "pricing", "usage-logs":
			return true
		case "system-settings":
			return isSystemSettingsSection(segments[1])
		}
	}
	if len(segments) == 3 && segments[2] != "" && segments[0] == "system-settings" {
		return isSystemSettingsSection(segments[1])
	}
	return false
}

// isPublicSEOPath 限制只有明确公开的营销页面可以被搜索引擎收录。
func isPublicSEOPath(routePath string) bool {
	switch routePath {
	case "/", "/about", "/pricing":
		return true
	default:
		return false
	}
}

// isSystemSettingsSection 校验系统设置路由树中的一级分区。
func isSystemSettingsSection(section string) bool {
	switch section {
	case "auth", "billing", "content", "models", "operations", "security", "site":
		return true
	default:
		return false
	}
}

// knownStaticWebRoutes 列出生成路由树中不含动态段的页面路径。
var knownStaticWebRoutes = map[string]struct{}{
	"/": {}, "/401": {}, "/403": {}, "/404": {}, "/500": {}, "/503": {},
	"/about": {}, "/api-service-agreement": {}, "/channels": {}, "/chat2link": {},
	"/dashboard": {}, "/forgot-password": {}, "/keys": {}, "/models": {}, "/oauth": {},
	"/otp": {}, "/playground": {}, "/pricing": {}, "/privacy-policy": {}, "/profile": {},
	"/rankings": {}, "/redemption-codes": {}, "/register": {}, "/reset": {}, "/security": {},
	"/setup": {}, "/sign-in": {}, "/sign-up": {}, "/subscriptions": {}, "/system-info": {},
	"/system-settings": {}, "/task-plugins": {}, "/usage-logs": {}, "/usage-logs/audit": {},
	"/user-agreement": {}, "/user/reset": {}, "/users": {}, "/wallet": {},
}

// isPricingSEOEnabled 仅在价格模块公开可访问时允许搜索收录。
func isPricingSEOEnabled() bool {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["HeaderNavModules"]
	common.OptionMapRWMutex.RUnlock()
	if strings.TrimSpace(raw) == "" {
		return true
	}
	var modules map[string]any
	if err := common.Unmarshal([]byte(raw), &modules); err != nil {
		return true
	}
	pricing, exists := modules["pricing"]
	if !exists {
		return true
	}
	switch value := pricing.(type) {
	case bool:
		return value
	case map[string]any:
		return parseSEOBoolean(value["enabled"], true) && !parseSEOBoolean(value["requireAuth"], false)
	default:
		return parseSEOBoolean(value, true)
	}
}

// parseSEOBoolean 兼容导航配置支持的布尔值表示形式。
func parseSEOBoolean(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		if typed == 1 {
			return true
		}
		if typed == 0 {
			return false
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1":
			return true
		case "false", "0":
			return false
		}
	}
	return fallback
}

// hasCustomPublicContent 判断管理端是否已覆盖首页或关于页内容。
func hasCustomPublicContent(routePath string) bool {
	key := ""
	switch routePath {
	case "/":
		key = "HomePageContent"
	case "/about":
		key = "About"
	default:
		return false
	}
	common.OptionMapRWMutex.RLock()
	configured := strings.TrimSpace(common.OptionMap[key]) != ""
	common.OptionMapRWMutex.RUnlock()
	return configured
}

// normalizeWebPath 将除根路径外的尾部斜杠统一移除。
func normalizeWebPath(routePath string) string {
	if routePath == "" || routePath == "/" {
		return "/"
	}
	return strings.TrimRight(routePath, "/")
}

// canonicalRoutePath 返回规范链接使用的稳定公开路径。
func canonicalRoutePath(routePath string) string {
	if routePath == "/" {
		return "/"
	}
	return strings.TrimRight(routePath, "/")
}

// absoluteSiteURL 只接受规范站点上的图片地址，避免生成第三方 canonical 资源。
func absoluteSiteURL(value string) string {
	if strings.HasPrefix(value, "/") {
		return canonicalSiteOrigin + value
	}
	if strings.HasPrefix(value, canonicalSiteOrigin+"/") {
		return value
	}
	return canonicalSiteOrigin + "/mansui-social.png"
}

// schemaTypeForPath 返回公开页面对应的 Schema.org 类型。
func schemaTypeForPath(routePath string) string {
	switch routePath {
	case "/":
		return "WebSite"
	case "/about":
		return "AboutPage"
	default:
		return "WebPage"
	}
}

// fallbackString 在清单字段为空时返回站点级默认值。
func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
