package utils

import (
	"fmt"
	"time"
)

func WaitForCondition(condition func() (bool, error), interval, timeout time.Duration) error {
	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timeout after %v", timeout)
		case <-ticker.C:
			ok, err := condition()
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
}
