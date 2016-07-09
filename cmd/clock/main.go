package main

import (
	"fmt"
	"time"
)

func main() {
	ticker := time.NewTicker(time.Second * 60)
	for t := range ticker.C {
		fmt.Printf("TODO handle tick %v", t)
	}
}
