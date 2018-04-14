package main

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"net/http/cgi"
	"strings"
)

func templateFromString(s string) *template.Template {
	t, err := template.New("").Parse(strings.TrimSpace(s))
	if err != nil {
		panic(err)
	}
	return t
}

var pageTemplate = templateFromString(`
<!doctype html>
<html>
  <head>
    <title>Hello, {{.}}!</title>
  </head>
  <body>
    <h1>Hello, {{.}}!</h1>
    <form method="POST" action=".">
      Please enter your name: <input name="q" value="{{.}}">
      <input type="submit">
    </form>
  </body>
</html>
`)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func handler(rw http.ResponseWriter, req *http.Request) {
	q := req.FormValue("q")
	if q == "" {
		q = "stranger"
	}
	buf := bytes.NewBuffer(nil)
	check(pageTemplate.Execute(buf, q))
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(rw)
}

func main() {
	http.HandleFunc("/", handler)
	check(cgi.Serve(nil))
}
