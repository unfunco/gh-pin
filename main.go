package main

import (
	"fmt"

	"github.com/cli/go-gh"
)

func main() {
	args := []string{"api", "user", "--jq", `"@\(.login) (\(.name))"`}
	identity, _, err := gh.Exec(args...)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(identity.String())
}
