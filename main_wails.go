package main

import (
	"GusSync/app"
	"log"
	"os"
	"time"
)

func main() {
	mainStartTime := time.Now()
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logger := log.New(os.Stderr, "[GusSync] ", log.LstdFlags|log.Lshortfile)
	logger.Printf("[TIMING %s] [MAIN] ⭐ PROGRAM START ⭐ - main() function entered", timestamp)
	
	if err := app.Run(); err != nil {
		mainDuration := time.Since(mainStartTime)
		logger.Printf("[TIMING %s] [MAIN] App.Run() returned ERROR after %v: %v", time.Now().Format("2006-01-02 15:04:05.000"), mainDuration, err)
		panic(err)
	}
	
	mainDuration := time.Since(mainStartTime)
	logger.Printf("[TIMING %s] [MAIN] ⭐ PROGRAM EXIT ⭐ - main() exiting after %v", time.Now().Format("2006-01-02 15:04:05.000"), mainDuration)
}

