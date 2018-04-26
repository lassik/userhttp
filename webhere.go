package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/http/cgi"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func templateFromString(s string) *template.Template {
	t, err := template.New("").Parse(strings.TrimSpace(s))
	if err != nil {
		panic(err)
	}
	return t
}

var errorTemplate = templateFromString(`
<!doctype html>
<html>
  <head>
    <title>{{.}}</title>
  </head>
  <body>
    <h1>{{.}}</h1>
  </body>
</html>
`)

type dirListVars struct {
	RelPath string
	Entries []os.FileInfo
}

var dirListTemplate = templateFromString(`
<!doctype html>
<html>
  <head>
    <title>Directory /{{.RelPath}}</title>
  </head>
  <body>
    <h1>Directory /{{.RelPath}}</h1>
    <ul>
      {{- range .Entries}}
      <li><a href="/{{$.RelPath}}{{.Name}}">{{.Name}}</a></li>
      {{- end}}
    </ul>
  </body>
</html>
`)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func respondWithError(resp http.ResponseWriter, req *http.Request, statusCode int) {
	status := fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode))
	buf := bytes.NewBuffer(nil)
	check(errorTemplate.Execute(buf, status))
	resp.Header().Set("Content-Type", "text/html; charset=utf-8")
	resp.WriteHeader(statusCode)
	resp.Write(buf.Bytes())
}

func respondWithRedirect(resp http.ResponseWriter, req *http.Request, newLocation string) {
	resp.Header().Set("Location", newLocation)
	switch req.Method {
	case "HEAD":
		fallthrough
	case "GET":
		resp.WriteHeader(http.StatusMovedPermanently)
	case "POST":
		resp.WriteHeader(http.StatusPermanentRedirect)
	default:
		resp.WriteHeader(http.StatusBadRequest)
	}
}

func requireGetMethod(resp http.ResponseWriter, req *http.Request) bool {
	if req.Method == "GET" || req.Method == "HEAD" {
		return true
	}
	resp.WriteHeader(http.StatusMethodNotAllowed)
	return false
}

var responseStatusCode int = http.StatusOK
var responseHeader = make(http.Header)
var responseBody []byte

type stdoutResponseWriter struct{}

func (rw stdoutResponseWriter) Header() http.Header {
	return responseHeader
}

func (rw stdoutResponseWriter) WriteHeader(statusCode int) {
	responseStatusCode = statusCode
}

func (rw stdoutResponseWriter) Write(body []byte) (int, error) {
	responseBody = append(responseBody, body...)
	return len(body), nil
}

func writeResponseToStdout(req *http.Request) {
	status := fmt.Sprintf("%d %s", responseStatusCode,
		http.StatusText(responseStatusCode))
	resp := http.Response{
		Status:        status,
		StatusCode:    responseStatusCode,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          ioutil.NopCloser(bytes.NewReader(responseBody)),
		ContentLength: int64(len(responseBody)),
		Request:       req,
		Header:        responseHeader,
	}
	resp.Write(os.Stdout)
}

func serveCgiScript(resp http.ResponseWriter, req *http.Request, relPath string) {
	abs, err := filepath.Abs(relPath)
	check(err)
	handler := cgi.Handler{
		Path:                path.Join(abs, "index.cgi"),
		Dir:                 abs,
		PathLocationHandler: http.DefaultServeMux,
	}
	handler.ServeHTTP(resp, req)
}

func serveDirList(resp http.ResponseWriter, req *http.Request, relPath string) {
	if !requireGetMethod(resp, req) {
		return
	}
	dir, err := os.Open(relPath)
	check(err)
	if relPath == "." {
		relPath = ""
	} else {
		relPath = relPath + "/"
	}
	fileInfos, err := dir.Readdir(1000)
	check(err)
	buf := bytes.NewBuffer(nil)
	check(dirListTemplate.Execute(buf, dirListVars{relPath, fileInfos}))
	resp.Header().Set("Content-Type", "text/html; charset=utf-8")
	resp.Write(buf.Bytes())
}

func serveDir(resp http.ResponseWriter, req *http.Request, relPath string) {
	filename := path.Join(relPath, "index.cgi")
	if _, err := os.Stat(filename); err == nil {
		serveCgiScript(resp, req, relPath)
		return
	}
	filename = path.Join(relPath, "index.html")
	if _, err := os.Stat(filename); err == nil {
		serveStaticFile(resp, req, filename)
		return
	}
	serveDirList(resp, req, relPath)
}

func serveStaticFile(resp http.ResponseWriter, req *http.Request, relPath string) {
	if !requireGetMethod(resp, req) {
		return
	}
	body, err := ioutil.ReadFile(relPath)
	if type_ := mime.TypeByExtension(path.Ext(relPath)); type_ != "" {
		resp.Header().Set("Content-Type", type_)
	}
	check(err)
	resp.Write(body)
}

func handleRequest(resp http.ResponseWriter, req *http.Request) {
	log.Printf("%s %s\n", req.Method, req.URL.Path)
	hadFinalSlash := strings.HasSuffix(req.URL.Path, "/")
	relPath := path.Join(".", req.URL.Path)
	relPath = path.Clean(relPath)
	info, err := os.Stat(relPath)
	if os.IsNotExist(err) {
		respondWithError(resp, req, http.StatusNotFound)
		return
	}
	check(err)
	switch mode := info.Mode(); {
	case mode.IsRegular():
		serveStaticFile(resp, req, relPath)
	case mode.IsDir():
		if !hadFinalSlash {
			respondWithRedirect(resp, req, "/"+relPath+"/")
			break
		}
		serveDir(resp, req, relPath)
	default:
		respondWithError(resp, req, http.StatusBadRequest)
	}
}

func serveStdinStdout() {
	req, err := http.ReadRequest(bufio.NewReader(os.Stdin))
	check(err)
	handleRequest(stdoutResponseWriter{}, req)
	writeResponseToStdout(req)
}

func main() {
	var addr string
	flag.StringVar(&addr, "b", "8080", "Bind address and port")
	flag.Parse()
	if addr == "-" {
		serveStdinStdout()
	} else {
		http.HandleFunc("/", handleRequest)
		http.ListenAndServe(addr, nil)
	}
}
