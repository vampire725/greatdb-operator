package base64

import (
	"encoding/base64"
)

func Encode(value string) string {

	// Base64 Standard Encoding
	return base64.StdEncoding.EncodeToString([]byte(value))

}

func Decode(value string) (string, error) {

	// Base64 Standard Decoding
	v, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}

	return string(v), nil
}
