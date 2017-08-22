// The main Bot structure
package discord_bridge

import (
	"github.com/bwmarrin/discordgo"
)

type BotInstance struct {

	GuildID string
	VoiceConnection *discordgo.VoiceConnection
	stop chan bool
}

func NewBotInstance(guildID string, vc *discordgo.VoiceConnection) *BotInstance  {

	return &BotInstance{
		GuildID: guildID,
		VoiceConnection: vc,
		stop: make(chan bool, 1),
	}
}

func (b *BotInstance) Stop() {

	b.stop <- true
	if b.VoiceConnection != nil {
		b.VoiceConnection.Disconnect()
	}
}
