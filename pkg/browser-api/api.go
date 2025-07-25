package browser_api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
)

const (
	// Default port that browser-api listens on.
	defaultPort = 8443

	// Default address that browser-api listens on.
	defaultHost = "0.0.0.0"

	defaultTlsCertFilePath     = "/tls/tls.crt"
	defaultTlsKeyFilePath      = "/tls/tls.key"
	developmentTlsCertFilePath = "./local/tls.crt"
	developmentTlsKeyFilePath  = "./local/tls.key"
)

type BrowserApi interface {
	Run()
}

type browserApiApp struct {
	BindAddress     string
	Port            int
	TlsCertFilePath string
	TlsKeyFilePath  string
}

func NewBrowserApi() BrowserApi {

	app := &browserApiApp{}

	if os.Getenv("HOST") != "" {
		app.BindAddress = os.Getenv("HOST")
	} else {
		app.BindAddress = defaultHost
	}

	if os.Getenv("PORT") != "" {
		port, err := strconv.Atoi(os.Getenv("PORT"))
		if err != nil {
			log.Fatal(err, "failed to parse PORT argument as integer")
		}
		app.Port = port
	} else {
		app.Port = defaultPort
	}

	if os.Getenv("ENVIRONMENT") == "development" {
		app.TlsCertFilePath = developmentTlsCertFilePath
		app.TlsKeyFilePath = developmentTlsKeyFilePath
	} else {
		app.TlsCertFilePath = defaultTlsCertFilePath
		app.TlsKeyFilePath = defaultTlsKeyFilePath
	}

	return app
}

func (app *browserApiApp) Run() {
	router := mux.NewRouter()

	router.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, "OK\n")
	}).Methods("GET")

	router.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprint(w, "OK\n")
	}).Methods("GET")

	router.HandleFunc(groupVersionBasePath(gvr), getAPIResourceList).Methods("GET")

	router.HandleFunc(namespacedResourceUrl(gvr, "vnc"), connectToBrowserVNC).Methods("GET")

	router.HandleFunc(namespacedResourceUrl(gvr, "action"), executeBrowserAction).Methods("PUT")

	fmt.Printf("Server listening on port %v\n", strconv.Itoa(app.Port))

	log.Fatal(http.ListenAndServeTLS(
		app.BindAddress+":"+strconv.Itoa(app.Port),
		app.TlsCertFilePath,
		app.TlsKeyFilePath,
		router,
	))
}
