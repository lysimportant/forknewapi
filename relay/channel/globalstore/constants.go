package globalstore

// ChannelName is the channel identifier reported through GetChannelName.
var ChannelName = "globalstore"

// ModelList holds the image model ids served by the GlobalStore model center.
//
// The ids mirror the IMAGE_MODELS keys of the working JavaScript implementation
// (plugins/tasks/globalstore/plugin.js) verbatim, including the "globalstore/"
// prefix that keeps the public id from colliding with one of the fork's built-in
// model names. That file also owns the video models, but this native channel
// serves image generation only.
var ModelList = []string{
	"banana2",
	"bananapro",
	"banana_c1",
	"banana_c2",
	"globalstore/gpt-image-2",
	"gpt-image-2.5",
	"seedream-5.0",
	"seedream_5.0Pro",
}

// Aspect ratio sets, grouped exactly as plugin.js groups them; every model
// references one of these lists.
var (
	ratiosI1 = []string{"21:9", "16:9", "3:2", "4:3", "5:4", "1:1", "4:5", "3:4", "2:3", "9:16", "auto", "1:4", "4:1", "1:8", "8:1"}
	ratiosI2 = []string{"21:9", "16:9", "3:2", "4:3", "1:1", "3:4", "2:3", "9:16", "4:1", "1:4"}
	ratiosI3 = []string{"1:1", "16:9", "9:16", "3:4", "4:3", "2:3", "3:2"}
	ratiosI4 = []string{"1:1", "3:4", "4:3", "16:9", "9:16", "3:2", "2:3", "21:9"}
)

// Documented pixel size enumerations, grouped by the resolution tier the vendor
// publishes them under. They are transcribed verbatim from the model center
// reference. The tier names are kept because the vendor documents them, but a
// request this channel emits always carries the exact WxH pixels of that tier,
// never the tier name itself.
var (
	tierSizesB30 = map[string][]string{
		"1k": {"1584x672", "1376x768", "1264x848", "1200x896", "1152x928", "1024x1024", "928x1152", "896x1200", "848x1264", "768x1376"},
		"2k": {"3168x1344", "2752x1536", "2528x1696", "2400x1792", "2304x1856", "2048x2048", "1856x2304", "1792x2400", "1696x2528", "1536x2752"},
		"4k": {"6336x2688", "5504x3072", "5056x3392", "4800x3584", "4608x3712", "4096x4096", "3712x4608", "3584x4800", "3392x5056", "3072x5504"},
	}
	tierSizesC14 = map[string][]string{
		"1k": {"1584x672", "1376x768", "1264x848", "1200x896", "1152x928", "1024x1024", "928x1152", "896x1200", "848x1264", "768x1376", "2880x384", "2048x512", "512x2048", "384x2880"},
	}
	tierSizesG14 = map[string][]string{
		"1k": {"1024x1024", "1280x720", "720x1280", "1152x864", "864x1152", "1248x832", "832x1248"},
		"2k": {"2048x2048", "2816x1584", "1584x2816", "1776x2368", "2368x1776", "1664x2496", "2496x1664"},
	}
)

// tierLongEdges is the long edge a tier name denotes. It is only used to map a
// requested pixel size back onto the documented enumeration or onto the tier name
// of a model whose vendor page publishes no size list at all.
var tierLongEdges = map[string]int{"1k": 1024, "2k": 2048, "3k": 3072, "4k": 3840}

// modelSpec is one IMAGE_MODELS entry of plugin.js: the accepted aspect ratios,
// the documented resolution tiers, the documented pixel size enumeration, the
// reference image ceiling and the accepted qualities.
//
// acceptsSize reports whether the vendor page documents a size field at all, and
// sizeTiers is empty for the one model that documents a size without publishing
// an enumeration. defaultSize is the documented default pixels and defaultRatio
// falls back to 1:1 when it is empty.
type modelSpec struct {
	ratios        []string
	resolutions   []string
	sizeTiers     map[string][]string
	acceptsSize   bool
	defaultSize   string
	maxReferences int
	qualities     []string
	defaultRatio  string
}

// modelSpecs mirrors plugin.js IMAGE_MODELS one-to-one.
var modelSpecs = map[string]modelSpec{
	"banana2":   {ratios: ratiosI1, resolutions: []string{"1k", "2k", "4k"}, sizeTiers: tierSizesB30, acceptsSize: true, defaultSize: "1024x1024", maxReferences: 14},
	"bananapro": {ratios: ratiosI1, resolutions: []string{"1k", "2k", "4K"}, sizeTiers: tierSizesB30, acceptsSize: true, defaultSize: "1024x1024", maxReferences: 14},
	"banana_c1": {ratios: ratiosI2, resolutions: []string{"1K"}, sizeTiers: tierSizesC14, acceptsSize: true, defaultSize: "1024x1024", maxReferences: 14, qualities: []string{"standard"}, defaultRatio: "21:9"},
	// banana_c2 documents no size field, so only a tier name can be sent.
	"banana_c2": {ratios: ratiosI1, resolutions: []string{"1K", "2K", "4K"}, maxReferences: 6},

	"globalstore/gpt-image-2": {ratios: ratiosI3, resolutions: []string{"1K", "2K"}, sizeTiers: tierSizesG14, acceptsSize: true, defaultSize: "2048x2048", maxReferences: 14, qualities: []string{"medium", "high"}},
	// gpt-image-2.5 documents the 1K tier only, so the 1K square is its default.
	"gpt-image-2.5": {ratios: ratiosI3, resolutions: []string{"1K"}, sizeTiers: tierSizesG14, acceptsSize: true, defaultSize: "1024x1024", maxReferences: 14, qualities: []string{"low", "medium", "high"}},

	// seedream-5.0 accepts a size but publishes no enumeration, so an explicit
	// pixel size is forwarded as-is and a tier-only request stays a tier name.
	"seedream-5.0":    {ratios: ratiosI4, resolutions: []string{"2K", "3K", "4K"}, acceptsSize: true, defaultSize: "2048x2048", maxReferences: 10},
	"seedream_5.0Pro": {ratios: ratiosI4, resolutions: []string{"1K", "2K"}, maxReferences: 10},
}

// upstreamModelNames maps a public model id onto the model name the upstream
// expects (plugin.js UPSTREAM_MODEL_NAMES). Ids without an entry are sent as-is.
var upstreamModelNames = map[string]string{
	"globalstore/gpt-image-2": "gpt-image-2",
}
