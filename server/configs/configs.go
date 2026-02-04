package configs

type ServerConfig struct {
	Host string `json:"host,omitempty"`
	Port string `json:"port,omitempty"`
	OIDC OIDC   `json:"oidc"`
}

type OIDC struct {
	ClientId        string `yaml:"clientId,omitempty"`
	ClientSecret    string `yaml:"clientSecret,omitempty"`
	FailRedirectUrl string `yaml:"failRedirectUrl,omitempty"`
	PassRedirectUrl string `yaml:"passRedirectUrl,omitempty"`
	ProviderUrl     string `yaml:"providerUrl,omitempty"`
	Timeout         uint32 `yaml:"timeout,omitempty"`
	JWTSecret       string `yaml:"jwtSecret,omitempty"`
	RedirectUrl     string `yaml:"redirectUrl,omitempty"`
}
