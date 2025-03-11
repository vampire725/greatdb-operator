package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

const (
	key string = "greatk8@op-db!1l"
)

// PKCS7
func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS7UnPadding(origData []byte) ([]byte, error) {
	length := len(origData)
	unpadding := int(origData[length-1])
	if unpadding > length {
		return nil, fmt.Errorf("Decoding failed")
	}
	return origData[:(length - unpadding)], nil
}

func AesCBCEncrypt(rawData []byte) ([]byte, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()
	rawData = PKCS7Padding(rawData, blockSize)

	cipherText := make([]byte, blockSize+len(rawData))

	iv := cipherText[:blockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherText[blockSize:], rawData)

	return cipherText, nil
}

func AesCBCDncrypt(encryptData []byte) ([]byte, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()

	if len(encryptData) < blockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	iv := encryptData[:blockSize]
	encryptData = encryptData[blockSize:]

	// CBC mode always works in whole blocks.
	if len(encryptData)%blockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	// CryptBlocks can work in-place if the two arguments are the same.
	mode.CryptBlocks(encryptData, encryptData)

	encryptData, err = PKCS7UnPadding(encryptData)
	if err != nil {
		return nil, err
	}
	return encryptData, nil
}

func EncryptBase64(rawData string) (string, error) {
	data, err := AesCBCEncrypt([]byte(rawData))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func DecryptBase64(data string) (string, error) {

	v, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	pwd, err := AesCBCDncrypt([]byte(v))
	if err != nil {
		return "", err
	}

	return string(pwd), nil
}
