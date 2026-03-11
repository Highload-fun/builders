package main

import (
	"crypto/md5"
	"fmt"
)

func main() {
	hash := md5.Sum([]byte("hello"))
	fmt.Printf("%x\n", hash)
}
