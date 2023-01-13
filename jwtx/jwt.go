package jwtx

import (
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
)

type JWT struct {
	Issuer     string
	nowFunc    func() time.Time
	SignMethod jwt.SigningMethod
}

//	==Payload默认7个字段==
//	Audience  接收方
//	ExpiresAt jwt过期时间
//	Id        jwt唯一身份标识
//	IssuedAt  签发时间
//	Issuer    签发人/发行人
//	NotBefore jwt生效时间
//	Subject   主题

type Claims struct {
	Extra     string `json:"extra"`
	ValidOnce bool   `json:"valid_once"`
	jwt.StandardClaims
}

// Valid 实现Claims接口,接管Payload参数校验方法
func (c Claims) Valid() error {
	vErr := new(jwt.ValidationError)
	now := jwt.TimeFunc().Unix()

	// 校验过期时间
	if c.VerifyExpiresAt(now, false) == false {
		delta := time.Unix(now, 0).Sub(time.Unix(c.ExpiresAt, 0))
		vErr.Inner = fmt.Errorf("token is expired by %v", delta)
		vErr.Errors |= jwt.ValidationErrorExpired
	}

	// 判断颁发时间是否晚于当前时间
	if c.VerifyIssuedAt(now, false) == false {
		vErr.Inner = fmt.Errorf("Token used before issued")
		vErr.Errors |= jwt.ValidationErrorIssuedAt
	}

	if c.VerifyNotBefore(now, false) == false {
		vErr.Inner = fmt.Errorf("token is not valid yet")
		vErr.Errors |= jwt.ValidationErrorNotValidYet
	}

	if vErr.Errors == 0 {
		return nil
	}

	return vErr
}

// VerifyIssuedAt 签发时间和当前时间比较逻辑，在这里修改
func (c Claims) VerifyIssuedAt(cmp int64, req bool) bool {
	if c.IssuedAt == 0 {
		return !req
	}
	iat := c.IssuedAt // 签发时间戳
	now := cmp        // 当前时间戳
	// 签发时间减去10s，容错
	return now >= iat-10
}

func NewJwt(issuer string, SignMethod jwt.SigningMethod) *JWT {
	return &JWT{
		Issuer:     issuer,
		nowFunc:    time.Now,
		SignMethod: SignMethod,
	}
}

// CreateToken 生成token
// extra 扩展字段，需要签名的信息，可以通过该字段传递
// accessID 用户唯一标识
// validOnce 一次有效：ture为一次有效，false为有效期内有效
func (j *JWT) CreateToken(accessID string, extra string, validOnce bool, expire time.Duration, prvKey interface{}) (string, error) {
	nowTime := j.nowFunc().Unix()
	var expireTime int64
	if expire > 0 {
		expireTime = nowTime + int64(expire.Seconds())
	}
	withClaims := jwt.NewWithClaims(j.SignMethod, Claims{
		Extra:     extra,
		ValidOnce: validOnce,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expireTime,
			IssuedAt:  nowTime,
			Issuer:    j.Issuer,
			Subject:   accessID,
		},
	})
	signedString, err := withClaims.SignedString(prvKey)
	return signedString, err
}

// ParseToken 解析token
func (j *JWT) ParseToken(token string, pubKey interface{}) (*Claims, error) {
	tokenClaims, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return pubKey, nil
	})

	if tokenClaims != nil {
		if claims, ok := tokenClaims.Claims.(*Claims); ok && tokenClaims.Valid {
			return claims, nil
		}
	}
	return nil, err
}
