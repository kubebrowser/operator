package browser_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getAPIResourceList(w http.ResponseWriter, req *http.Request) {
	list := &metav1.APIResourceList{
		GroupVersion: gvr.Group + "/" + gvr.Version,
		APIResources: []metav1.APIResource{
			{
				Name:       "browsers/vnc",
				Namespaced: true,
				Verbs:      []string{"get"},
			},
			{
				Name:       "browsers/action",
				Namespaced: true,
				Verbs:      []string{"update"},
			},
			{
				Name:       "browsers/state",
				Namespaced: true,
				Verbs:      []string{"get"},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(list)
	if err != nil {
		fmt.Println(err, "failed to encode json encode api resource list")
		w.WriteHeader(500)
	}
}

const (
	// browserServerTimeout bounds requests from browser-api to a browser-server.
	browserServerTimeout = 15 * time.Second
	// vncDialTimeout bounds connecting to a browser's VNC port.
	vncDialTimeout = 10 * time.Second
	// maxActionBodyBytes limits the size of an action request body.
	maxActionBodyBytes = 1 << 20
	// maxUpstreamErrorBytes limits how much of a browser-server error body is relayed.
	maxUpstreamErrorBytes = 4 << 10
	// maxStateBodyBytes limits the size of a browser-server state response.
	maxStateBodyBytes = 64 << 10

	actionKindNavigate = "page-navigate"
)

var (
	browserServerClient = &http.Client{Timeout: browserServerTimeout}

	// actionUrl and stateUrl resolve browser-server endpoints; tests override them.
	actionUrl = func(name, namespace string) string { return browserServerUrl(name, namespace, "/action") }
	stateUrl  = func(name, namespace string) string { return browserServerUrl(name, namespace, "/state") }
)

// writeError logs err and writes it as a text/plain response with the given status.
func writeError(w http.ResponseWriter, status int, err error) {
	fmt.Println("ERROR", err)
	http.Error(w, err.Error(), status)
}

// validateNavigateUrl only allows absolute http and https URLs.
func validateNavigateUrl(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("'url' must be an absolute http or https URL")
	}
	return nil
}

// writeRequestError writes a failed request to a browser-server as a response.
func writeRequestError(w http.ResponseWriter, err error) {
	var netErr net.Error
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		fmt.Println("ERROR", err)
		http.Error(w, "browser is not running or not reachable", http.StatusServiceUnavailable)
	case errors.As(err, &netErr) && netErr.Timeout():
		fmt.Println("ERROR", err)
		http.Error(w, "timed out waiting for the browser to respond", http.StatusGatewayTimeout)
	default:
		writeError(w, http.StatusBadGateway, err)
	}
}

// writeUpstreamStatusError relays an unexpected browser-server status and body as a bad gateway.
func writeUpstreamStatusError(w http.ResponseWriter, resp *http.Response) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamErrorBytes))
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("failed to read browser server response body: %w", err))
		return
	}

	writeError(w, http.StatusBadGateway, errors.New("request to browser server returned status code '"+
		strconv.Itoa(resp.StatusCode)+"' and body '"+string(body)+"'"))
}

func closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		fmt.Printf("error closing body: %v\n", err)
	}
}

func executeBrowserAction(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	namespace := vars["namespace"]
	name := vars["name"]

	if namespace == "" || name == "" {
		writeError(w, http.StatusBadRequest, errors.New("namespace or name parameter is missing in the url"))
		return
	}

	// check browser exists in this namespace or if user has acces?

	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxActionBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	type action struct {
		Kind string `json:"kind"`
		Url  string `json:"url,omitempty"`
	}

	actionData := action{}

	err = json.Unmarshal(body, &actionData)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	validKinds := []string{
		"page-reload",
		actionKindNavigate,
		"page-goback",
		"page-goforward",
		"page-reset",
	}

	if !slices.Contains(validKinds, actionData.Kind) {
		writeError(w, http.StatusBadRequest,
			errors.New("'kind' must be one of the following values: '"+strings.Join(validKinds, ", ")+"'"))
		return
	}

	if actionData.Kind == actionKindNavigate && actionData.Url == "" {
		writeError(w, http.StatusBadRequest, errors.New("'url' parameter is required for 'page-navigate' action kind"))
		return
	}

	if actionData.Kind != actionKindNavigate && actionData.Url != "" {
		writeError(w, http.StatusBadRequest,
			errors.New("'url' parameter is not allowed for '"+actionData.Kind+"' action kind"))
		return
	}

	if actionData.Kind == actionKindNavigate {
		if err := validateNavigateUrl(actionData.Url); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}

	fmt.Printf("Executing '%s' on browser '%s' in namespace '%s'\n", actionData.Kind, name, namespace)
	resp, err := browserServerClient.Post(actionUrl(name, namespace), "application/json", bytes.NewReader(body))
	if err != nil {
		writeRequestError(w, err)
		return
	}

	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		writeUpstreamStatusError(w, resp)
		return
	}

	_, err = fmt.Fprint(w, "action executed")
	if err != nil {
		fmt.Println(err)
	}
}

// browserState is the page a browser is showing.
type browserState struct {
	Url   string `json:"url"`
	Title string `json:"title"`
}

func getBrowserState(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	namespace := vars["namespace"]
	name := vars["name"]

	if namespace == "" || name == "" {
		writeError(w, http.StatusBadRequest, errors.New("namespace or name parameter is missing in the url"))
		return
	}

	resp, err := browserServerClient.Get(stateUrl(name, namespace))
	if err != nil {
		writeRequestError(w, err)
		return
	}

	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		writeUpstreamStatusError(w, resp)
		return
	}

	// decode and re-encode so only the known fields are passed on
	state := browserState{}
	err = json.NewDecoder(io.LimitReader(resp.Body, maxStateBodyBytes)).Decode(&state)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("failed to decode browser server state: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	err = json.NewEncoder(w).Encode(state)
	if err != nil {
		fmt.Println(err)
	}
}

func connectToBrowserVNC(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	namespace := vars["namespace"]
	name := vars["name"]

	if namespace == "" || name == "" {
		writeError(w, http.StatusBadRequest, errors.New("namespace or name parameter is missing in the url"))
		return
	}

	// check browser exists in this namespace or if user has acces?

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	wsConn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		fmt.Println("Upgrade failed:", err)
		return
	}

	defer func() {
		if err := wsConn.Close(); err != nil {
			fmt.Printf("error closing connection: %v\n", err)
		}
	}()

	tcpConn, err := net.DialTimeout("tcp", browserVncUrl(name, namespace), vncDialTimeout)
	if err != nil {
		fmt.Println("TCP connection failed:", err)
		err := wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
		if err != nil {
			fmt.Println(err)
		}
		return
	}
	defer func() {
		if err := tcpConn.Close(); err != nil {
			fmt.Printf("error closing connection: %v\n", err)
		}
	}()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := tcpConn.Read(buf)
			if err != nil {
				err := wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "TCP closed"))
				if err != nil {
					fmt.Println(err)
				}
				break
			}
			err = wsConn.WriteMessage(websocket.BinaryMessage, buf[:n])
			if err != nil {
				break
			}
		}
	}()

	for {
		msgType, data, err := wsConn.ReadMessage()
		if err != nil {
			break
		}
		if msgType != websocket.BinaryMessage {
			fmt.Println("Nonbinary message received, ignoring")
			continue
		}
		_, err = tcpConn.Write(data)
		if err != nil {
			break
		}
	}
}
