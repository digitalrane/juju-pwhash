package main

import "fmt"
import "flag"
import "os"
import "github.com/juju/utils"

func main() {

	var password = flag.String("p", "", "Generate hash for this password (required).")
	var salt = flag.String("s", "", "If provided, generate a user hash instead of machine hash using this salt for this password (optional).")

	flag.Parse()

	if *password == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	if *salt == "" {
		fmt.Println(utils.AgentPasswordHash(*password))
	} else {
		fmt.Println(utils.UserPasswordHash(*password, *salt))
	}
}
