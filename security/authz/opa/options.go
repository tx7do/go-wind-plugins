package opa

import (
	"context"

	"github.com/open-policy-agent/opa/ast"
	windlog "github.com/tx7do/go-wind/log"
)

type OptFunc func(*State)

func WithRegoVersion(version string) OptFunc {
	return func(s *State) {
		switch version {
		case "v0":
			s.regoVersion = ast.RegoV0

		case "v0v1":
			s.regoVersion = ast.RegoV0CompatV1

		default:
			fallthrough
		case "v1":
			s.regoVersion = ast.RegoV1
		}
	}
}

func WithLogger(logger windlog.Logger) OptFunc {
	return func(s *State) {
		s.log = logger.With("module", "opa.authz.engine")
	}
}

func WithEnableQueryTracer(enable bool) OptFunc {
	return func(s *State) {
		s.enableQueryTracer = enable
	}
}

func WithModules(mods map[string]*ast.Module) OptFunc {
	return func(s *State) {
		s.modules = mods
	}
}

func WithModulesFromFiles(modules map[string]string) OptFunc {
	return func(s *State) {
		if err := s.InitModulesFromFiles(modules); err != nil {
			s.log.Error(context.Background(), "failed to init modules from files", "error", err)
		}
	}
}

func WithModulesFromString(modules map[string]string) OptFunc {
	return func(s *State) {
		if err := s.InitModulesFromString(modules); err != nil {
			s.log.Error(context.Background(), "failed to init modules from string", "error", err)
		}
	}
}

func WithProjectsAuthorizedQuery(query string) OptFunc {
	return func(s *State) {
		s.authzProjectsQuery = query
	}
}

func WithFilterAuthorizedPairsQuery(query string) OptFunc {
	return func(s *State) {
		s.filteredPairsQuery = query
	}
}

func WithFilterAuthorizedProjectsQuery(query string) OptFunc {
	return func(s *State) {
		s.filteredProjectsQuery = query
	}
}
