package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"wsproxy/internal/logging"
	"wsproxy/test/e2e/app/security/authn"
)

// PasswordCredentials holds password-credentials
type PasswordCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UsersByRoles maps roles to lists of user holding the role
type UsersByRoles map[string][]string

// Options holds the available command-line options
type Options struct {
	ServerHostname              string                     `env:"SERVER_HOSTNAME" long:"server-hostname" short:"h" default:"localhost" description:"Server hostname"`
	ServerPort                  int                        `env:"SERVER_PORT" long:"server-port" short:"p" default:"8080" description:"Server port"`
	ServerURLContext            string                     `env:"SERVER_URL_CONTEXT" long:"server-url-context" short:"c" default:"" description:"Server url context"`
	SessionMaxAge               int                        `env:"SESSION_MAX_AGE" long:"session-max-age" short:"s" default:"86400" description:"The maximum age in secods of a user's session"`
	LoadBalancerAddress         string                     `env:"LOAD_BALANCER_ADDRESS" long:"load-balancer-address" short:"" default:"" description:"The load balancer address patter"`
	AppDescription              string                     `env:"APP_DESCRIPTION" long:"app-description" short:"" default:"" description:"Application description"`
	AuthenticationType          authn.AuthenticationScheme `env:"AUTHENTICATION_TYPE" long:"authentication-type" short:"a" default:"oidc" description:"Authentication type"`
	PasswordCredentials_string  string                     `env:"PASSWORD_CREDENTIALS" long:"password-credentials"`
	PasswordCredentials         []PasswordCredentials
	OIDCClientID                string `env:"OIDC_CLIENT_ID" long:"oidc-client-id" short:"" default:"" description:"OIDC client id"`
	OIDCClientSecret            string `env:"OIDC_CLIENT_SECRET" long:"oidc-client-secret" short:"" default:"" description:"OIDC client secret"`
	OIDCAccessTokenURL          string `env:"OIDC_ACCESS_TOKEN_URL" long:"oidc-access-token-url" short:"" default:"" description:"OIDC access token url"`
	OIDCUserAuthorizationURL    string `env:"OIDC_USER_AUTHORIZATION_URL" long:"oidc-user-authorization-url" short:"" default:"" description:"OIDC user authorization url"`
	OIDCClientRedirectBackURL   string `env:"OIDC_CLIENT_REDIRECT_BACK_URL" long:"oidc-client-redirect-back-url" short:"" default:"" description:"OIDC client redirect back url"`
	OIDCTokenIssuer             string `env:"OIDC_TOKEN_ISSUER" long:"oidc-token-issuer" short:"" default:"" description:"OIDC token issuer"`
	OIDCLogoutURL               string `env:"OIDC_LOGOUT_URL" long:"oidc-logout-url" short:"" default:"" description:"OIDC logout URL"`
	OIDCIpJwtPublicKeyURL       string `env:"OIDC_IP_JWT_PUBLIC_KEY_URL" long:"oidc-ip-jwt-public-key-url" short:"" default:"" description:"OIDC ip jwt public key url"`
	OIDCIpJwtPublicKeyPemBase64 string `env:"OIDC_IP_JWT_PUBLIC_KEY_PEM_BASE64" long:"oidc-ip-jwt-public-key-pem-base64" short:"" default:"" description:"OIDC ip jwt public key pem base64"`
	OIDCIpLogoutURL             string `env:"OIDC_IP_LOGOUT_URL" long:"oidc-ip-logout-url" short:"" default:"" description:"OIDC ip logout url"`
	UsersByRoles_string         string `env:"USERS_BY_ROLES" long:"users-by-roles" short:"" default:"" description:"Users by roles"`
	UsersByRoles                UsersByRoles
	SessionDbName               string `env:"SESSION_DB_NAME" long:"session-db-name" short:"" default:"" description:"Name of the session DB"`
	DBHost                      string `env:"DB_HOST" long:"db-host" short:"" default:"localhost" description:"DB host"`
	DBPort                      int    `env:"DB_PORT" long:"db-port" short:"" default:"5432" description:"DB port"`
	DBName                      string `json:"dbName" env:"DB_NAME" long:"db-name" short:"" default:"iconrepo" description:"Name of the database"`
	DBUser                      string `env:"DB_USER" long:"db-user" short:"" default:"iconrepo" description:"DB user"`
	DBPassword                  string `env:"DB_PASSWORD" long:"db-password" short:"" default:"iconrepo" description:"DB password"`
	EnableBackdoors             bool   `env:"ENABLE_BACKDOORS" long:"enable-backdoors" short:"" description:"Enable backdoors"`
	UsernameCookie              string `env:"USERNAME_COOKIE" long:"username-cookie" short:"" description:"The name of the cookie, if any, carrying username. Only OIDC for now."`
	LogLevel                    string `env:"LOG_LEVEL" long:"log-level" short:"l" default:"info"`
	AllowedClientURLsRegex      string `env:"ALLOWED_CLIENT_URLS_REGEX" long:"allowed-client-urls-regex" short:"" default:""`
	DynamodbURL                 string `env:"DYNAMODB_URL" long:"dynamodb-url" short:"" default:""`
}

var DefaultIconRepoHome = filepath.Join(os.Getenv("HOME"), ".ui-toolbox/iconrepo")
var DefaultIconDataLocationGit = filepath.Join(DefaultIconRepoHome, "git-repo")
var DefaultConfigFilePath = filepath.Join(DefaultIconRepoHome, "config.json")

type ConfigFilePath string

func findCliArg(name string, clArgs []string) string {
	for index, arg := range clArgs {
		if arg == name {
			return clArgs[index+1]
		}
	}
	return ""
}

func ParseCommandLineArgs(clArgs []string) Options {
	logger := logging.Get().With().Str(logging.UnitLogger, "config").Logger()

	opts := Options{}
	psResult := reflect.ValueOf(&opts)
	t := reflect.TypeOf(opts)
	for fieldIndex := 0; fieldIndex < t.NumField(); fieldIndex++ {
		field := t.FieldByIndex([]int{fieldIndex})
		fWritable := psResult.Elem().FieldByName(field.Name)
		if !fWritable.IsValid() || !fWritable.CanSet() {
			panic(fmt.Sprintf("Field %v is not valid (%v) or not writeable (%v).", field.Name, fWritable.IsValid(), fWritable.CanAddr()))
		}
		longopt := fmt.Sprintf("--%s", field.Tag.Get("long"))
		if cliArg := findCliArg(longopt, clArgs); cliArg != "" {
			logger.Debug().Str("longopt", longopt).Send()
			err := setValueFromString(cliArg, fWritable)
			if err != nil {
				panic(fmt.Errorf("failed to set command-line argument value %v=%v: %w", longopt, cliArg, err))
			}
			continue
		}
		envName := field.Tag.Get("env")
		if envVal := os.Getenv(envName); envVal != "" {
			err := setValueFromString(envVal, fWritable)
			if err != nil {
				panic(fmt.Errorf("failed to set environment variable value %v=%v: %w", envName, envVal, err))
			}
			continue
		}
		if dfltValue := field.Tag.Get("default"); dfltValue != "" {
			err := setValueFromString(dfltValue, fWritable)
			if err != nil {
				panic(fmt.Errorf("failed to set default value %v=%v: %w", envName, dfltValue, err))
			}
			continue
		}
	}

	if len(opts.PasswordCredentials_string) > 0 {
		var pwcreds []PasswordCredentials
		parseJson(opts.PasswordCredentials_string, &pwcreds)
		opts.PasswordCredentials = pwcreds
	}

	if len(opts.UsersByRoles_string) > 0 {
		var usersByRoles UsersByRoles
		parseJson(opts.UsersByRoles_string, &usersByRoles)
		opts.UsersByRoles = usersByRoles
	}

	logger.Info().Any("parsed config", opts).Send()

	return opts
}

func parseJson(value string, parsed any) {
	unmarshalError := json.Unmarshal([]byte(value), parsed)
	if unmarshalError != nil {
		panic(fmt.Sprintf("failed to parse %s: %#v\n", value, unmarshalError))
	}
}

func setValueFromString(value string, target reflect.Value) error {
	switch target.Kind() {
	case reflect.Int:
		{
			val, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			x := int64(val)
			if target.OverflowInt(x) {
				return fmt.Errorf("overflow error")
			}
			target.SetInt(x)
		}
	case reflect.String:
		{
			target.SetString(value)
		}
	case reflect.Bool:
		{
			var x bool
			switch value {
			case "true":
				x = true
			case "false":
				x = false
			default:
				return fmt.Errorf("expected 'true' or 'false', found: %s", value)
			}
			target.SetBool(x)
		}
	default:
		return fmt.Errorf("unexpected property type: %v", target.Kind())
	}
	return nil
}
