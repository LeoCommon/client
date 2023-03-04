package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Global variable
var zapLog *zap.Logger

func Init(debug bool) {
	var config zap.Config
	var encoderConf zapcore.EncoderConfig

	if debug {
		config = zap.NewDevelopmentConfig()
		encoderConf = zap.NewDevelopmentEncoderConfig()

		// Use a human readable time
		encoderConf.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		config = zap.NewProductionConfig()
		encoderConf = zap.NewProductionEncoderConfig()

		// Use unix timestamp millis for production
		encoderConf.EncodeTime = zapcore.EpochMillisTimeEncoder

		// todo: should we disable the stack traces?
		// encoderConf.StacktraceKey = ""
	}

	// Assign the config
	config.EncoderConfig = encoderConf

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

func Warn(message string, fields ...zap.Field) {
	zapLog.Warn(message, fields...)
}

func Error(message string, fields ...zap.Field) {
	zapLog.Error(message, fields...)
}

func Fatal(message string, fields ...zap.Field) {
	zapLog.Fatal(message, fields...)
}

func Panic(message string, fields ...zap.Field) {
	zapLog.Panic(message, fields...)
}
