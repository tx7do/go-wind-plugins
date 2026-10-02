package casbin

import (
	"fmt"

	"github.com/casbin/casbin/v2/model"
	windlog "github.com/tx7do/go-wind/log"

	engine "github.com/tx7do/go-wind-plugins/security/authz"
	"github.com/tx7do/go-wind-plugins/security/authz/casbin/assets"
)

type OptFunc func(*State)

func WithModel(model model.Model) OptFunc {
	return func(s *State) {
		// 后设置的模型选项覆盖先前的（含先前记录的失败）。
		s.modelOptErr = nil
		s.model = model
	}
}

func WithStringModel(str string) OptFunc {
	return func(s *State) {
		s.modelOptErr = nil
		m, err := model.NewModelFromString(str)
		if err != nil {
			// OptFunc 无法返回错误：记录下来，由 NewEngine 冒泡返回，
			// 不再静默回退到默认模型。
			s.modelOptErr = err
			return
		}
		s.model = m
	}
}

func WithFileModel(path string) OptFunc {
	return func(s *State) {
		s.modelOptErr = nil
		m, err := model.NewModelFromFile(path)
		if err != nil {
			// 同 WithStringModel：记录错误，由 NewEngine 冒泡返回。
			s.modelOptErr = err
			return
		}
		s.model = m
	}
}

func WithDefaultModel(name string) OptFunc {
	return func(s *State) {
		s.modelOptErr = nil
		var str string
		switch name {
		case "rbac":
			str = assets.DefaultRbacModel

		case "rbac_with_domains":
			str = assets.DefaultRbacWithDomainModel

		case "abac":
			str = assets.DefaultAbacModel

		case "acl":
			str = assets.DefaultAclModel

		case "restfull":
			str = assets.DefaultRestfullModel

		case "restfull_with_role":
			str = assets.DefaultRestfullWithRoleModel
		}

		if str == "" {
			s.modelOptErr = fmt.Errorf("casbin: unknown default model %q", name)
			return
		}

		m, err := model.NewModelFromString(str)
		if err != nil {
			// 同 WithStringModel：记录错误，由 NewEngine 冒泡返回。
			s.modelOptErr = err
			return
		}
		s.model = m
	}
}

func WithPolicyAdapter(policy *Adapter) OptFunc {
	return func(s *State) {
		s.policy = policy
	}
}

func WithPolices(policies map[string]interface{}) OptFunc {
	return func(s *State) {
		if s.policy == nil {
			s.policy = newAdapter()
		}
		s.policy.SetPolicies(policies)
	}
}

func WithProjects(projects engine.Projects) OptFunc {
	return func(s *State) {
		s.projects = projects
	}
}

func WithWildcardItem(item string) OptFunc {
	return func(s *State) {
		s.wildcardItem = item
	}
}

func WithAuthorizedProjectsMatcher(matcher string) OptFunc {
	return func(s *State) {
		s.authorizedProjectsMatcher = matcher
	}
}

func WithLogger(logger windlog.Logger) OptFunc {
	return func(s *State) {
		s.log = logger.With("module", "casbin.authz.engine")
	}
}
