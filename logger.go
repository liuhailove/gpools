package gpools

// Logger 用于日志格式化输出
type Logger interface {
	// Printf 需要和log.Printf保持相同语义
	Printf(format string, args ...interface{})
}
