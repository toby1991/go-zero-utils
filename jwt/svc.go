package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

func NewJwt(conf JwtConf) *jwtClient {
	return &jwtClient{
		secret:                       conf.Secret,
		accessTokenPrefix:            conf.AccessTokenPrefix,
		accessTokenExpiredInSeconds:  conf.AccessTokenExpiredInSeconds,
		refreshTokenPrefix:           conf.RefreshTokenPrefix,
		refreshTokenExpiredInSeconds: conf.RefreshTokenExpiredInSeconds,
	}
}

type jwtClient struct {
	secret                       string
	accessTokenPrefix            string
	accessTokenExpiredInSeconds  uint64
	refreshTokenPrefix           string
	refreshTokenExpiredInSeconds uint64
}

func (j *jwtClient) AccessToken(userId string) (string, time.Time, error) {
	return j.generate(j.accessTokenPrefix, userId, j.accessTokenExpiredInSeconds)
}
func (j *jwtClient) RefreshToken(userId string) (string, time.Time, error) {
	return j.generate(j.refreshTokenPrefix, userId, j.refreshTokenExpiredInSeconds)
}

func (j *jwtClient) Verify(tokenStr string) (userId string, expiredAt time.Time, tokenId string, err error) {
	// Parse takes the token string and a function for looking up the key. The latter is especially
	// useful if you use multiple keys for your application.  The standard is to use 'kid' in the
	// head of the token to identify which key to use, but the parsed token (head and claims) is provided
	// to the callback, providing flexibility.

	var claims jwt.RegisteredClaims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}

		// hmacSampleSecret is a []byte containing your secret, e.g. []byte("my_secret_key")
		return []byte(j.secret), nil
	})

	_ = token

	return claims.Subject, claims.ExpiresAt.Time, claims.ID, claims.Valid()
}

func (j *jwtClient) generate(idPrefix string, subject string, expiresInSeconds uint64) (string, time.Time, error) {
	now := time.Now()
	expiredAt := now.Add(time.Duration(expiresInSeconds) * time.Second)

	claim := jwt.RegisteredClaims{
		//Issuer:    "",
		Subject: subject,
		//Audience:  nil,
		ExpiresAt: jwt.NewNumericDate(expiredAt),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        idPrefix + uuid.New().String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	signedString, err := token.SignedString([]byte(j.secret))
	if err != nil {
		return "", time.Time{}, err
	}

	return signedString, expiredAt, nil
}
