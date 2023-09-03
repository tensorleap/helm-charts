package log

import (
	"io"

	"github.com/sirupsen/logrus"
)

type LoggerWriters struct {
	writers []io.Writer
	logger  *logrus.Logger // Assuming the existence of a Logger struct with a VerboseLogger field
}

func NewLoggerOutputs(logger *logrus.Logger) *LoggerWriters {
	return &LoggerWriters{
		writers: []io.Writer{},
		logger:  logger,
	}
}

func (lw *LoggerWriters) Add(w io.Writer) {
	lw.writers = append(lw.writers, w)
	lw.Set()
}

func (lw *LoggerWriters) Remove(w io.Writer) {
	for i, writer := range lw.writers {
		if writer == w {
			// Remove the writer from the slice
			lw.writers = append(lw.writers[:i], lw.writers[i+1:]...)
			break
		}
	}
	lw.Set()
}

func (lw *LoggerWriters) Set() {
	if len(lw.writers) == 0 {
		lw.logger.SetOutput(io.Discard)
		return
	}
	// Create a multi-writer that writes to all the writers in the slice
	lw.logger.SetOutput(io.MultiWriter(lw.writers...))
}
