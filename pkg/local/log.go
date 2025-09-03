package local

import (
	k3dLogger "github.com/k3d-io/k3d/v5/pkg/logger"
	"github.com/sirupsen/logrus"
)

func SetupK3dLogger(logger *logrus.Logger) {
	// k3d use logger.Logger as its logger,
	// but also set hooks and print messages with default logrus.Logger this is done by function called 'initLogging' and called on initialization of k3s root command
	// so we re assigned it
	k3dLogger.Logger = logger
}
