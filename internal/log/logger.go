package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.SugaredLogger

func Init() {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}
	
	l, err := config.Build()
	if err != nil {
		panic(err)
	}
	
	log = l.Sugar()
}

func Sync() {
	if log != nil {
		_ = log.Sync()
	}
}

func Info(msg string, keysAndValues ...interface{}) {
	log.Infow(msg, keysAndValues...)
}

func Error(msg string, keysAndValues ...interface{}) {
	log.Errorw(msg, keysAndValues...)
}

func Fatal(msg string, keysAndValues ...interface{}) {
	log.Fatalw(msg, keysAndValues...)
}

func Debug(msg string, keysAndValues ...interface{}) {
	log.Debugw(msg, keysAndValues...)
}

func Warn(msg string, keysAndValues ...interface{}) {
	log.Warnw(msg, keysAndValues...)
}