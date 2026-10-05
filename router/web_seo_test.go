package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSEOIndex = `<!doctype html><html><head><!--site-seo-head:start--><title>Static default title</title><meta name="description" content="static description"><!--site-seo-head:end--></head><body><!--site-seo-content--><div id="root"></div></body></html>`

// newSEOWebRouter 创建仅包含 Web 回退链的测试路由。
func newSEOWebRouter(t *testing.T, manifest string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetWebRouter(engine, WebAssets{
		IndexPage: []byte(testSEOIndex),
		SiteSEO:   []byte(manifest),
	}, func(c *gin.Context) { c.Next() })
	return engine
}

// performSEORequest 发起带指定 Host 的 Web 请求。
func performSEORequest(engine http.Handler, path, host string) *httptest.ResponseRecorder {
	return performSEORequestMethod(engine, http.MethodGet, path, host)
}

// performSEORequestMethod 发起指定方法和 Host 的 Web 请求。
func performSEORequestMethod(engine http.Handler, method, path, host string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	request.Host = host
	engine.ServeHTTP(recorder, request)
	return recorder
}

// withSEOOption 临时设置全局站点选项并在测试结束后恢复。
func withSEOOption(t *testing.T, key, value string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous, exists := common.OptionMap[key]
	common.OptionMap[key] = value
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if exists {
			common.OptionMap[key] = previous
			return
		}
		delete(common.OptionMap, key)
	})
}

func TestWebSEOFilesUseCanonicalOriginAndPublicPricingGuard(t *testing.T) {
	withSEOOption(t, "HeaderNavModules", `{"pricing":{"enabled":false,"requireAuth":false}}`)
	engine := newSEOWebRouter(t, `{
		"origin":"https://attacker.invalid",
		"siteName":"ManSuiAI",
		"description":"site",
		"image":"/mansui-social.png",
		"pages":{"/":{"title":"home"},"/about":{"title":"about"},"/pricing":{"title":"pricing"}}
	}`)

	robots := performSEORequest(engine, "/robots.txt", "evil.example")
	require.Equal(t, http.StatusOK, robots.Code)
	assert.Equal(t, "text/plain; charset=utf-8", robots.Header().Get("Content-Type"))
	assert.Contains(t, robots.Header().Get("Cache-Control"), "no-cache")
	assert.Contains(t, robots.Body.String(), "Sitemap: https://api.lolicon.beer/sitemap.xml")
	assert.NotContains(t, robots.Body.String(), "evil.example")

	sitemap := performSEORequest(engine, "/sitemap.xml", "evil.example")
	require.Equal(t, http.StatusOK, sitemap.Code)
	assert.Equal(t, "application/xml; charset=utf-8", sitemap.Header().Get("Content-Type"))
	assert.Contains(t, sitemap.Body.String(), "<loc>https://api.lolicon.beer/</loc>")
	assert.Contains(t, sitemap.Body.String(), "<loc>https://api.lolicon.beer/about</loc>")
	assert.NotContains(t, sitemap.Body.String(), "/pricing</loc>")
	assert.NotContains(t, sitemap.Body.String(), "attacker.invalid")
}

func TestSEOEndpointsAndPagesReturnEmptyHEADBodies(t *testing.T) {
	engine := newSEOWebRouter(t, `{"siteName":"ManSuiAI","pages":{"/":{"title":"home"},"/about":{"title":"about"}}}`)
	for _, path := range []string{"/robots.txt", "/sitemap.xml", "/", "/about", "/404", "/missing"} {
		response := performSEORequestMethod(engine, http.MethodHead, path, "api.lolicon.beer")
		assert.Empty(t, response.Body.String(), path)
		if path == "/404" || path == "/missing" {
			assert.Equal(t, http.StatusNotFound, response.Code, path)
			continue
		}
		assert.Equal(t, http.StatusOK, response.Code, path)
	}
}

func TestKnownWebRoutePrecedesRealStaticIndexAndRedirectsIndexHTML(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "index.html"), []byte("static index bypassed"), 0o600))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manifest := defaultSiteSEOManifest()
	engine.NoRoute(
		serveKnownWebRoutes(manifest, []byte(testSEOIndex)),
		static.Serve("/", static.LocalFile(directory, false)),
		func(c *gin.Context) { c.String(http.StatusTeapot, "fallback") },
	)

	root := performSEORequest(engine, "/", "api.lolicon.beer")
	require.Equal(t, http.StatusOK, root.Code)
	assert.Contains(t, root.Body.String(), defaultSiteTitle)
	assert.NotContains(t, root.Body.String(), "static index bypassed")

	index := performSEORequest(engine, "/index.html", "api.lolicon.beer")
	require.Equal(t, http.StatusMovedPermanently, index.Code)
	assert.Equal(t, "/", index.Header().Get("Location"))
	assert.Equal(t, "noindex, nofollow", index.Header().Get("X-Robots-Tag"))
	assert.Empty(t, index.Body.String())

	indexHead := performSEORequestMethod(engine, http.MethodHead, "/index.html", "api.lolicon.beer")
	require.Equal(t, http.StatusMovedPermanently, indexHead.Code)
	assert.Equal(t, "/", indexHead.Header().Get("Location"))
	assert.Empty(t, indexHead.Body.String())
}

func TestPublicWebRouteInjectsEscapedMetadataAndSemanticContent(t *testing.T) {
	withSEOOption(t, "HomePageContent", "")
	engine := newSEOWebRouter(t, `{
		"origin":"https://ignored.invalid",
		"siteName":"ManSuiAI",
		"description":"site description",
		"image":"/mansui-social.png",
		"pages":{"/":{
			"title":"ManSuiAI - AI 聚合平台",
			"description":"聚合 <模型> & API",
			"heading":"可信 <标题>",
			"paragraphs":["正文 <script>alert(1)</script>"],
			"links":[{"href":"/about?x=1&y=2","label":"了解 <我们>"}]
		}}
	}`)

	response := performSEORequest(engine, "/?campaign=test", "preview.local")
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "<title>ManSuiAI - AI 聚合平台</title>")
	assert.Contains(t, body, `<link rel="canonical" href="https://api.lolicon.beer/" />`)
	assert.Contains(t, body, `<meta property="og:image" content="https://api.lolicon.beer/mansui-social.png" />`)
	assert.Contains(t, body, `<section id="public-seo-content"`)
	assert.Contains(t, body, "可信 &lt;标题&gt;")
	assert.Contains(t, body, "正文 &lt;script&gt;alert(1)&lt;/script&gt;")
	assert.Contains(t, body, `href="/about?x=1&amp;y=2"`)
	assert.NotContains(t, body, "preview.local")
	assert.NotContains(t, body, "Static default title")
	assert.Empty(t, response.Header().Get("X-Robots-Tag"))
	assert.Equal(t, 1, strings.Count(body, `<title>`))
	assert.Contains(t, body, `<script id="site-jsonld" type="application/ld+json">`)
}

func TestSEOIndexSupportsSingularAndMissingMarkerFallbacks(t *testing.T) {
	page := resolveSEOPage(defaultSiteSEOManifest(), "/", true)
	singular := renderSEOIndex([]byte(`<!doctype html><html><head><!--site-seo-head--></head><body><!--site-seo-content--><div id="root"></div></body></html>`), page)
	assert.Equal(t, 1, strings.Count(string(singular), "<title>"))
	assert.Contains(t, string(singular), `<section id="public-seo-content"`)

	withoutMarkers := renderSEOIndex([]byte(`<!doctype html><html><head><title>Legacy title</title><meta name="description" content="legacy"><meta property="og:title" content="legacy"><script type="application/ld+json">{"name":"legacy"}</script></head><body><div id="root"></div></body></html>`), page)
	output := string(withoutMarkers)
	assert.Equal(t, 1, strings.Count(output, "<title>"))
	assert.NotContains(t, output, "Legacy title")
	assert.NotContains(t, output, `content="legacy"`)
	assert.NotContains(t, output, `"name":"legacy"`)
	assert.Contains(t, output, `<script id="site-jsonld" type="application/ld+json">`)
	assert.Less(t, strings.Index(output, publicSEOContentID), strings.Index(output, `id="root"`))
}

func TestPublicWebRoutesUseDistinctMetadataAndCanonicalURLs(t *testing.T) {
	withSEOOption(t, "HeaderNavModules", `{"pricing":{"enabled":true,"requireAuth":false}}`)
	engine := newSEOWebRouter(t, `{
		"siteName":"ManSuiAI",
		"description":"site",
		"pages":{
			"/about":{"title":"About ManSuiAI","description":"About description","heading":"About"},
			"/pricing":{"title":"ManSuiAI Pricing","description":"Pricing description","heading":"Pricing"}
		}
	}`)

	about := performSEORequest(engine, "/about/?source=test", "preview.local")
	require.Equal(t, http.StatusOK, about.Code)
	aboutBody := about.Body.String()
	assert.Contains(t, aboutBody, "<title>About ManSuiAI</title>")
	assert.Contains(t, aboutBody, `content="About description"`)
	assert.Contains(t, aboutBody, `href="https://api.lolicon.beer/about"`)
	assert.Contains(t, aboutBody, `property="og:url" content="https://api.lolicon.beer/about"`)
	assert.Contains(t, aboutBody, `name="twitter:title" content="About ManSuiAI"`)
	assert.Contains(t, aboutBody, `"@type":"AboutPage"`)

	pricing := performSEORequest(engine, "/pricing", "preview.local")
	require.Equal(t, http.StatusOK, pricing.Code)
	pricingBody := pricing.Body.String()
	assert.Contains(t, pricingBody, "<title>ManSuiAI Pricing</title>")
	assert.Contains(t, pricingBody, `content="Pricing description"`)
	assert.Contains(t, pricingBody, `href="https://api.lolicon.beer/pricing"`)
	assert.Contains(t, pricingBody, `property="og:url" content="https://api.lolicon.beer/pricing"`)
	assert.Contains(t, pricingBody, `name="twitter:title" content="ManSuiAI Pricing"`)
	assert.Contains(t, pricingBody, `"@type":"WebPage"`)
	assert.NotEqual(t, aboutBody, pricingBody)
}

func TestPrivateUnknownAndValidDeepWebRoutes(t *testing.T) {
	engine := newSEOWebRouter(t, `{"siteName":"ManSuiAI","pages":{"/":{"title":"home"},"/sign-in":{"title":"must stay private"}}}`)

	privatePage := performSEORequest(engine, "/sign-in", "api.lolicon.beer")
	require.Equal(t, http.StatusOK, privatePage.Code)
	assert.Equal(t, "noindex, nofollow", privatePage.Header().Get("X-Robots-Tag"))
	assert.Contains(t, privatePage.Body.String(), `<meta name="robots" content="noindex, nofollow" />`)
	assert.Contains(t, privatePage.Body.String(), "<title>"+defaultSiteTitle+"</title>")
	assert.NotContains(t, privatePage.Body.String(), `rel="canonical"`)
	assert.NotContains(t, privatePage.Body.String(), "must stay private")

	sectionPage := performSEORequest(engine, "/system-settings/security", "api.lolicon.beer")
	require.Equal(t, http.StatusOK, sectionPage.Code)
	assert.Equal(t, "noindex, nofollow", sectionPage.Header().Get("X-Robots-Tag"))

	deepPage := performSEORequest(engine, "/system-settings/security/rate-limit", "api.lolicon.beer")
	require.Equal(t, http.StatusOK, deepPage.Code)
	assert.Equal(t, "noindex, nofollow", deepPage.Header().Get("X-Robots-Tag"))

	legacyPage := performSEORequest(engine, "/console/models", "api.lolicon.beer")
	require.Equal(t, http.StatusOK, legacyPage.Code)

	unknownPage := performSEORequest(engine, "/this-route-does-not-exist", "api.lolicon.beer")
	require.Equal(t, http.StatusNotFound, unknownPage.Code)
	assert.Equal(t, "noindex, nofollow", unknownPage.Header().Get("X-Robots-Tag"))
	assert.Contains(t, unknownPage.Body.String(), `<div id="root"></div>`)

	explicitNotFound := performSEORequest(engine, "/404", "api.lolicon.beer")
	require.Equal(t, http.StatusNotFound, explicitNotFound.Code)
	assert.Equal(t, "noindex, nofollow", explicitNotFound.Header().Get("X-Robots-Tag"))
}

func TestCustomPublicContentKeepsNeutralSEOAndFallbackBody(t *testing.T) {
	manifest := `{
		"siteName":"ManSuiAI",
		"description":"Neutral site description",
		"pages":{
			"/":{"title":"Default marketing claim","description":"Default claim","heading":"Default heading","paragraphs":["Default body"]},
			"/about":{"title":"Default about claim","description":"Default about description","heading":"Default about heading","paragraphs":["Default about body"]}
		}
	}`
	for _, testCase := range []struct {
		name      string
		optionKey string
		routePath string
	}{
		{name: "home", optionKey: "HomePageContent", routePath: "/"},
		{name: "about", optionKey: "About", routePath: "/about"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			withSEOOption(t, testCase.optionKey, "https://custom.example/content")
			engine := newSEOWebRouter(t, manifest)

			response := performSEORequest(engine, testCase.routePath, "api.lolicon.beer")
			require.Equal(t, http.StatusOK, response.Code)
			body := response.Body.String()
			assert.Contains(t, body, "<title>"+defaultSiteTitle+"</title>")
			assert.Contains(t, body, "Neutral site description")
			assert.NotContains(t, body, "Default")
			assert.Contains(t, body, `<section id="public-seo-content"`)
			assert.Contains(t, body, "<h1>ManSuiAI</h1>")
			assert.Contains(t, body, "<p>Neutral site description</p>")
			assert.Contains(t, body, `<nav aria-label="相关页面">`)
			assert.Contains(t, body, `href="/"`)
			assert.Contains(t, body, `href="/about"`)
		})
	}
}
