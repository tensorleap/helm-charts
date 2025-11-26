package k8s

import (
	"os"

	"github.com/sirupsen/logrus"
	"k8s.io/klog/v2"
)

func SetupLogger(logger *logrus.Logger) {
	// Setting kubernetes logs by setting writeKlogBuffer
	// Note: If logger write fails, write to stderr directly to avoid recursive logging errors
	klog.SetLoggerWithOptions(klog.NewKlogr(), klog.WriteKlogBuffer(func(b []byte) {
		_, err := logger.Writer().Write(b)
		if err != nil {
			// Write to stderr directly to avoid recursive klog errors
			// This prevents infinite recursion when logger.Writer().Write() fails
			os.Stderr.Write(b)
		}
	}))
}
