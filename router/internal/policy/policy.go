package policy

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

const DummyPrefix = "SUISOU__"

var markerRe = regexp.MustCompile(DummyPrefix + `(\w+)`)

type Policy struct {
	Endpoints   []Endpoint
	Credentials []CredentialRule
}

type EndpointSpec struct {
	Domain         string
	Methods        []string
	Paths          []string
	Ports          []int
	AllowPlainHTTP bool
}

type Endpoint struct {
	Domain         string
	DomainMatcher  Matcher
	Methods        map[string]struct{}
	PathMatchers   []Matcher
	Ports          map[int]struct{}
	AllowPlainHTTP bool
}

type CredentialRule struct {
	Domain  string
	Matcher Matcher
	Header  string
	Env     string
}

func NewEndpoint(spec EndpointSpec) (Endpoint, error) {
	domainMatcher, err := NewMatcher(spec.Domain)
	if err != nil {
		return Endpoint{}, err
	}

	methods := make(map[string]struct{}, len(spec.Methods))
	for _, method := range spec.Methods {
		methods[strings.ToUpper(method)] = struct{}{}
	}

	pathMatchers := make([]Matcher, 0, len(spec.Paths))
	for _, path := range spec.Paths {
		matcher, err := NewMatcher(path)
		if err != nil {
			return Endpoint{}, err
		}
		pathMatchers = append(pathMatchers, matcher)
	}

	ports := make(map[int]struct{}, len(spec.Ports))
	for _, port := range spec.Ports {
		ports[port] = struct{}{}
	}

	return Endpoint{
		Domain:         spec.Domain,
		DomainMatcher:  domainMatcher,
		Methods:        methods,
		PathMatchers:   pathMatchers,
		Ports:          ports,
		AllowPlainHTTP: spec.AllowPlainHTTP,
	}, nil
}

func (p *Policy) PlainHTTPAllowed(host string) bool {
	for _, ep := range p.Endpoints {
		if ep.AllowPlainHTTP && ep.DomainMatcher.Match(host) {
			return true
		}
	}
	return false
}

func (p *Policy) EndpointAllowed(host, method, path string, port int) bool {
	for _, ep := range p.Endpoints {
		if ep.Match(host, method, path, port) {
			return true
		}
	}
	return false
}

func (ep Endpoint) Match(host, method, path string, port int) bool {
	if !ep.DomainMatcher.Match(host) {
		return false
	}
	if len(ep.Methods) > 0 {
		if _, ok := ep.Methods[method]; !ok {
			return false
		}
	}
	if len(ep.PathMatchers) > 0 {
		matched := false
		for _, matcher := range ep.PathMatchers {
			if matcher.Match(path) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(ep.Ports) > 0 {
		if _, ok := ep.Ports[port]; !ok {
			return false
		}
	}
	return true
}

func (p *Policy) InjectCredentials(req *http.Request, host string) {
	for _, rule := range p.Credentials {
		if !rule.Matcher.Match(host) {
			continue
		}
		current := req.Header.Get(rule.Header)
		marker := DummyPrefix + rule.Env
		if !strings.Contains(current, marker) {
			continue
		}
		real := os.Getenv(rule.Env)
		if real == "" {
			continue
		}
		req.Header.Set(rule.Header, strings.ReplaceAll(current, marker, real))
	}
}

func (p *Policy) AllowedEnvsForHost(host string) map[string]struct{} {
	envs := make(map[string]struct{})
	for _, rule := range p.Credentials {
		if rule.Matcher.Match(host) {
			envs[rule.Env] = struct{}{}
		}
	}
	return envs
}

func (p *Policy) ReplaceMarkers(text string, allowedEnvs map[string]struct{}) string {
	return markerRe.ReplaceAllStringFunc(text, func(match string) string {
		envName := strings.TrimPrefix(match, DummyPrefix)
		if _, ok := allowedEnvs[envName]; !ok {
			return match
		}
		if val := os.Getenv(envName); val != "" {
			return val
		}
		return match
	})
}

type Matcher struct {
	re *regexp.Regexp
}

func NewMatcher(pattern string) (Matcher, error) {
	re, err := regexp.Compile(globToRegexp(pattern))
	if err != nil {
		return Matcher{}, fmt.Errorf("invalid fnmatch pattern %q: %w", pattern, err)
	}
	return Matcher{re: re}, nil
}

func (m Matcher) Match(name string) bool {
	return m.re.MatchString(name)
}

func globToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '.', '+', '^', '$', '(', ')', '{', '}', '|', '\\':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		case '[':
			j := i + 1
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j < len(pattern) {
				b.WriteString(pattern[i : j+1])
				i = j
			} else {
				b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			}
		default:
			b.WriteByte(pattern[i])
		}
	}
	b.WriteString("$")
	return b.String()
}
