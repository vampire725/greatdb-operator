package base64

import (
	"fmt"
	"testing"
)

func TestEncode(t *testing.T) {

	value := Encode("123456")
	fmt.Println(value)

}

func TestDecode(t *testing.T) {
	value, err := Decode("MTIzNDU2")
	if err != nil {
		t.Fatalf("%s", err.Error())
	}
	fmt.Println(value)
}
