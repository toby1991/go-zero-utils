package jwt

import "time"

type JwtClient interface {
	Verify(tokenStr string) (userId string, expiredAt time.Time, tokenId string, err error)
	AccessToken(userId string) (token string, expiredAt time.Time, err error)
	RefreshToken(userId string) (token string, expiredAt time.Time, err error)
}
