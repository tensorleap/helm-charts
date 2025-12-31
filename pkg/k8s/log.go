package k8s

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
)

type nopSink struct{}

func (nopSink) Init(logr.RuntimeInfo)                  {}
func (nopSink) Enabled(int) bool                       { return false }
func (nopSink) Info(int, string, ...interface{})       {}
func (nopSink) Error(error, string, ...interface{})    {}
func (nopSink) WithValues(...interface{}) logr.LogSink { return nopSink{} }
func (nopSink) WithName(string) logr.LogSink           { return nopSink{} }
func (nopSink) WithCallDepth(int) logr.LogSink         { return nopSink{} }

func SetupLogger(logger *logrus.Logger) {
	klog.SetLogger(logr.New(nopSink{}))

	utilruntime.ErrorHandlers = []utilruntime.ErrorHandler{
		func(ctx context.Context, err error, msg string, keysAndValues ...interface{}) {
			logger.Error(err)
			logger.Errorf(msg, keysAndValues...)
		},
	}
}
