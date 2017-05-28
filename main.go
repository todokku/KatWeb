package main

import (
	"encoding/json"
	"fmt"
	"github.com/NYTimes/gziphandler"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

/* Currently unused, will be used in near future
func RemoveIndex(s []string, i int) []string {
	s[len(s)-1], s[i] = s[i], s[len(s)-1]
	return s[:len(s)-1]
}
*/

// Config file structure
type Conf struct {
	IdleTime int `json:"keepAliveTimeout"`
	CachTime int `json:"cachingTimeout"`
	HSTS     struct {
		Run bool `json:"enabled"`
		Sub bool `json:"includeSubDomains"`
		Pre bool `json:"preload"`
	} `json:"hsts"`
	Secure bool `json:"https"`
	BSniff bool `json:"nosniff"`
	IFrame bool `json:"sameorigin"`
	Zip    bool `json:"gzip"`
	Dyn    bool `json:"dynamicServing"`
	DynCa  bool `json:"cacheStruct"`
	No     bool `json:"silent"`
	Cache  struct {
		Run bool `json:"enabled"`
		Up  int  `json:"updates"`
	} `json:"hcache"`
	Name string `json:"name"`
}

var (
	handleReq  http.Handler
	handleHTTP http.Handler
	path       string
	conf       Conf
	cacheA     = []string{"html/"}
	cacheB     = []string{"ssl/", "error/", "cache/"}
)

// Peform pre-startup checks.
func checkIntact() {
	_, err := os.Stat("html")
	_, err1 := os.Stat("error")
	if err != nil || err1 != nil {
		fmt.Println("[Fatal] : HTML folders do not exist! Server will now stop.")
		os.Exit(0)
	}
	if conf.Secure {
		_, err = os.Stat("ssl/server.crt")
		_, err1 = os.Stat("ssl/server.key")
		if err != nil || err1 != nil {
			fmt.Println("[Warn] : SSL Certs do not exist! Falling back to non-secure mode...")
			conf.Secure = false
		}
	}
	if conf.Cache.Run {
		_, err = os.Stat("cache")
		if err != nil {
			fmt.Println("[Warn] : Cache folder does not exist! Disabling HTTP Cache...")
			conf.Cache.Run = false
		}
	}
}

// Check if path exists for domain, and use it instead of default if it does.
func detectPath(p string) string {
	if !conf.Dyn {
		return "html/"
	}

	// Cache stuff into a list, so that we use the hard disk less.
	if conf.DynCa {
		loc := sort.SearchStrings(cacheA, p)
		if loc < len(cacheA) && cacheA[loc] == p {
			return p
		}
		loc = sort.SearchStrings(cacheB, p)
		if loc < len(cacheB) && cacheB[loc] == p {
			return "html/"
		}
	} else {
		if p == "ssl/" || p == "error/" || p == "cache/" || p == "html/" {
			return "html/"
		}
	}

	// If it's not in the cache, check the hard disk, and add it to the cache.
	_, err := os.Stat(p)
	if err != nil {
		if conf.DynCa {
			cacheB = append(cacheB, p)
			sort.Strings(cacheB)
			if !conf.No {
				fmt.Println("[Cache][NotFound] : " + p)
			}
		}
		return "html/"
	}

	if conf.DynCa {
		cacheA = append(cacheA, p)
		sort.Strings(cacheA)
		if !conf.No {
			fmt.Println("[Cache][Found] : " + p)
		}
	}
	return p
}

// Update the Simple HTTP Cache
func updateCache() {
	for {
		filepath.Walk("cache/", func(path string, info os.FileInfo, _ error) error {
			if !info.IsDir() && path[len(path)-4:] == ".txt" {
				fmt.Println("[Cache][HTTP] : Updating " + path[6:len(path)-4] + "...")
				b, err := ioutil.ReadFile(path)
				err1 := os.Remove("cache/" + path[6:len(path)-4])
				out, err2 := os.Create("cache/" + path[6:len(path)-4])
				defer out.Close()
				resp, err3 := http.Get(strings.TrimSpace(string(b)))
				if err != nil || err1 != nil || err2 != nil || err3 != nil {
					fmt.Println("[Cache][Warn] : Unable to update " + path[6:len(path)-4] + "!")
				} else {
					defer resp.Body.Close()
					_, err = io.Copy(out, resp.Body)
					if err != nil {
						fmt.Println("[Cache][Warn] : Unable to update " + path[6:len(path)-4] + "!")
					}
				}
			}
			return nil
		})
		if !conf.No {
			fmt.Println("[Cache][HTTP] : All files in HTTP Cache updated!")
		}
		time.Sleep(time.Duration(conf.Cache.Up) * time.Second)
	}
}

func main() {
	checkIntact()

	// Load and parse config files
	fmt.Println("Loading server...")
	data, err := ioutil.ReadFile("conf.json")
	if err != nil {
		fmt.Println("[Fatal] : Unable to read config file. Server will now stop.")
		os.Exit(0)
	}
	err = json.Unmarshal(data, &conf)
	if err != nil {
		fmt.Println("[Fatal] : Unable to parse config file. Server will now stop.")
		os.Exit(0)
	}

	// We must use the UTC format when using .Format(http.TimeFormat) on the time.
	location, err := time.LoadLocation("UTC")
	if !conf.No && err != nil {
		fmt.Println("[Fatal] : Unable to load timezones. Server will now stop.")
		os.Exit(0)
	}

	// This handles all web requests
	mainHandle := func(w http.ResponseWriter, r *http.Request) {
		// Check path and file info
		url := r.URL.EscapedPath()
		if len(url) > 6 && conf.Cache.Run && url[:6] == "/cache" {
			path = "cache/"
			url = url[6:]
		} else {
			path = detectPath(r.Host + "/")
		}
		finfo, err := os.Stat(path + url)

		// Add important headers
		w.Header().Add("Server", conf.Name)
		w.Header().Add("Keep-Alive", "timeout="+strconv.Itoa(conf.IdleTime))
		if conf.CachTime != 0 {
			w.Header().Set("Cache-Control", "max-age="+strconv.Itoa(3600*conf.CachTime)+", public, stale-while-revalidate=3600")
			w.Header().Set("Expires", time.Now().In(location).Add(time.Duration(conf.CachTime)*time.Hour).Format(http.TimeFormat))
		}
		if conf.HSTS.Run {
			if conf.HSTS.Sub {
				if conf.HSTS.Pre {
					w.Header().Add("Strict-Transport-Security", "max-age=31536000;includeSubDomains;preload")
				} else {
					w.Header().Add("Strict-Transport-Security", "max-age=31536000;includeSubDomains")
				}
			} else {
				// Preload requires includeSubDomains for some reason, idk why.
				w.Header().Add("Strict-Transport-Security", "max-age=31536000")
			}
		}
		if conf.BSniff {
			w.Header().Add("X-Content-Type-Options", "nosniff")
		}
		if conf.IFrame {
			w.Header().Add("X-Frame-Options", "sameorigin")
		}
		// Check if file exists, and if it does then add modification timestamp. Then send file.
		if err != nil {
			if !conf.No {
				fmt.Println("[Web404][" + r.Host + url + "] : " + r.RemoteAddr)
			}
			http.ServeFile(w, r, "error/NotFound.html")
		} else {
			w.Header().Set("Last-Modified", finfo.ModTime().In(location).Format(http.TimeFormat))
			if !conf.No {
				fmt.Println("[Web][" + r.Host + url + "] : " + r.RemoteAddr)
			}
			http.ServeFile(w, r, path+url)
		}
	}

	// Choose the correct handler
	if conf.Zip {
		handleReq = gziphandler.GzipHandler(http.HandlerFunc(mainHandle))
	} else {
		handleReq = http.HandlerFunc(mainHandle)
	}
	if conf.Secure {
		if conf.HSTS.Run {
			handleHTTP = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "https://"+r.Host+r.URL.EscapedPath(), http.StatusMovedPermanently)
				if !conf.No {
					fmt.Println("[WebHSTS][" + r.Host + r.URL.EscapedPath() + "] : " + r.RemoteAddr)
				}
			})
		} else {
			// Serve unencrypted content on HTTP
			fmt.Println("[Info] : HSTS is disabled, causing people to use HTTP by default. Enabling it is recommended.")
			handleHTTP = handleReq
		}
	} else {
		fmt.Println("[Info] : HTTPS is disabled, allowing hackers to intercept your connection. Enabling it is recommended.")
		handleHTTP = handleReq
	}

	// Config for HTTPS Server
	srv := &http.Server{
		Addr:         ":443",
		Handler:      handleReq,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  time.Duration(conf.IdleTime) * time.Second,
	}
	// Config for HTTP Server
	srvh := &http.Server{
		Addr:         ":80",
		Handler:      handleHTTP,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  time.Duration(conf.IdleTime) * time.Second,
	}

	// This code actually starts the servers.
	fmt.Println("KatWeb HTTP Server Started.")
	if conf.Cache.Run {
		go updateCache()
	}
	if conf.Secure {
		go srvh.ListenAndServe()
		srv.ListenAndServeTLS("ssl/server.crt", "ssl/server.key")
	} else {
		srvh.ListenAndServe()
	}
	fmt.Println("[Fatal] : KatWeb was unable to bind to the needed ports!")
}