package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
)

const dummyPrefix = "SUISOU__"

var markerRe = regexp.MustCompile(dummyPrefix + `(\w+)`)

func (p *Policy) PlainHTTPAllowed(host string) bool {
	for _, ep := range p.Endpoints {
		if ep.AllowPlainHTTP && domainMatch(ep.Domain, host) {
			return true
		}
	}
	return false
}

func (p *Policy) EndpointAllowed(host, method, path string, port int) bool {
	for _, ep := range p.Endpoints {
		if endpointMatches(ep, host, method, path, port) {
			return true
		}
	}
	return false
}

func endpointMatches(ep Endpoint, host, method, path string, port int) bool {
	if !domainMatch(ep.Domain, host) {
		return false
	}
	if len(ep.Methods) > 0 {
		found := false
		for _, m := range ep.Methods {
			if m == method {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(ep.Paths) > 0 {
		found := false
		for _, pat := range ep.Paths {
			if fnmatch(pat, path) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(ep.Ports) > 0 {
		found := false
		for _, pp := range ep.Ports {
			if pp == port {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (p *Policy) InjectCredentials(req *http.Request, host string) {
	for _, rule := range p.Credentials {
		if !domainMatch(rule.Domain, host) {
			continue
		}
		current := req.Header.Get(rule.Header)
		marker := dummyPrefix + rule.Env
		if !strings.Contains(current, marker) {
			continue
		}
		real := os.Getenv(rule.Env)
		if real != "" {
			req.Header.Set(rule.Header, strings.Replace(current, marker, real, 1))
		} else {
			slog.Warn("env var not set for credential injection", "env", rule.Env, "host", host)
		}
	}
}

func (p *Policy) AllowedEnvsForHost(host string) map[string]struct{} {
	envs := make(map[string]struct{})
	for _, rule := range p.Credentials {
		if domainMatch(rule.Domain, host) {
			envs[rule.Env] = struct{}{}
		}
	}
	return envs
}

func (p *Policy) ReplaceMarkers(text string, allowedEnvs map[string]struct{}) string {
	return markerRe.ReplaceAllStringFunc(text, func(match string) string {
		envName := strings.TrimPrefix(match, dummyPrefix)
		if _, ok := allowedEnvs[envName]; !ok {
			return match
		}
		if val := os.Getenv(envName); val != "" {
			return val
		}
		return match
	})
}

func blockResponse(status int, msg string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Content-Type": {"text/plain"}},
		Body:       http.NoBody,
	}
}

