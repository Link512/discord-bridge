// The main bot logic
package discord_bridge

import (
	"github.com/bwmarrin/discordgo"
	"strings"
	"github.com/nu7hatch/gouuid"
)

type BotManagerError struct {
	msg string
}

func (e BotManagerError) Error() string {

	return e.msg
}

type BotManager struct {

	token string
	bots []*BotInstance
	commandHandlers map[string]func ([]string, string, *discordgo.Session, *BotManager)
	sessions map[string]*BotInstance
	discord *discordgo.Session
}

func NewBotManager(token string) (bm *BotManager, err error) {

	if token == "" {
		return nil, BotManagerError{"Token can't be empty"}
	}

	bm = &BotManager{
		token: token,
		bots: make([]*BotInstance, 2),
		commandHandlers: map[string]func ([]string, string, *discordgo.Session, *BotManager){
			"help": help,
			"generate": generate,
		},
		sessions: make(map[string]*BotInstance),
	}

	discordSession, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, BotManagerError{"Couldn't initialize BotManager: " + err.Error()}
	}
	bm.discord = discordSession

	bm.discord.AddHandler(bm.guildCreate)
	bm.discord.AddHandler(bm.messageCreated)

	return bm, nil
}

func (bm *BotManager) Start() error{

	err := bm.discord.Open()
	if err != nil {
		return BotManagerError{"Couldn't open session"}
	}
	return nil
}

func (bm *BotManager) Stop() {

	if bm.discord != nil {
		bm.discord.Close()
	}
}

func (bm *BotManager) messageCreated(s *discordgo.Session, message *discordgo.MessageCreate) {

	if message.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(message.Content, "$$") {
		fields := strings.Fields(message.Content)
		cmd := strings.TrimPrefix(fields[0], "$$")
		command, ok := bm.commandHandlers[cmd]
		if ok == false {
			bm.discord.ChannelMessageSend(message.ChannelID, "Invalid command. Type $$help for a list of commands")
			return
		}
		command(fields[1:], message.ChannelID, s, bm)
	}
}

func (bm *BotManager) guildCreate(session *discordgo.Session, event *discordgo.GuildCreate) {

	if event.Guild.Unavailable {
		return
	}

	for _, channel := range event.Guild.Channels {
		if strings.Contains(channel.Name, "general") {
			_, _ = session.ChannelMessageSend(channel.ID, "Server Bridge is ready. Type $$help to see the list of commands")
		}
	}
}

func help(_ []string, channelID string, session *discordgo.Session, _ *BotManager) {

	msg := "List of commands: \n" +
		"$$help - prints this message\n" +
		"$$generate - Generates a unique ID to be used when bridging\n" +
		"$$connect <UID> - Connects to the specified session and begins the bridge\n" +
		"$$disconnect - Disconnects from session"
	session.ChannelMessageSend(channelID, msg)
}

func generate(_ []string, channelID string, session *discordgo.Session, bm *BotManager) {

	if bm == nil {
		session.ChannelMessageSend(channelID, "PANIC")
		return
	}
	uuid, err := uuid.NewV4()
	if err != nil {
		session.ChannelMessageSend(channelID, "There was an error generating the unique id")
		return
	}
	uuid_str := uuid.String()
	_, ok := bm.sessions[uuid_str]
	if ok == true {
		session.ChannelMessageSend(channelID, "Generated unique id already exists. PANIC")
		return
	}
	bm.sessions[uuid_str] = nil
	session.ChannelMessageSend(channelID, "Session id generated: " + uuid_str)
}
