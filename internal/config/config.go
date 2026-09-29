package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf
	Database struct {
		DSN string
	}
	Audio struct {
		Endpoint  string
		AccessKey string
		SecretKey string
		Bucket    string
		UseSSL    bool
	}
	App struct {
		DatasetVersion string
		// RequireAuth protects filtered catalog queries, poem details, and audio
		// playback with the existing SSO session middleware. Keep this false to
		// allow anonymous reading while retaining the login endpoints.
		RequireAuth bool
	}
	OIDC struct {
		Issuer        string
		ClientID      string
		ClientSecret  string
		RedirectURL   string
		SessionSecret string
		CookieName    string
		AdminEmails   string
	}
	Stats struct {
		InternalURL string
		Service     string
		Secret      string
	}
}
