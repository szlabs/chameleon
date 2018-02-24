package lib

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

//ServerConfig used to config the proxy server
type ServerConfig struct {
	Host        string
	Port        int
	DockerdHost string
}

//ProxyServer serves the requests
type ProxyServer struct {
	server      *http.Server
	proxy       *httputil.ReverseProxy
	host        string
	port        int
	backend     string
	dockerdHost string
	running     bool
	reqParser   *ParserChain
	executor    *Executor
	packer      *Packer
}

//NewProxyServer create new server instance
func NewProxyServer(config ServerConfig) *ProxyServer {
	return &ProxyServer{
		host:        config.Host,
		port:        config.Port,
		dockerdHost: config.DockerdHost,
	}
}

//Start the proxy server
func (ps *ProxyServer) Start() error {
	if ps.running {
		return nil
	}

	if ps.reqParser == nil {
		ps.reqParser = &ParserChain{}
	}
	if err := ps.reqParser.Init(); err != nil {
		return err
	}

	if ps.executor == nil {
		ps.executor = NewExecutor(ps.dockerdHost, 2375)
	}

	if ps.packer == nil {
		ps.packer = NewPacker(ps.dockerdHost, 2375, "10.112.122.204")
	}

	if ps.proxy == nil {
		t := &http.Transport{
			//Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   60 * time.Second,
				KeepAlive: 60 * time.Second,
			}).DialContext,
			MaxIdleConnsPerHost:   100,
			MaxIdleConns:          100,
			IdleConnTimeout:       120 * time.Second,
			TLSHandshakeTimeout:   20 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
		ps.proxy = &httputil.ReverseProxy{
			Transport: t,
			Director: func(req *http.Request) {
				log.Printf("PROXY: %s %s\n", req.Method, req.URL.String())
				//Parse request
				if ps.reqParser != nil {
					meta, err := ps.reqParser.Parse(req)
					if err != nil {
						log.Fatalf("Parse error: %s\n", err)
						return
					}

					if meta.HasHit {
						log.Printf("Proxy traffic: %#v\n", meta)
						runtime, err := ps.executor.Exec(meta)
						if err != nil {
							log.Fatalf("Exec error: %s\n", err)
							return
						}
						/*runtime := Environment{
							Target:    "10.160.162.129:52198",
							RuntimeID: "594ab4ebd69bfc6edc7520949c3e4f76c445edda8dd00a3c1ade799d49d63d55",
						}*/

						log.Printf("Exec runtime: %#v\n", runtime)
						//Proxy
						target, err := url.Parse(fmt.Sprintf("%s%s", "http://", runtime.Target))
						if err != nil {
							log.Fatalf("Url parse error: %s\n", err)
							return
						}
						targetQuery := target.RawQuery
						req.URL.Scheme = target.Scheme
						req.URL.Host = target.Host
						req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
						if targetQuery == "" || req.URL.RawQuery == "" {
							req.URL.RawQuery = targetQuery + req.URL.RawQuery
						} else {
							req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
						}
						if _, ok := req.Header["User-Agent"]; !ok {
							// explicitly disable User-Agent so it's not set to default value
							req.Header.Set("User-Agent", "")
						}
						//Add additional info
						req.Header.Set("harbor-runtime", runtime.RuntimeID)
						req.Header.Set("runtime-stage", meta.RequestStage)
						req.Header.Add("container-image", meta.Image)
						req.Header.Add("container-image", meta.Tag)
					}
				}
				//do nothing
			},

			ModifyResponse: func(res *http.Response) error {
				log.Printf("ModifyResponse: %#v\n", res.Header)

				runtimeID := strings.TrimSpace(res.Request.Header.Get("harbor-runtime"))
				defer func() {
					if len(runtimeID) > 0 {
						if err := ps.executor.Destroy(runtimeID); err != nil {
							log.Fatalf("Destroy runtime error: %s\n", err)
						} else {
							log.Printf("Destroy runtime: %s\n", runtimeID)
						}
					}
				}()
				image := strings.TrimSpace(res.Request.Header.Get("container-image"))
				runtimeStage := strings.TrimSpace(res.Request.Header.Get("runtime-stage"))
				if runtimeStage == requestStageSession {
					//Build
					if err := ps.packer.BuildLocal(runtimeID, image, ""); err != nil {
						return err
					}
				}
				if runtimeStage == requestStagePack {
					if err := ps.packer.Build(runtimeID, "experiment-npm-package", fmt.Sprintf("v%d", time.Now().UnixNano())); err != nil {
						return err
					}
				}

				return nil
			},
		}
	}

	if ps.server == nil {
		ps.server = &http.Server{
			Addr: fmt.Sprintf("%s:%d", ps.host, ps.port),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ps.proxy.ServeHTTP(w, r)
			}),
		}
	}

	return ps.server.ListenAndServe()
}

//Stop the proxy server
func (ps *ProxyServer) Stop(ctx context.Context) error {
	if !ps.running {
		return nil
	}

	if ps.server == nil {
		return errors.New("No server existing")
	}

	err := ps.server.Shutdown(ctx)
	if err == nil {
		ps.running = false
	}

	return err
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
