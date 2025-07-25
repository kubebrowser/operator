package main

import (
	browser_manager "github.com/kubebrowser/operator/pkg/browser-manager/manager"
)

func main() {
	manager := browser_manager.NewBrowserManager()
	manager.Run()
}
