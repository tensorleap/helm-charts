package log

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
)

func ConnectFileToVerboseLogOutput(filePath string) (close func(), err error) {

	// 0664 so other members of the shared group can read it; applied
	// explicitly because OpenFile masks the mode with the umask.
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o664)
	if err != nil {
		err = fmt.Errorf("Error opening log file: %s", err)
		return
	}
	_ = file.Chmod(0o664) // best effort: refused when another user owns the file

	VerboseLoggerOutputs.Add(file)

	close = func() {
		VerboseLoggerOutputs.Remove(file)
		file.Close()
	}
	return
}

func NewSpinner(message string) (start func(), stop func(), s *spinner.Spinner) {
	s = spinner.New(spinner.CharSets[33], 500*time.Millisecond)
	s.Suffix = fmt.Sprintf(" %s", message)
	start = s.Start

	stop = func() {
		s.Stop()
		Info(message)
	}

	return
}
