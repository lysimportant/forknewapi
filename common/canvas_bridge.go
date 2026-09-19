package common

// CanvasBridgeEnabled 判断是否启用 Canvas 只读计费桥接；默认关闭，不改变原有中继入口。
func CanvasBridgeEnabled() bool {
	return GetEnvOrDefaultBool("CANVAS_BRIDGE_ENABLED", false)
}
