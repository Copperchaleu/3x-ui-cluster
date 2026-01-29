package main

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

func main() {
	godotenv.Load()
	
	err := database.InitDB(config.GetDBPath())
	if err != nil {
		log.Fatal(err)
	}

	nodeService := &service.NodeService{}
	
	// Try to push config to node 2 (agent1)
	fmt.Println("Attempting to push config to node 2 (agent1)...")
	err = nodeService.PushConfig(2)
	if err != nil {
		fmt.Printf("Failed to push config: %v\n", err)
		fmt.Println("\nPossible reasons:")
		fmt.Println("1. Node 2 is not connected via WebSocket")
		fmt.Println("2. No WebSocket connection exists for this node")
		fmt.Println("3. Master server needs to be running")
	} else {
		fmt.Println("Config pushed successfully!")
	}
}
