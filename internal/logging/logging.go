// Package logging 移植自 ArchiSteamFarm.NLog（C# NLog 封装），提供分级日志。
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level 日志级别。
type Level int

const (
	Trace Level = iota
	Debug
	Info
	Warn
	Error
	Fatal
)

func (l Level) String() string {
	switch l {
	case Trace:
		return "TRACE"
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	case Fatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger 简单文件+控制台双写日志器。
type Logger struct {
	mu     sync.Mutex
	file   *os.File
	out    io.Writer
	level  Level
	prefix string
}

// New 创建日志器。logPath 为空时仅输出到控制台。
func New(prefix, logPath string, level Level) (*Logger, error) {
	l := &Logger{out: os.Stdout, level: level, prefix: prefix}
	if logPath != "" {
		dir := filepath.Dir(logPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建日志目录失败: %w", err)
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("打开日志文件失败: %w", err)
		}
		l.file = f
	}
	return l, nil
}

// Close 关闭日志文件句柄。
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Logf 记录指定级别日志。
func (l *Logger) Logf(level Level, format string, args ...any) {
	if level < l.level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %-5s [%s] %s\n", time.Now().Format("01/02 15:04:05.000"), level.String(), l.prefix, msg)
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = io.WriteString(l.out, line)
	if l.file != nil {
		_, _ = io.WriteString(l.file, line)
	}
}

func (l *Logger) Trace(format string, args ...any) { l.Logf(Trace, format, args...) }
func (l *Logger) Debug(format string, args ...any) { l.Logf(Debug, format, args...) }
func (l *Logger) Info(format string, args ...any)  { l.Logf(Info, format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.Logf(Warn, format, args...) }
func (l *Logger) Error(format string, args ...any) { l.Logf(Error, format, args...) }
func (l *Logger) Fatal(format string, args ...any) { l.Logf(Fatal, format, args...) }
