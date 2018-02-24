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
	HarborHost  string
	HarborProto string
}

//ProxyServer serves the requests
type ProxyServer struct {
	server      *http.Server
	proxy       *httputil.ReverseProxy
	host        string
	port        int
	dockerdHost string
	harbor      string
	harborProto string
	running     bool
	reqParser   *ParserChain
	scheduler   *Scheduler
}

//NewProxyServer create new server instance
func NewProxyServer(config ServerConfig) *ProxyServer {
	return &ProxyServer{
		host:        config.Host,
		port:        config.Port,
		dockerdHost: config.DockerdHost,
		harbor:      config.HarborHost,
		harborProto: config.HarborProto,
	}
}

//Start the proxy server
func (ps *ProxyServer) Start(ctx context.Context) error {
	if ps.running {
		return nil
	}

	if ps.reqParser == nil {
		ps.reqParser = &ParserChain{}
	}
	if err := ps.reqParser.Init(); err != nil {
		return err
	}

	if ps.scheduler == nil {
		sConfig := SchedulerConfig{
			DockerHost: ps.dockerdHost,
			HPort:      2375,
			Harbor:     ps.harbor,
		}
		ps.scheduler = NewScheduler(ctx, sConfig)
		ps.scheduler.Start()
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
						var rawTarget string
						if meta.RegistryType == registryTypeNpm {
							env, err := ps.scheduler.Schedule(meta)
							if err != nil {
								log.Fatalf("schedule error: %s\n", err)
								return
							}
							rawTarget = fmt.Sprintf("%s%s", "http://", env.Target)

							if env.Rebuild != nil {
								h, err := env.Rebuild.Encode()
								if err != nil {
									log.Fatalf("set rebuild header failed: %s", err)
									return
								}
								req.Header.Set("registry-factory", h)
							}
						} else {
							//Treat as harbor
							rawTarget = fmt.Sprintf("%s://%s", ps.harborProto, ps.harbor)
						}

						target, err := url.Parse(rawTarget)
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
					}
				}
				//do nothing
			},

			ModifyResponse: func(res *http.Response) error {
				rebuildPolicyHeader := res.Request.Header.Get("registry-factory")
				if len(rebuildPolicyHeader) > 0 {
					rebuildPolicy := &BuildPolicy{}
					err := rebuildPolicy.Decode(rebuildPolicyHeader)
					if err != nil {
						return err
					}

					return ps.scheduler.Rebuild(rebuildPolicy)
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
