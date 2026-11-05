package browser_api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{cert: cert, key: key}
}

func (ca *testCA) pem() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw})
}

func (ca *testCA) clientCert(t *testing.T, cn string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func writeAuthFiles(t *testing.T, dir string, caPEM []byte, names string) *requestHeaderAuth {
	t.Helper()
	caFile := filepath.Join(dir, "requestheader-client-ca-file")
	namesFile := filepath.Join(dir, "requestheader-allowed-names")
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(namesFile, []byte(names), 0o600); err != nil {
		t.Fatal(err)
	}
	return newRequestHeaderAuth(caFile, namesFile)
}

func newAuthServer(t *testing.T, auth *requestHeaderAuth) *httptest.Server {
	t.Helper()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	server := httptest.NewUnstartedServer(auth.middleware(ok))
	server.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func get(t *testing.T, server *httptest.Server, path string, cert *tls.Certificate) int {
	t.Helper()
	transport := server.Client().Transport.(*http.Transport).Clone()
	if cert != nil {
		transport.TLSClientConfig.Certificates = []tls.Certificate{*cert}
	}
	resp, err := (&http.Client{Transport: transport}).Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestRequestHeaderAuth(t *testing.T) {
	frontProxyCA := newTestCA(t)
	otherCA := newTestCA(t)
	auth := writeAuthFiles(t, t.TempDir(), frontProxyCA.pem(), `["system:openshift-aggregator"]`)
	server := newAuthServer(t, auth)

	aggregator := frontProxyCA.clientCert(t, "system:openshift-aggregator")
	wrongName := frontProxyCA.clientCert(t, "someone-else")
	untrusted := otherCA.clientCert(t, "system:openshift-aggregator")

	const actionPath = "/apis/subresource.kubebrowser.io/v1alpha1/namespaces/ns/browsers/b/action"
	cases := []struct {
		name string
		path string
		cert *tls.Certificate
		want int
	}{
		{"no certificate", actionPath, nil, http.StatusUnauthorized},
		{"certificate from another CA", actionPath, &untrusted, http.StatusUnauthorized},
		{"common name not allowed", actionPath, &wrongName, http.StatusForbidden},
		{"front-proxy certificate", actionPath, &aggregator, http.StatusOK},
		{"healthz without certificate", "/healthz", nil, http.StatusOK},
		{"readyz without certificate", "/readyz", nil, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := get(t, server, tc.path, tc.cert); got != tc.want {
				t.Errorf("got status %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRequestHeaderAuthEmptyAllowedNames(t *testing.T) {
	ca := newTestCA(t)
	server := newAuthServer(t, writeAuthFiles(t, t.TempDir(), ca.pem(), `[]`))

	cert := ca.clientCert(t, "any-name")
	if got := get(t, server, "/apis/x", &cert); got != http.StatusOK {
		t.Errorf("got status %d, want %d", got, http.StatusOK)
	}
}

func TestRequestHeaderAuthMissingCA(t *testing.T) {
	ca := newTestCA(t)
	dir := t.TempDir()
	server := newAuthServer(t, newRequestHeaderAuth(filepath.Join(dir, "missing"), filepath.Join(dir, "missing-names")))

	// Fail closed when the CA hasn't been mounted.
	cert := ca.clientCert(t, "system:openshift-aggregator")
	if got := get(t, server, "/apis/x", &cert); got != http.StatusInternalServerError {
		t.Errorf("got status %d, want %d", got, http.StatusInternalServerError)
	}
}

func TestRequestHeaderAuthReloadsRotatedCA(t *testing.T) {
	oldCA := newTestCA(t)
	newCA := newTestCA(t)
	dir := t.TempDir()
	auth := writeAuthFiles(t, dir, oldCA.pem(), `[]`)
	server := newAuthServer(t, auth)

	cert := newCA.clientCert(t, "system:openshift-aggregator")
	if got := get(t, server, "/apis/x", &cert); got != http.StatusUnauthorized {
		t.Fatalf("before rotation: got status %d, want %d", got, http.StatusUnauthorized)
	}

	caFile := filepath.Join(dir, "requestheader-client-ca-file")
	if err := os.WriteFile(caFile, newCA.pem(), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(caFile, future, future); err != nil {
		t.Fatal(err)
	}

	if got := get(t, server, "/apis/x", &cert); got != http.StatusOK {
		t.Errorf("after rotation: got status %d, want %d", got, http.StatusOK)
	}
}
