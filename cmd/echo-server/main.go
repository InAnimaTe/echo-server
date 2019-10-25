package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"strings"
	"sort"
	"log"
	"time"
)

func RunServer(addr string, sslAddr string, ssl map[string]string) chan error {
	// Thanks buddy guy: http://stackoverflow.com/a/29468115
	errs := make(chan error)

	go func() {
		fmt.Printf("Echo server listening on port %s.\n", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
            		errs <- err
        	}
	}()

	go func() {
		fmt.Printf("Echo server listening on ssl port %s.\n", sslAddr)
		if err := http.ListenAndServeTLS(sslAddr, ssl["cert"], ssl["key"], nil); err != nil {
            		errs <- err
        	}
	}()

	return errs
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	sslport := os.Getenv("SSLPORT")
	if sslport == "" {
		sslport = "8443"
	}

	http.HandleFunc("/", http.HandlerFunc(handler))

	errs := RunServer(":"+port, ":"+sslport, map[string]string{
	"cert": "cert.pem",
	"key":  "key.pem",
	})

	// This will run forever until channel receives error
	select {
	case err := <-errs:
	log.Printf("Could not start serving service due to (error: %s)", err)
	}

}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

func handler(wr http.ResponseWriter, req *http.Request) {
	fmt.Printf("%s | %s | %s %s\n", req.RemoteAddr, req.UserAgent(), req.Method, req.URL)
	if websocket.IsWebSocketUpgrade(req) {
		serveWebSocket(wr, req)
	}	else if req.URL.Path == "/ws" {
		wr.Header().Add("Content-Type", "text/html")
		wr.WriteHeader(200)
		io.WriteString(wr, websocketHTML)
	} else if req.URL.Path == "/health" {
		healthEndpoint(wr, req)
	}	else {
		serveHTTP(wr, req)
	}
}

func healthEndpoint(wr http.ResponseWriter, req *http.Request) {

	// Health Check endpoint

	type Health struct {
		Status    string
	}

	health := Health{"ok"}

	healthResponse, err := json.Marshal(health)
	if err != nil {
			panic(err)
	}

	wr.Header().Set("Content-Type", "application/json")
	wr.Write(healthResponse)

}

func serveWebSocket(wr http.ResponseWriter, req *http.Request) {
	connection, err := upgrader.Upgrade(wr, req, nil)
	if err != nil {
		fmt.Printf("%s | %s\n", req.RemoteAddr, err)
		return
	}

	defer connection.Close()
	fmt.Printf("%s | upgraded to websocket\n", req.RemoteAddr)

	var message []byte

	host, err := os.Hostname()
	if err == nil {
		message = []byte(fmt.Sprintf("Request served by %s", host))
	} else {
		message = []byte(fmt.Sprintf("Server hostname unknown: %s", err.Error()))
	}

	err = connection.WriteMessage(websocket.TextMessage, message)
	if err == nil {
		var messageType int

		for {
			messageType, message, err = connection.ReadMessage()
			if err != nil {
				break
			}

			if messageType == websocket.TextMessage {
				fmt.Printf("%s | txt | %s\n", req.RemoteAddr, message)
			} else {
				fmt.Printf("%s | bin | %d byte(s)\n", req.RemoteAddr, len(message))
			}

			err = connection.WriteMessage(messageType, message)
			if err != nil {
				break
			}
		}
	}

	if err != nil {
		fmt.Printf("%s | %s\n", req.RemoteAddr, err)
	}
}

func serveHTTP(wr http.ResponseWriter, req *http.Request) {
	// Take in extra headers to present
	addheaders := os.Getenv("ADD_HEADERS")
	if len(addheaders) > 0 {
		// rawjson := json.RawMessage(addheaders)
		var headerjson map[string]interface{}
		json.Unmarshal([]byte(addheaders), &headerjson)
		for k, v := range headerjson {
			//fmt.Printf("%s = %s", k, v)
			wr.Header().Add(k, v.(string))
	    	}
	}


	wr.Header().Add("Content-Type", "text/plain")
	wr.WriteHeader(200)

	// -> Intro

	fmt.Fprintln(wr, "Welcome to echo-server!  Here's what I know.")
	fmt.Fprintf(wr, "  > Head to /ws for interactive websocket echo!\n\n")
	// -> Hostname

	host, err := os.Hostname()
	if err == nil {
		fmt.Fprintf(wr, "-> My hostname is: %s\n\n", host)
	} else {
		fmt.Fprintf(wr, "-> Server hostname unknown: %s\n\n", err.Error())
	}

	// -> Pod Details

	podname := os.Getenv("POD_NAME")
	if len(podname) > 0 {
		fmt.Fprintf(wr, "-> My Pod Name is: %s\n", podname)
	}

	podnamespace := os.Getenv("POD_NAMESPACE")
	if len(podname) > 0 {
		fmt.Fprintf(wr, "-> My Pod Namespace is: %s\n", podnamespace)
	}

	podip := os.Getenv("POD_IP")
	if len(podname) > 0 {
		fmt.Fprintf(wr, "-> My Pod IP is: %s\n\n", podip)
	}

	// Requesting/Source IP

	fmt.Fprintf(wr, "-> Requesting IP: %s\n\n", req.RemoteAddr)

	// -> TLS Info
	if req.TLS != nil {
		fmt.Fprintln(wr, "-> TLS Connection Info | \n")
		fmt.Fprintf(wr, "  %+v\n\n", req.TLS)
	}

	// -> Request Header

	fmt.Fprintln(wr, "-> Request Headers | \n")
	fmt.Fprintf(wr, "  %s %s %s\n", req.Proto, req.Method, req.URL)
	fmt.Fprintf(wr, "\n")
	fmt.Fprintf(wr, "  Host: %s\n", req.Host)

	var reqheaders []string
	for k,vs := range req.Header {
		for _, v := range vs {
			reqheaders = append(reqheaders, (fmt.Sprintf("%s: %s", k,v)))
		}
	}
	sort.Strings(reqheaders)
	for _, h := range reqheaders {
		fmt.Fprintf(wr, "  %s\n", h)
	}

	// -> Response Headers

	fmt.Fprintf(wr, "\n\n")
	fmt.Fprintln(wr, "-> Response Headers | \n")
	var respheaders []string
	for k,vs := range wr.Header() {
		for _, v := range vs {
			respheaders = append(respheaders, (fmt.Sprintf("%s: %s", k,v)))
		}
	}
	sort.Strings(respheaders)
	for _, h := range respheaders {
		fmt.Fprintf(wr, "  %s\n", h)
	}
	fmt.Fprintln(wr, "\n  > Note that you may also see \"Transfer-Encoding\" and \"Date\"!")

	// -> Environment

	fmt.Fprintf(wr, "\n\n")
	fmt.Fprintln(wr, "-> My environment |")
	envs := os.Environ()
	sort.Strings(envs)
	for _, e := range envs {
		pair := strings.Split(e, "=")
		fmt.Fprintf(wr, "  %s=%s\n", pair[0],pair[1])
	}

	// Lets get resolv.conf
	fmt.Fprintf(wr, "\n\n")
	resolvfile, err := ioutil.ReadFile("/etc/resolv.conf") // just pass the file name
	if err != nil {
		fmt.Fprint(wr, "%s", err)
	}

	str := string(resolvfile) // convert content to a 'string'

	fmt.Fprintf(wr, "-> Contents of /etc/resolv.conf | \n%s\n", str) // print the content as a 'string'

	// Lets get hosts
	fmt.Fprintf(wr, "\n")
	hostsfile, err := ioutil.ReadFile("/etc/hosts") // just pass the file name
	if err != nil {
		fmt.Fprint(wr, "%s", err)
	}

	hostsstr := string(hostsfile) // convert content to a 'string'

	fmt.Fprintf(wr, "-> Contents of /etc/hosts | \n%s\n\n", hostsstr) // print the content as a 'string'


	fmt.Fprintln(wr, "")
	curtime := time.Now().UTC()
	fmt.Fprintln(wr, "-> And that's the way it is", curtime)
	fmt.Fprintln(wr, "\n// Thanks for using echo-server, a project by Mario Loria (InAnimaTe).")
	fmt.Fprintln(wr, "// https://github.com/inanimate/echo-server")
	fmt.Fprintln(wr, "// https://hub.docker.com/r/inanimate/echo-server")
	io.Copy(wr, req.Body)
}
