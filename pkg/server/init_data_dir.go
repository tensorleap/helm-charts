package server

import (
	"context"

	"github.com/tensorleap/helm-charts/pkg/local"
)

type InitDataDirFuncType func(ctx context.Context, flag string) (isDataTransfer bool, err error)

func defaultDataDirFunc(ctx context.Context, flag string) (bool, error) {
	previousDataDir := local.DEFAULT_DATA_DIR
	err := local.SetDataDir(previousDataDir, flag)
	if err != nil {
		return false, err
	}
	isTransfer, err := TransferData(ctx)
	if err != nil {
		return isTransfer, err
	}
	return isTransfer, nil
}

var InitDataDirFunc InitDataDirFuncType = defaultDataDirFunc

func SetInitDataDirFunc(f InitDataDirFuncType) {
	InitDataDirFunc = f
}
