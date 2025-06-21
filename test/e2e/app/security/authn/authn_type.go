package authn

type AuthenticationScheme string

const (
	SchemeBasic     AuthenticationScheme = "basic"
	SchemeOIDC      AuthenticationScheme = "oidc"
	SchemeOIDCProxy AuthenticationScheme = "oidc-proxy"
)
