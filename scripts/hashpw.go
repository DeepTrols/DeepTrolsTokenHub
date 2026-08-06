//go:build ignore

package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	h, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	fmt.Print(string(h))
}
