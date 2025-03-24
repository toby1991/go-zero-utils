package jwt

type JwtConf struct {
	Secret                       string
	AccessTokenPrefix            string
	AccessTokenExpiredInSeconds  uint64
	RefreshTokenPrefix           string
	RefreshTokenExpiredInSeconds uint64
}
