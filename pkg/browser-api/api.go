package browser_api

import (
	"crypto/tls"
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
	// nil in development, where requests aren't proxied by the kube-apiserver
	Auth *requestHeaderAuth
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
		app.Auth = newRequestHeaderAuth(defaultClientCAFilePath, defaultAllowedNamesFilePath)
	}

	return app
}

func (app *browserApiApp) Run() {
	router := mux.NewRouter()

	router.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		_, err := fmt.Fprint(w, "OK\n")
		if err != nil {
			fmt.Println(err)
		}
	}).Methods("GET")

	router.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) {
		_, err := fmt.Fprint(w, "OK\n")
		if err != nil {
			fmt.Println(err)
		}

	}).Methods("GET")

	router.HandleFunc(groupVersionBasePath(gvr), getAPIResourceList).Methods("GET")

	router.HandleFunc(namespacedResourceUrl(gvr, "vnc"), connectToBrowserVNC).Methods("GET")

	router.HandleFunc(namespacedResourceUrl(gvr, "action"), executeBrowserAction).Methods("PUT")

	var handler http.Handler = router
	if app.Auth != nil {
		handler = app.Auth.middleware(router)
	} else {
		fmt.Println("WARNING: front-proxy client certificate check is disabled in development")
	}

	server := &http.Server{
		Addr:    app.BindAddress + ":" + strconv.Itoa(app.Port),
		Handler: handler,
		// The certificate is checked in the middleware against a CA that can
		// change at runtime, so the handshake only asks for it.
		TLSConfig: &tls.Config{ClientAuth: tls.RequestClientCert},
	}

	fmt.Printf("Server listening on port %v\n", strconv.Itoa(app.Port))

	log.Fatal(server.ListenAndServeTLS(app.TlsCertFilePath, app.TlsKeyFilePath))
}
