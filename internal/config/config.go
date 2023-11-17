package config

type Config struct {
	ServerHost          string
	AppBaseUrl          string
	LoadBalancerAddress string // TODO: remove this
	ServerPort          int
}

func GetConfig(args []string) Config {
	return Config{}
}
