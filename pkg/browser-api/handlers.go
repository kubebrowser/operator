package browser_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"

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
		},
	}

	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(list)
	if err != nil {
		fmt.Println(err, "failed to encode json encode api resource list")
		w.WriteHeader(500)
	}
}

func executeBrowserAction(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	namespace := vars["namespace"]
	name := vars["name"]

	if namespace == "" || name == "" {
		err := errors.New("namespace or name parameter is missing in the url")
		fmt.Println("ERROR", err)
		w.WriteHeader(500)
		_, err = fmt.Fprint(w, err.Error())
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	// check browser exists in this namespace or if user has acces?

	body, err := io.ReadAll(req.Body)
	if err != nil {
		fmt.Println("ERROR", err)
		w.WriteHeader(400)
		_, err = fmt.Fprint(w, err.Error())
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	type action struct {
		Kind string `json:"kind"`
		Url  string `json:"url,omitempty"`
	}

	actionData := action{}

	err = json.Unmarshal(body, &actionData)
	if err != nil {
		fmt.Println("ERROR", err)
		w.WriteHeader(400)
		_, err = fmt.Fprint(w, err.Error())
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	validKinds := []string{
		"page-reload",
		"page-navigate",
		"page-goback",
		"page-goforward",
		"page-reset",
	}

	if !slices.Contains(validKinds, actionData.Kind) {
		err := errors.New("'kind' must be one of the following values: '" + strings.Join(validKinds, ", ") + "'")
		fmt.Println("ERROR", err)
		w.WriteHeader(400)
		_, err = fmt.Fprint(w, err.Error())
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	if actionData.Kind == "page-navigate" && actionData.Url == "" {
		err := errors.New("'url' parameter is required for 'page-navigate' action kind")
		fmt.Println("ERROR", err)
		w.WriteHeader(400)
		_, err = fmt.Fprint(w, err.Error())
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	if actionData.Kind != "page-navigate" && actionData.Url != "" {
		err := errors.New("'url' parameter is not allowed for '" + actionData.Kind + "' action kind")
		fmt.Println("ERROR", err)
		w.WriteHeader(400)
		_, err = fmt.Fprint(w, err.Error())
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	fmt.Printf("Executing '%s' on browser '%s' in namespace '%s'\n", actionData.Kind, name, namespace)
	resp, err := http.Post(browserServerUrl(name, namespace), "application/json", bytes.NewBuffer(body))
	if err != nil {
		fmt.Println("ERROR", err)
		w.WriteHeader(500)
		_, err = fmt.Fprint(w, err.Error())
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	if resp.StatusCode != 200 && resp.StatusCode != 202 {

		defer func() {
			if err := resp.Body.Close(); err != nil {
				fmt.Printf("error closing body: %v", err)
			}
		}()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("ERROR", err)
			w.WriteHeader(400)
			_, err = fmt.Fprint(w, err.Error()+":failed to parse response body")
			if err != nil {
				fmt.Println(err)
			}
			return
		}

		err := errors.New("request to browser server returned status code '" + strconv.Itoa(resp.StatusCode) + "' and body '" + string(body) + "'")
		fmt.Println("ERROR", err)
		w.WriteHeader(500)
		_, err = fmt.Fprint(w, err)
		if err != nil {
			fmt.Println(err)
		}
		return
	}

	_, err = fmt.Fprint(w, "action executed")
	if err != nil {
		fmt.Println(err)
	}
}

func connectToBrowserVNC(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	namespace := vars["namespace"]
	name := vars["name"]

	if namespace == "" || name == "" {
		err := errors.New("namespace or name parameter is missing in the url")
		fmt.Println("ERROR", err)
		w.WriteHeader(500)
		_, err = fmt.Fprint(w, err.Error())
		if err != nil {
			fmt.Println(err)
		}
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

	tcpConn, err := net.Dial("tcp", browserVncUrl(name, namespace))
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
