// The bot instance that will be present on each server
package discord_bridge

import (
	"github.com/bwmarrin/discordgo"
)

type voicePacket struct {
	botID  string
	Packet *discordgo.Packet
}

type botChannel chan voicePacket

type botInstance struct {
	ID              string
	Join            chan botChannel
	Leave           chan botChannel
	Send            botChannel
	voiceConnection *discordgo.VoiceConnection
	recv            botChannel
	stop            chan bool
}

func NewBotInstance(id string,
	vc *discordgo.VoiceConnection,
	joinChan chan botChannel,
	leaveChan chan botChannel,
	sendChan botChannel) *botInstance {

	return &botInstance{
		ID:              id,
		Join:            joinChan,
		Leave:           leaveChan,
		Send:            sendChan,
		voiceConnection: vc,
		stop:            make(chan bool),
	}
}

func (b *botInstance) Stop() {

	close(b.stop)
	if b.voiceConnection != nil {
		b.voiceConnection.Disconnect()
	}
}

func (b *botInstance) Listen() {

	go b.receive()
	b.recv = make(chan voicePacket)
	b.Join <- b.recv
	for {
		select {
		case packet, ok := <-b.voiceConnection.OpusRecv:
			if !ok {
				continue
			}
			b.Send <- voicePacket{b.ID, packet}
		case <-b.stop:
			b.Leave <- b.recv
			close(b.recv)
			return
		}
	}
}

func (b *botInstance) receive() {

	for packet := range b.recv {
		if packet.botID == b.ID {
			continue
		}
		b.voiceConnection.OpusSend <- packet.Packet.Opus
	}
}
