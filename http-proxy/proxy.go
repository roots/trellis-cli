package httpProxy

import (
	_ "embed"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed files/not_found.html
var NotFoundTemplate string

var (
	hostProxy map[string]*httputil.ReverseProxy = map[string]*httputil.ReverseProxy{}
)

type proxyHandler struct {
	hostsPath        string
	notFoundTemplate *template.Template
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if fn, ok := hostProxy[host]; ok {
		fn.ServeHTTP(w, r)
		return
	}

	targetHosts := fetchHosts(h.hostsPath)

	data := struct {
		Hosts map[string]string
	}{
		Hosts: targetHosts,
	}

	if target, ok := targetHosts[host]; ok {
		remoteUrl, err := url.Parse(target)
		if err != nil {
			log.Println("target parse fail:", err)
			return
		}

		errorHandler := func(rw http.ResponseWriter, req *http.Request, err error) {
			h.notFoundTemplate.Execute(rw, data)
			rw.WriteHeader(http.StatusBadGateway)
		}

		proxy := newSingleHostReverseProxy(remoteUrl, errorHandler)
		hostProxy[host] = proxy
		proxy.ServeHTTP(w, r)
		return
	}

	w.WriteHeader(http.StatusBadGateway)
	h.notFoundTemplate.Execute(w, data)
}

func fetchHosts(path string) map[string]string {
	hosts := make(map[string]string)

	files, err := os.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(path, file.Name()))

		if err != nil {
			panic(err)
		}

		hosts[file.Name()] = strings.TrimRight(string(content), "\n")
	}

	return hosts
}

func newSingleHostReverseProxy(target *url.URL, errorHandler func(http.ResponseWriter, *http.Request, error)) *httputil.ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path, req.URL.RawPath = joinURLPath(target, req.URL)
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to default value
			req.Header.Set("User-Agent", "")
		}
	}

	return &httputil.ReverseProxy{Director: director, ErrorHandler: errorHandler}
}

func joinURLPath(a, b *url.URL) (path, rawpath string) {
	if a.RawPath == "" && b.RawPath == "" {
		return singleJoiningSlash(a.Path, b.Path), ""
	}
	// Same as singleJoiningSlash, but uses EscapedPath to determine
	// whether a slash should be added
	apath := a.EscapedPath()
	bpath := b.EscapedPath()

	aslash := strings.HasSuffix(apath, "/")
	bslash := strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	}
	return a.Path + b.Path, apath + bpath
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
