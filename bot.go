// The bot instance that will be present on each server
package discord_bridge

import (
	"github.com/bwmarrin/discordgo"
)

type voicePacket struct {
	GuildID string
	Packet  *discordgo.Packet
}

type botInstance struct {
	GuildID         string
	voiceConnection *discordgo.VoiceConnection
	Recv            chan voicePacket
	stop            chan bool
}

func NewBotInstance(guildID string, vc *discordgo.VoiceConnection, recv chan voicePacket) *botInstance {

	return &botInstance{
		GuildID:         guildID,
		voiceConnection: vc,
		stop:            make(chan bool),
		Recv:            recv,
	}
}

func (b *botInstance) Stop() {

	close(b.stop)
	if b.voiceConnection != nil {
		b.voiceConnection.Disconnect()
	}
}

func (b *botInstance) Listen() {

	for {
		select {
		case packet, ok := <-b.voiceConnection.OpusRecv:
			if !ok {
				continue
			}
			b.Recv <- voicePacket{b.GuildID, packet}
		case <-b.stop:
			return
		}
	}
}

func (b *botInstance) Broadcast() {

	for {
		select {
		case packet := <-b.Recv:
			if packet.GuildID == b.GuildID {
				continue
			}
			b.voiceConnection.OpusSend <- packet.Packet.Opus
		case <-b.stop:
			return
		}
	}
}
