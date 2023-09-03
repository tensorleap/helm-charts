package main

import (
	"github.com/tensorleap/helm-charts/cmd/server"
)

func main() {
	err := server.RootCommand.Execute()
	if err != nil {
		panic(err)
	}
}
