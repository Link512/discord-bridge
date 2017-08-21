package main

import (
	"github.com/Link512/discord-bridge"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	token := os.Getenv("CHANNEL_BRIDGE_TOKEN")
	if token == "" {
		fmt.Println("Please set CHANNEL_BRIDGE_TOKEN environment variable with the token")
		return
	}
	bm, err := discord_bridge.NewBotManager(token)
	if err != nil {
		fmt.Println("Couldn't initialize BotManager:" + err.Error())
		return
	}

	err = bm.Start()
	if err != nil {
		fmt.Println("Couldn't start BotManager: " + err.Error())
		return
	}
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	bm.Stop()
}
