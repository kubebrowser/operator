package main

import (
	browser_api "github.com/kubebrowser/operator/pkg/browser-api"
)

func main() {
	app := browser_api.NewBrowserApi()
	app.Run()
}
