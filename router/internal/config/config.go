package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"suisou/router/internal/policy"

	"github.com/BurntSushi/toml"
)

type endpointConfig struct {
	Domain         string   `toml:"domain"`
	Methods        []string `toml:"methods"`
	Paths          []string `toml:"paths"`
	Ports          []int    `toml:"ports"`
	AllowPlainHTTP bool     `toml:"allow_plain_http"`
}

type credentialConfig struct {
	Header string `toml:"header"`
	Env    string `toml:"env"`
}

type serviceConfig struct {
	Endpoints   []endpointConfig  `toml:"endpoints"`
	Credentials *credentialConfig `toml:"credentials"`
}

type fileConfig struct {
	Services map[string]serviceConfig `toml:"services"`
}

func LoadPolicy(path string) (*policy.Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("config file not found, using empty config", "path", path)
			return &policy.Policy{}, nil
		}
		return nil, err
	}

	var cfg fileConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	result := &policy.Policy{}
	for name, svc := range cfg.Services {
		for _, ep := range svc.Endpoints {
			methods := make([]string, len(ep.Methods))
			for i, m := range ep.Methods {
				methods[i] = strings.ToUpper(m)
			}

			compiledEndpoint, err := policy.NewEndpoint(policy.EndpointSpec{
				Domain:         ep.Domain,
				Methods:        methods,
				Paths:          ep.Paths,
				Ports:          ep.Ports,
				AllowPlainHTTP: ep.AllowPlainHTTP,
			})
			if err != nil {
				return nil, fmt.Errorf("service %q endpoint %q: %w", name, ep.Domain, err)
			}
			result.Endpoints = append(result.Endpoints, compiledEndpoint)

			if svc.Credentials != nil {
				domainMatcher, err := policy.NewMatcher(ep.Domain)
				if err != nil {
					return nil, fmt.Errorf("service %q credential domain %q: %w", name, ep.Domain, err)
				}
				result.Credentials = append(result.Credentials, policy.CredentialRule{
					Domain:  ep.Domain,
					Matcher: domainMatcher,
					Header:  svc.Credentials.Header,
					Env:     svc.Credentials.Env,
				})
			}
		}
		slog.Info("loaded service", "name", name, "endpoints", len(svc.Endpoints))
	}

	slog.Info("config loaded", "endpoints", len(result.Endpoints), "credential_rules", len(result.Credentials))
	return result, nil
}
