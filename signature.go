package main

import (
	"crypto/md5"
	"fmt"
	"time"
)

const (
	appCode   = "cosy"
	secretB64 = "d2FyLCB3YXIgbmV2ZXIgY2hhbmdlcw==" // base64("war, war never changes")
	sep       = "&"
)

func currentDate() string {
	return time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
}

func sign(date string) string {
	return md5Hex(appCode + sep + secretB64 + sep + date)
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}
