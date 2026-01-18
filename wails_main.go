package main

import (
	"GusSync/app"
)

func main() {
	if err := app.Run(); err != nil {
		panic(err)
	}
}

