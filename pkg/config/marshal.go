package config

import "gopkg.in/yaml.v2"

func (config Server) MarshalYAML() ([]byte, error) {
	const sensitive = "<sensitive>"
	if config.Auth.JWTSecret != "" {
		config.Auth.JWTSecret = sensitive
	}
	if config.Auth.Github.ClientSecret != "" {
		config.Auth.Github.ClientSecret = sensitive
	}
	if config.Auth.Google.ClientSecret != "" {
		config.Auth.Google.ClientSecret = sensitive
	}
	if config.Auth.Gitlab.ClientSecret != "" {
		config.Auth.Gitlab.ClientSecret = sensitive
	}
	if config.Auth.Internal.AdminUser.Password != "" {
		config.Auth.Internal.AdminUser.Password = sensitive
	}
	return yaml.Marshal(config)
}
