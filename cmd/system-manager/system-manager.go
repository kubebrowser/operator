package main

import (
	system_manager "github.com/kubebrowser/operator/pkg/system-manager/manager"
)

func main() {
	manager := system_manager.NewSystemManager()
	manager.Run()
}
