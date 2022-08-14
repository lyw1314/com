package com

import (
	"bytes"
	"math/rand"
	"time"
)

// RandString 取得随机字符包含数字、大小写等，可以自己随意扩展。
func RandString(l int) string {
	var inibyte []byte
	var result bytes.Buffer
	for i := 48; i < 123; i++ {
		switch {
		case i < 58:
			inibyte = append(inibyte, byte(i))
		case i >= 97 && i < 123:
			inibyte = append(inibyte, byte(i))
		}
	}
	var temp byte
	for i := 0; i < l; {
		if inibyte[randInt(0, len(inibyte))] != temp {
			temp = inibyte[randInt(0, len(inibyte))]
			result.WriteByte(temp)
			i++
		}
	}

	return result.String()
}

func randInt(min int, max int) byte {
	rand.Seed(time.Now().UnixNano())
	return byte(min + rand.Intn(max-min))
}
