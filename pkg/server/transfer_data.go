package server

import (
	"context"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

func TransferData(ctx context.Context) (isTransfer bool, err error) {
	previousDataPath := local.GetPreviousServerDataDir()
	currentDataPath := local.GetServerDataDir()

	if previousDataPath == currentDataPath || previousDataPath == "" {
		return false, nil
	}

	previousStatus, err := local.CheckDirectoryStatus(previousDataPath)
	if !previousStatus.Exists {
		return false, nil
	}

	if err != nil {
		msg := fmt.Sprintf("Unable to access the potential previous storage directory (%s). This can be ignored if this is the first installation. Do you want to continue?", previousDataPath)
		isContinue, confirmErr := confirm(msg, true)
		if confirmErr != nil {
			return false, confirmErr
		}
		if !isContinue {
			return false, fmt.Errorf("failed accessing previous storage: %v", err)
		}
		return false, nil
	}

	currentStatus, err := local.CheckDirectoryStatus(currentDataPath)
	if err != nil {
		return false, fmt.Errorf("failed to check current storage directory: %v", err)
	}

	if currentStatus.Exists {
		return false, fmt.Errorf("we can't allow to move tensorleap data to an existing directory consider to use path '%s/tensorleap_data' or delete the directory manually", currentDataPath)
	} else {
		msg := fmt.Sprintf("The storage directory has changed from '%s' to '%s'. The storage will be transferred and server reinstall. Do you want to continue?", previousDataPath, currentDataPath)
		isContinue, confirmErr := confirm(msg, true)
		if confirmErr != nil {
			return false, confirmErr
		}
		if !isContinue {
			return false, fmt.Errorf("not continuing with storage transfer")
		}
	}

	err = Uninstall(ctx, false)
	if err != nil {
		return false, fmt.Errorf("failed to uninstall: %v", err)
	}

	log.Printf("Moving storage from %s to %s", previousDataPath, currentDataPath)
	if err := local.MoveOrCopyDirectory(previousStatus, currentStatus); err != nil {
		return false, fmt.Errorf("failed to move storage: %v", err)
	}

	return true, nil
}

func confirm(msg string, def bool) (bool, error) {
	err := survey.AskOne(&survey.Confirm{
		Message: msg,
		Default: def,
	}, &def)
	return def, err
}
