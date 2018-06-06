// KatWeb by kittyhacker101 - HTTP(S) / Websockets Reverse Proxy
package main

import (
	"crypto/tls"
	"encoding/json"
	"github.com/yhat/wsutil"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// UpdateData contains a struct for parsing returned json from the request
type UpdateData struct {
	Latest string `json:"tag_name"`
}

var (
	upd UpdateData

	tlsp = &tls.Config{
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP521,
			tls.CurveP384,
			tls.CurveP256,
		},
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	proxy = &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			prox, loc := GetProxy(r)
			r.URL, _ = url.Parse(prox + strings.TrimPrefix(r.URL.String(), "/"+loc))
		},
		ErrorLog: Logger,
		Transport: &http.Transport{
			TLSClientConfig:     tlsp,
			MaxIdleConns:        4096,
			MaxIdleConnsPerHost: 256,
			IdleConnTimeout:     time.Duration(conf.DatTime*8) * time.Second,
		},
	}

	wsproxy = &wsutil.ReverseProxy{
		Director: func(r *http.Request) {
			prox, loc := GetProxy(r)
			r.URL, _ = url.Parse(prox + strings.TrimPrefix(r.URL.String(), "/"+loc))
			if r.URL.Scheme == "https" {
				r.URL.Scheme = "wss://"
			} else {
				r.URL.Scheme = "ws://"
			}
		},
		ErrorLog:        Logger,
		TLSClientConfig: tlsp,
	}

	// UpdateClient is the http.Client used for checking the latest version of KatWeb
	UpdateClient = &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig:   tlsp,
		},
		Timeout: 2 * time.Second,
	}

	proxyMap, redirMap sync.Map
)

// GetProxy finds the correct proxy index to use from the conf.Proxy struct
func GetProxy(r *http.Request) (string, string) {
	url, err := url.QueryUnescape(r.URL.EscapedPath())
	if err != nil {
		url = r.URL.EscapedPath()
	}
	urlp := strings.Split(url, "/")

	if val, ok := proxyMap.Load(r.Host); ok {
		return val.(string), r.Host
	}

	if len(urlp) == 0 {
		return "", ""
	}

	if val, ok := proxyMap.Load(urlp[1]); ok {
		return val.(string), urlp[1]
	}

	return "", ""
}

// MakeProxyMap converts the conf.Proxy into a map[string]string
func MakeProxyMap() {
	for i := range conf.Proxy {
		proxyMap.Store(conf.Proxy[i].Loc, conf.Proxy[i].URL)
	}
	for i := range conf.Redir {
		redirMap.Store(conf.Redir[i].Loc, conf.Redir[i].URL)
	}
}

// ProxyRequest reverse-proxies a request, or websocket
func ProxyRequest(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Connection"), "Upgrade") && strings.Contains(r.Header.Get("Upgrade"), "websocket") {
		wsproxy.ServeHTTP(w, r)
	} else {
		proxy.ServeHTTP(w, r)
	}
}

// CheckUpdate checks if you are using the latest version of KatWeb
func CheckUpdate(current string) string {
	resp, _ := UpdateClient.Get("https://api.github.com/repos/kittyhacker101/KatWeb/releases/latest")
	if resp.Body == nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	if json.Unmarshal(body, &upd) != nil {
		return ""
	}

	if !strings.HasPrefix(current, upd.Latest[:4]) && current[5:] != "0" {
		return "[Warn] : KatWeb is very out of date (" + upd.Latest[1:] + " > " + current[1:] + "). Please update to the latest version as soon as possible.\n"
	}
	if strings.HasSuffix(current, "-dev") {
		return "[Info] : Running a development version of KatWeb is not recommended.\n"
	}
	if upd.Latest != current {
		return "[Info] : KatWeb is out of date (" + upd.Latest[1:] + " > " + current[1:] + "). Using the latest version is recommended.\n"
	}
	return ""
}
