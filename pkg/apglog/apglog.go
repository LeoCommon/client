package apglog

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var zapLog *zap.Logger

func init() {
	// #todo switch to production config on production build
	config := zap.NewDevelopmentConfig()
	enccoderConfig := zap.NewDevelopmentEncoderConfig()
	config.EncoderConfig = enccoderConfig
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	enccoderConfig.StacktraceKey = "" // Disable stack traces

	// Build the logger and skip one caller as thats our own log package
	var err error
	zapLog, err = config.Build(zap.AddCallerSkip(1))

	// Panic if we cant log correctly
	if err != nil {
		panic(err)
	}
}

func Debug(message string, fields ...zap.Field) {
	zapLog.Debug(message, fields...)
}

func Info(message string, fields ...zap.Field) {
	zapLog.Info(message, fields...)
}

func Error(message string, fields ...zap.Field) {
	zapLog.Error(message, fields...)
}

func Fatal(message string, fields ...zap.Field) {
	zapLog.Fatal(message, fields...)
}
