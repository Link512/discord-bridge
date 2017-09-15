// Handler for discord events
package discord_bridge

import (
	"github.com/bwmarrin/discordgo"
	"fmt"
	"strings"
)

const (
	helpMsg = "List of commands: \n" +
		"$$help - prints this message\n" +
		"$$start - Generates a unique session ID to be used when bridging\n" +
		"$$connect <UID> - Connects to the specified session and begins the bridge\n" +
		"$$disconnect - Disconnects from session"
)

type Bridge struct {
	discordSession *discordgo.Session
	botManager     *botManager
}

func NewBridge(token string) (*Bridge, error) {

	if token == "" {
		return nil, fmt.Errorf("token can't be empty")
	}

	if discordSession, err := discordgo.New("Bot " + token); err == nil {
		botManager := newBotManager(discordSession)
		discordSession.AddHandler(guildCreate)
		bridge := Bridge{
			discordSession,
			botManager,
		}
		discordSession.AddHandler(bridge.messageCreated)
		return &bridge, nil
	}
	return nil, fmt.Errorf("could not initialize discord session")
}

func (b *Bridge) Start() error {

	return b.botManager.Start()
}

func (b *Bridge) Stop() {

	b.botManager.Stop()
}

func (b *Bridge) messageCreated(session *discordgo.Session, message *discordgo.MessageCreate) {

	if message.Author.ID == session.State.User.ID {
		return
	}

	if !strings.HasPrefix(message.Content, "$$") {
		return
	}
	fields := strings.Fields(message.Content)
	cmd := strings.TrimPrefix(fields[0], "$$")

	switch cmd {

	case "help":
		session.ChannelMessageSend(message.ChannelID, helpMsg)
	case "start":
		b.botManager.generate(message.ChannelID, session)
	case "connect":
		if len(fields) != 2 {
			session.ChannelMessageSend(message.ChannelID, "Must provide session id with $$connect")
			return
		}
		b.botManager.connect(fields[1], message.ChannelID, session, message.Author)
	case "disconnect":
		if len(fields) != 2 {
			session.ChannelMessageSend(message.ChannelID, "Must provide session id with $$disconnect")
			return
		}
		b.botManager.disconnect(fields[1], message.ChannelID, session)
	default:
		b.discordSession.ChannelMessageSend(message.ChannelID, "Invalid command. Type $$help for a list of commands")
	}
}

func guildCreate(session *discordgo.Session, event *discordgo.GuildCreate) {

	if event.Guild.Unavailable {
		return
	}

	for _, channel := range event.Guild.Channels {
		if strings.Contains(channel.Name, "general") {
			session.ChannelMessageSend(channel.ID, "Server Bridge is ready. Type $$help to see the list of commands")
			return
		}
	}
}
