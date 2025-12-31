package log

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

var stdoutLogger = (*logrus.Logger)(nil)
var VerboseLogger = (*logrus.Logger)(nil)
var VerboseLoggerOutputs = (*LoggerWriters)(nil)

func Printf(format string, args ...interface{}) {
	stdoutLogger.Printf(format, args...)
	VerboseLogger.Printf(format, args...)
}

func Println(args ...interface{}) {
	stdoutLogger.Println(args...)
	VerboseLogger.Println(args...)
}

func Info(args ...interface{}) {
	stdoutLogger.Info(args...)
	VerboseLogger.Info(args...)
}

func Infof(format string, args ...interface{}) {
	stdoutLogger.Infof(format, args...)
	VerboseLogger.Infof(format, args...)
}

func Warn(args ...interface{}) {
	stdoutLogger.Warn(args...)
	VerboseLogger.Warn(args...)
}

func Warnf(format string, args ...interface{}) {
	stdoutLogger.Warnf(format, args...)
	VerboseLogger.Warnf(format, args...)
}

func Error(args ...interface{}) {
	stdoutLogger.Error(args...)
	VerboseLogger.Error(args...)
}

func Errorf(format string, args ...interface{}) {
	stdoutLogger.Errorf(format, args...)
	VerboseLogger.Errorf(format, args...)
}

func Fatalln(args ...interface{}) {
	stdoutLogger.Fatalln(args...)
	VerboseLogger.Fatalln(args...)
}

func Fatalf(format string, args ...interface{}) {
	stdoutLogger.Fatalf(format, args...)
	VerboseLogger.Fatalf(format, args...)
}

func GetLevel() logrus.Level {
	if os.Getenv("VERBOSE") == "true" {
		return logrus.DebugLevel
	}
	return logrus.InfoLevel
}

func init() {

	textFormat := &logrus.TextFormatter{
		DisableTimestamp: true,
		ForceColors:      true,
		ForceQuote:       true,
	}
	stdoutLogger = logrus.New()
	stdoutLogger.SetFormatter(textFormat)

	VerboseLogger = logrus.New()
	VerboseLogger.SetFormatter(&logrus.JSONFormatter{})
	VerboseLogger.SetLevel(logrus.DebugLevel)
	VerboseLoggerOutputs = NewLoggerOutputs(VerboseLogger)

	if verbose := os.Getenv("VERBOSE") == "true"; verbose {
		VerboseLogger.SetFormatter(textFormat)
		VerboseLoggerOutputs.Add(os.Stdout)
		stdoutLogger.SetOutput(io.Discard)
	} else {
		VerboseLoggerOutputs.Set()
	}
}
