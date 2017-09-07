package main

import "fmt"
import "encoding/base64"
import "crypto/sha512"
import "flag"
import "os"

func AgentPasswordHash(password string) string {
	sum := sha512.New()
	sum.Write([]byte(password))
	h := sum.Sum(nil)
	return base64.StdEncoding.EncodeToString(h[:18])
}

func main() {

	var password = flag.String("p", "", "Generate hash for this password (required).")

	flag.Parse()

	if *password == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	fmt.Println(AgentPasswordHash(*password))
}
