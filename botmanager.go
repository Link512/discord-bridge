// The main bot logic
package discord_bridge

import (
	"sync"
	"strings"
	"github.com/bwmarrin/discordgo"
	"github.com/Link512/gouuid"
	"reflect"
)

type BotManagerError struct {
	msg string
}

func (e BotManagerError) Error() string {

	return e.msg
}

type session struct {
	Send      chan voicePacket
	Instances []*botInstance
}

type BotManager struct {
	token           string
	commandHandlers map[string]func([]string, string, *discordgo.Session, *BotManager, *discordgo.User)
	sessions        map[string]session
	sessionLocks    map[string]*sync.RWMutex
	discord         *discordgo.Session
}

func NewBotManager(token string) (bm *BotManager, err error) {

	if token == "" {
		return nil, BotManagerError{"Token can't be empty"}
	}

	bm = &BotManager{
		token: token,
		commandHandlers: map[string]func([]string, string, *discordgo.Session, *BotManager, *discordgo.User){
			"help":       help,
			"start":      generate,
			"connect":    connect,
			"disconnect": disconnect,
		},
		sessions:     make(map[string]session),
		sessionLocks: make(map[string]*sync.RWMutex),
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

func (bm *BotManager) Start() error {

	err := bm.discord.Open()
	if err != nil {
		return BotManagerError{"Couldn't open session"}
	}
	return nil
}

func (bm *BotManager) Stop() {

	for _, session := range bm.sessions {
		for _, botInstance := range session.Instances {
			botInstance.Stop()
		}
	}
	if bm.discord != nil {
		bm.discord.Close()
	}
}

func (bm *BotManager) messageCreated(session *discordgo.Session, message *discordgo.MessageCreate) {

	if message.Author.ID == session.State.User.ID {
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
		command(fields[1:], message.ChannelID, session, bm, message.Author)
	}
}

func (bm *BotManager) guildCreate(session *discordgo.Session, event *discordgo.GuildCreate) {

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

func help(_ []string, channelID string, session *discordgo.Session, _ *BotManager, _ *discordgo.User) {

	msg := "List of commands: \n" +
		"$$help - prints this message\n" +
		"$$start - Generates a unique session ID to be used when bridging\n" +
		"$$connect <UID> - Connects to the specified session and begins the bridge\n" +
		"$$disconnect - Disconnects from session"
	session.ChannelMessageSend(channelID, msg)
}

func generate(_ []string, channelID string, discordSession *discordgo.Session, bm *BotManager, _ *discordgo.User) {

	if bm == nil {
		discordSession.ChannelMessageSend(channelID, "Something went really wrong. Hide yo kids")
		return
	}
	uuid, err := uuid.NewV4()
	if err != nil {
		discordSession.ChannelMessageSend(channelID, "There was an error generating the unique id")
		return
	}
	uuid_str := uuid.StringNoSeparator()

	if _, ok := bm.sessions[uuid_str]; ok == true {
		discordSession.ChannelMessageSend(channelID, "Generated unique id already exists. PANIC")
		return
	}
	bm.sessions[uuid_str] = session{make(chan voicePacket), nil}
	bm.sessionLocks[uuid_str] = &sync.RWMutex{}
	discordSession.ChannelMessageSend(channelID, "Session id generated: "+uuid_str)
}

func connect(params []string, channelID string, session *discordgo.Session, bm *BotManager, author *discordgo.User) {

	if len(params) == 0 {
		session.ChannelMessageSend(channelID, "Must provide session id with $$connect")
		return
	}

	sessionId := params[0]

	if _, ok := bm.sessions[sessionId]; ok == false {
		session.ChannelMessageSend(channelID, "SessionID not found")
		return
	}

	sessionsLock, ok := bm.sessionLocks[sessionId]
	if ok == false {
		session.ChannelMessageSend(channelID, "SessionID not found")
		return
	}

	channel, err := session.Channel(channelID)
	if err != nil {
		return
	}

	voiceConn := getVoiceConnection(session, channel, author.ID)
	if voiceConn == nil {
		session.ChannelMessageSend(channelID, "Must be in voice channel to bridge")
		return
	}

	botInst := NewBotInstance(channel.GuildID, voiceConn, nil)

	for success := false; success == false; {

		sessionsLock.RLock()
		botSession := bm.sessions[sessionId]
		sessionsLock.RUnlock()

		_, found := findChannel(botSession.Instances, channel.GuildID)

		if found == false {
			sessionsLock.Lock()
			tmpSessions := bm.sessions[sessionId].Instances

			if reflect.DeepEqual(botSession.Instances, tmpSessions) == false {
				sessionsLock.Unlock()
				continue
			}
			botSession.Instances = append(botSession.Instances, botInst)

			botInst.Recv = botSession.Send
			bm.sessions[sessionId] = botSession
			sessionsLock.Unlock()

			go botInst.Broadcast()
			go botInst.Listen()
		}
		success = true
	}
}

func disconnect(params []string, channelID string, session *discordgo.Session, bm *BotManager, _ *discordgo.User) {

	if len(params) == 0 {
		session.ChannelMessageSend(channelID, "Must provide session id with $$disconnect")
		return
	}
	sessionId := params[0]

	if _, ok := bm.sessions[sessionId]; ok == false {
		session.ChannelMessageSend(channelID, "SessionID not found")
		return
	}

	sessionsLock, ok := bm.sessionLocks[sessionId]
	if ok == false {
		session.ChannelMessageSend(channelID, "SessionID not found")
		return
	}

	channel, err := session.Channel(channelID)
	if err != nil {
		return
	}

	for success := false; success == false; {

		sessionsLock.RLock()
		botSession := bm.sessions[sessionId]
		sessionsLock.RUnlock()

		sessionIdx, found := findChannel(botSession.Instances, channel.GuildID)

		if found == false {
			session.ChannelMessageSend(channelID, "Must be a part of session to disconnect from it")
			break
		}

		session.ChannelMessageSend(channelID, "Disconnected client from voice channel")

		sessionsLock.Lock()
		tmpSessions := bm.sessions[sessionId].Instances

		if reflect.DeepEqual(tmpSessions, botSession.Instances) == false {
			sessionsLock.Unlock()
			continue
		}
		botSession.Instances[sessionIdx].Stop()

		botSession.Instances = append(botSession.Instances[:sessionIdx], botSession.Instances[sessionIdx+1:]...)
		if len(botSession.Instances) == 0 {
			session.ChannelMessageSend(channelID, "This was the last client in the session."+
				"Terminating session. You must create a new session if you want to use the bot again.")
			delete(bm.sessions, sessionId)
			delete(bm.sessionLocks, sessionId)
		} else {
			bm.sessions[sessionId] = botSession
		}
		sessionsLock.Unlock()
		success = true
	}
}

func findChannel(instances []*botInstance, guildId string) (int, bool) {

	for i, session := range instances {
		if session.GuildID == guildId {
			return i, true
		}
	}
	return -1, false
}

func getVoiceConnection(session *discordgo.Session, channel *discordgo.Channel, userId string) *discordgo.VoiceConnection {

	guild, err := session.State.Guild(channel.GuildID)
	if err != nil {
		return nil
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == userId {
			if result, err := session.ChannelVoiceJoin(guild.ID, vs.ChannelID, false, false); err == nil {
				return result
			}
			return nil
		}
	}
	return nil
}
