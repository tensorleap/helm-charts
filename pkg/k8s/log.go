package k8s

import (
	"github.com/sirupsen/logrus"
	"k8s.io/klog/v2"
)

func SetupLogger(logger *logrus.Logger) {
	// Setting kubernetes logs by setting writeKlogBuffer
	klog.SetLoggerWithOptions(klog.NewKlogr(), klog.WriteKlogBuffer(func(b []byte) {
		_, err := logger.Writer().Write(b)
		if err != nil {
			logger.Error(err)
		}
	}))
}
