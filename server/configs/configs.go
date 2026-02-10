package configs

type ServerConfig struct {
	Host      string `yaml:"host,omitempty"`
	Port      string `yaml:"port,omitempty"`
	JWTSecret string `yaml:"jwtSecret,omitempty"`
}

type OIDC struct {
	ClientId        string `json:"clientId,omitempty"`
	ClientSecret    string `json:"clientSecret,omitempty"`
	FailRedirectUrl string `json:"failRedirectUrl,omitempty"`
	PassRedirectUrl string `json:"passRedirectUrl,omitempty"`
	ProviderUrl     string `json:"providerUrl,omitempty"`
	Timeout         uint32 `json:"timeout,omitempty"`
	JWTSecret       string `json:"jwtSecret,omitempty"`
	RedirectUrl     string `json:"redirectUrl,omitempty"`
	AdminGroup      string `json:"adminGroup,omitempty"`
}
