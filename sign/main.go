package main

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

func main() {
	var p url.Values
	p = make(map[string][]string, 0)
	p["cc"] = []string{"333"}
	p["www"] = []string{"34444"}
	p["aaa"] = []string{"33222"}
	p["sign"] = []string{"wefwef"}
	sign := getSign(p, "aaaaa", "sign")
	fmt.Println(sign)
}

func getSign(params url.Values, secret string, filter string) string {
	encode := EncodeQuery(params, filter)
	fmt.Println(encode)
	encode += "&AppSecret=" + secret
	encodeMD5 := strings.ToUpper(EncodeMD5(encode))
	hmacsha1 := HMACSHA1(secret, encodeMD5)
	return hmacsha1
}

// Base64Encode base64 encode
func Base64Encode(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

// Base64Decode base64 decode
func Base64Decode(str string) (string, error) {
	s, e := base64.StdEncoding.DecodeString(str)
	return string(s), e
}

// EncodeMD5 md5 encryption
func EncodeMD5(value string) string {
	m := md5.New()
	m.Write([]byte(value))
	return hex.EncodeToString(m.Sum(nil))
}

func HMACSHA1(keyStr, value string) string {
	key := []byte(keyStr)
	mac := hmac.New(sha1.New, key)
	mac.Write([]byte(value))
	//进行base64编码
	res := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return res
}

func EncodeQuery(v url.Values, filter string) string {
	if v == nil {
		return ""
	}
	var buf strings.Builder
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == filter {
			continue
		}
		vs := v[k]
		for _, v := range vs {
			if buf.Len() > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(k)
			buf.WriteByte('=')
			buf.WriteString(v)
		}
	}
	return buf.String()
}
