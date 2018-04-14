package main

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/http/cgi"
	"os"
	"path"
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

func respond(req *http.Request, statusCode int, header http.Header, body []byte) {
	status := fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode))
	resp := &http.Response{
		Status:        status,
		StatusCode:    statusCode,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          ioutil.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		Header:        header,
	}
	resp.Write(os.Stdout)
}

func respondWithError(req *http.Request, statusCode int) {
	status := fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode))
	buf := bytes.NewBuffer(nil)
	check(errorTemplate.Execute(buf, status))
	body := buf.Bytes()
	header := make(http.Header, 0)
	header.Set("Content-Type", "text/html; charset=utf-8")
	respond(req, statusCode, header, body)
}

func respondWithRedirect(req *http.Request, newLocation string) {
	header := make(http.Header, 0)
	header.Set("Location", newLocation)
	body := []byte{}
	switch req.Method {
	case "HEAD":
		fallthrough
	case "GET":
		respond(req, http.StatusMovedPermanently, header, body)
	case "POST":
		respond(req, http.StatusPermanentRedirect, header, body)
	default:
		respondWithError(req, http.StatusBadRequest)
	}
}

func requireGetMethod(req *http.Request) bool {
	if req.Method == "GET" || req.Method == "HEAD" {
		return true
	}
	respondWithError(req, http.StatusMethodNotAllowed)
	return false
}

type stdoutResponseWriter struct {
	req        *http.Request
	statusCode int
	header     http.Header
}

func makeStdoutResponseWriter(req *http.Request) stdoutResponseWriter {
	return stdoutResponseWriter{
		req:        req,
		statusCode: http.StatusOK,
		header:     make(http.Header),
	}
}

func (rw stdoutResponseWriter) Header() http.Header {
	return rw.header
}

func (rw stdoutResponseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
}

func (rw stdoutResponseWriter) Write(body []byte) (int, error) {
	respond(rw.req, rw.statusCode, rw.header, body)
	return len(body), nil
}

func serveStaticFile(req *http.Request, relPath string) {
	if !requireGetMethod(req) {
		return
	}
	body, err := ioutil.ReadFile(relPath)
	header := make(http.Header, 0)
	if type_ := mime.TypeByExtension(path.Ext(relPath)); type_ != "" {
		header.Set("Content-Type", type_)
	}
	check(err)
	respond(req, http.StatusOK, header, body)
}

func serveCgiScript(req *http.Request, relPath string) {
	os.Chdir(relPath)
	handler := cgi.Handler{
		Path:                "index.cgi",
		PathLocationHandler: http.DefaultServeMux,
	}
	handler.ServeHTTP(makeStdoutResponseWriter(req), req)
}

func serveDirList(req *http.Request, relPath string) {
	if !requireGetMethod(req) {
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
	header := make(http.Header, 0)
	header.Set("Content-Type", "text/html; charset=utf-8")
	respond(req, http.StatusOK, header, buf.Bytes())
}

func serveDir(req *http.Request, relPath string) {
	filename := path.Join(relPath, "index.cgi")
	if _, err := os.Stat(filename); err == nil {
		serveCgiScript(req, relPath)
		return
	}
	filename = path.Join(relPath, "index.html")
	if _, err := os.Stat(filename); err == nil {
		serveStaticFile(req, filename)
		return
	}
	serveDirList(req, relPath)
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	req, err := http.ReadRequest(reader)
	check(err)
	log.Printf("%s %s\n", req.Method, req.URL.Path)
	hadFinalSlash := strings.HasSuffix(req.URL.Path, "/")
	relPath := path.Join(".", req.URL.Path)
	relPath = path.Clean(relPath)
	info, err := os.Stat(relPath)
	if os.IsNotExist(err) {
		respondWithError(req, http.StatusNotFound)
		return
	}
	check(err)
	switch mode := info.Mode(); {
	case mode.IsRegular():
		serveStaticFile(req, relPath)
	case mode.IsDir():
		if !hadFinalSlash {
			respondWithRedirect(req, "/"+relPath+"/")
			break
		}
		serveDir(req, relPath)
	default:
		respondWithError(req, http.StatusBadRequest)
	}
}
