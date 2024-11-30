package gool

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger() *zap.Logger {
	encoderConfig := zapcore.EncoderConfig{ // 创建编码配置
		TimeKey:        "T",                           // 时间键
		LevelKey:       "L",                           // 日志级别键
		NameKey:        "log",                         // 日志名称键
		CallerKey:      "C",                           // 日志调用键
		MessageKey:     "msg",                         // 日志消息键
		StacktraceKey:  "stacktrace",                  // 堆栈跟踪键
		LineEnding:     zapcore.DefaultLineEnding,     // 行结束符,默认为 \n       EncodeLevel:    zapcore.CapitalLevelEncoder,   // 日志级别编码器,将日志级别转换为大写
		EncodeTime:     zapcore.ISO8601TimeEncoder,    // 时间编码器,将时间格式化为 ISO8601 格式
		EncodeDuration: zapcore.StringDurationEncoder, // 持续时间编码器,将持续时间编码为字符串
		EncodeCaller:   zapcore.ShortCallerEncoder,    // 调用编码器,显示文件名和行号
	}
	encoder := zapcore.NewConsoleEncoder(encoderConfig)                    // 创建控制台编码器,使用编码配置
	atomicLevel := zap.NewAtomicLevel()                                    // 创建原子级别,用于动态设置日志级别
	atomicLevel.SetLevel(zap.InfoLevel)                                    // 设置日志级别,只有 Info 级别及以上的日志才会输出
	core := zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), atomicLevel) // 将日志输出到标准输出
	logger := zap.New(core, zap.AddCaller(), zap.Development())            // 创建 Logger,添加调用者和开发模式
	defer logger.Sync()
	return logger
}
