package jwt

import "time"

type JwtClient interface {
	Generate(userId uint64) (accessToken, refreshToken string, expiredAt uint64, err error)
	GenerateBySubject(subject string) (accessToken, refreshToken string, expiredAt uint64, err error)
	VerifyAccessToken(tokenStr string) (userId string, expiredAt time.Time, tokenId string, err error)
	VerifyRefreshToken(tokenStr string) (userId string, expiredAt time.Time, tokenId string, err error)
	AccessToken(userId string) (token string, expiredAt time.Time, err error)
	RefreshToken(userId string) (token string, expiredAt time.Time, err error)
}
