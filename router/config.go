package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type EndpointConfig struct {
	Domain         string   `toml:"domain"`
	Methods        []string `toml:"methods"`
	Paths          []string `toml:"paths"`
	Ports          []int    `toml:"ports"`
	AllowPlainHTTP bool     `toml:"allow_plain_http"`
}

type CredentialConfig struct {
	Header string `toml:"header"`
	Env    string `toml:"env"`
}

type ServiceConfig struct {
	Endpoints   []EndpointConfig  `toml:"endpoints"`
	Credentials *CredentialConfig `toml:"credentials"`
}

type Config struct {
	Services map[string]ServiceConfig `toml:"services"`
}

type Endpoint struct {
	Domain         string
	Methods        []string
	Paths          []string
	Ports          []int
	AllowPlainHTTP bool
}

type CredentialRule struct {
	Domain string
	Header string
	Env    string
}

type Policy struct {
	Endpoints   []Endpoint
	Credentials []CredentialRule
}

func LoadConfig(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("config file not found, using empty config", "path", path)
			return &Policy{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	p := &Policy{}
	for name, svc := range cfg.Services {
		for _, ep := range svc.Endpoints {
			methods := make([]string, len(ep.Methods))
			for i, m := range ep.Methods {
				methods[i] = strings.ToUpper(m)
			}
			p.Endpoints = append(p.Endpoints, Endpoint{
				Domain:         ep.Domain,
				Methods:        methods,
				Paths:          ep.Paths,
				Ports:          ep.Ports,
				AllowPlainHTTP: ep.AllowPlainHTTP,
			})
			if svc.Credentials != nil {
				p.Credentials = append(p.Credentials, CredentialRule{
					Domain: ep.Domain,
					Header: svc.Credentials.Header,
					Env:    svc.Credentials.Env,
				})
			}
		}
		slog.Info("loaded service", "name", name, "endpoints", len(svc.Endpoints))
	}

	slog.Info("config loaded", "endpoints", len(p.Endpoints), "credential_rules", len(p.Credentials))
	return p, nil
}

func domainMatch(pattern, host string) bool {
	return fnmatch(pattern, host)
}
