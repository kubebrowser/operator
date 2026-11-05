package browser_api

import (
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"
)

// The kube-apiserver proxies aggregated API requests with a front-proxy client
// certificate. Only requests carrying that certificate are allowed through, so
// pods in the cluster can't call browser-api directly and skip RBAC.
//
// The operator copies the CA and allowed names from
// kube-system/extension-apiserver-authentication into a ConfigMap mounted here.
const (
	defaultClientCAFilePath     = "/auth/requestheader-client-ca-file"
	defaultAllowedNamesFilePath = "/auth/requestheader-allowed-names"
)

type requestHeaderAuth struct {
	caFile    string
	namesFile string

	mu       sync.Mutex
	loadedAt time.Time
	roots    *x509.CertPool
	names    []string
}

func newRequestHeaderAuth(caFile string, namesFile string) *requestHeaderAuth {
	return &requestHeaderAuth{caFile: caFile, namesFile: namesFile}
}

// load re-reads the CA and allowed names when the mounted files change, so CA
// rotation is picked up without a restart.
func (a *requestHeaderAuth) load() (*x509.CertPool, []string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	info, err := os.Stat(a.caFile)
	if err != nil {
		return nil, nil, err
	}
	if a.roots != nil && info.ModTime().Equal(a.loadedAt) {
		return a.roots, a.names, nil
	}

	caPEM, err := os.ReadFile(a.caFile)
	if err != nil {
		return nil, nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, nil, errors.New("no certificates found in " + a.caFile)
	}

	var names []string
	namesJSON, err := os.ReadFile(a.namesFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if len(namesJSON) > 0 {
		if err := json.Unmarshal(namesJSON, &names); err != nil {
			return nil, nil, fmt.Errorf("failed to parse %s: %w", a.namesFile, err)
		}
	}

	a.roots, a.names, a.loadedAt = roots, names, info.ModTime()
	return roots, names, nil
}

func (a *requestHeaderAuth) verify(req *http.Request) (int, error) {
	if req.TLS == nil || len(req.TLS.PeerCertificates) == 0 {
		return http.StatusUnauthorized, errors.New("client certificate required")
	}

	roots, names, err := a.load()
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("failed to load front-proxy CA: %w", err)
	}

	certs := req.TLS.PeerCertificates
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}
	_, err = certs[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return http.StatusUnauthorized, fmt.Errorf("client certificate not trusted: %w", err)
	}

	// An empty list means any certificate signed by the CA is accepted.
	if len(names) > 0 && !slices.Contains(names, certs[0].Subject.CommonName) {
		return http.StatusForbidden, fmt.Errorf("client certificate common name '%s' is not allowed", certs[0].Subject.CommonName)
	}

	return http.StatusOK, nil
}

func (a *requestHeaderAuth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/healthz" || req.URL.Path == "/readyz" {
			next.ServeHTTP(w, req)
			return
		}

		if status, err := a.verify(req); err != nil {
			fmt.Println("ERROR", req.RemoteAddr, err)
			w.WriteHeader(status)
			_, err = fmt.Fprint(w, http.StatusText(status))
			if err != nil {
				fmt.Println(err)
			}
			return
		}

		next.ServeHTTP(w, req)
	})
}
