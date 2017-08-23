// The main bot logic
package discord_bridge

import (
	"github.com/bwmarrin/discordgo"
	"strings"
	"github.com/nu7hatch/gouuid"
	"sync"
)

type BotManagerError struct {

	msg string
}

func (e BotManagerError) Error() string {

	return e.msg
}

type BotManager struct {

	token string
	commandHandlers map[string]func ([]string, string, *discordgo.Session, *BotManager, *discordgo.User)
	sessions map[string][]*BotInstance
	sessionLocks map[string]*sync.RWMutex
	discord *discordgo.Session
}

func NewBotManager(token string) (bm *BotManager, err error) {

	if token == "" {
		return nil, BotManagerError{"Token can't be empty"}
	}

	bm = &BotManager{
		token: token,
		commandHandlers: map[string]func ([]string, string, *discordgo.Session, *BotManager, *discordgo.User){
			"help": help,
			"start": generate,
			"connect": connect,
			"disconnect": disconnect,
		},
		sessions: make(map[string][]*BotInstance),
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

func (bm *BotManager) Start() error{

	err := bm.discord.Open()
	if err != nil {
		return BotManagerError{"Couldn't open session"}
	}
	return nil
}

func (bm *BotManager) Stop() {

	for _, sessions := range bm.sessions {
		for _, session := range sessions {
			session.Stop()
		}
	}
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
		command(fields[1:], message.ChannelID, s, bm, message.Author)
	}
}

func (bm *BotManager) guildCreate(session *discordgo.Session, event *discordgo.GuildCreate) {

	if event.Guild.Unavailable {
		return
	}

	for _, channel := range event.Guild.Channels {
		if strings.Contains(channel.Name, "general") {
			session.ChannelMessageSend(channel.ID, "Server Bridge is ready. Type $$help to see the list of commands")
		}
	}
}

func (bm *BotManager) listen(b *BotInstance, sessionID string) {

	lock := bm.sessionLocks[sessionID]
	for {
		select {
		case packet, ok := <-b.VoiceConnection.OpusRecv:
			if ok == false {
				continue
			}

			lock.RLock()
			siblings := make([]*BotInstance, len(bm.sessions[sessionID]))
			copy(siblings, bm.sessions[sessionID])
			lock.RUnlock()

			for _, s := range siblings {
				if s.GuildID != b.GuildID {
					s.VoiceConnection.Speaking(true)
					s.VoiceConnection.OpusSend <- packet.Opus
					s.VoiceConnection.Speaking(false)

				}
			}
		case <- b.stop:
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

func generate(_ []string, channelID string, session *discordgo.Session, bm *BotManager, _ *discordgo.User) {

	if bm == nil {
		session.ChannelMessageSend(channelID, "Something went really wrong. Hide yo kids")
		return
	}
	uuid, err := uuid.NewV4()
	if err != nil {
		session.ChannelMessageSend(channelID, "There was an error generating the unique id")
		return
	}
	uuid_str := uuid.String()
	uuid_str = uuid_str[:4]
	_, ok := bm.sessions[uuid_str]
	if ok == true {
		session.ChannelMessageSend(channelID, "Generated unique id already exists. PANIC")
		return
	}
	bm.sessions[uuid_str] = nil
	bm.sessionLocks[uuid_str] = &sync.RWMutex{}
	session.ChannelMessageSend(channelID, "Session id generated: " + uuid_str)
}

func connect(params []string, channelID string, session *discordgo.Session, bm *BotManager, author *discordgo.User) {

	if len(params) == 0 {
		session.ChannelMessageSend(channelID, "Must provide session id with $$connect")
		return
	}

	sessionId := params[0]

	_, ok := bm.sessions[sessionId]

	if ok == false {
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

	botInst := NewBotInstance(channel.GuildID, voiceConn)

	for success := false ; success == false ; {

		sessionsLock.RLock()
		sessions := bm.sessions[sessionId]
		sessionsLock.RUnlock()

		_, found := findChannel(sessions, channel.GuildID)

		if found == false {
			sessions = append(sessions, botInst)

			sessionsLock.Lock()
			tmpSessions := bm.sessions[sessionId]

			if compare(sessions, tmpSessions) == false {
				sessionsLock.Unlock()
				continue
			}
			bm.sessions[sessionId] = sessions
			sessionsLock.Unlock()

			go bm.listen(botInst, sessionId)
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

	_, ok := bm.sessions[sessionId]
	if ok == false {
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

	for success := false ; success == false ; {

		sessionsLock.RLock()
		sessions := bm.sessions[sessionId]
		sessionsLock.RUnlock()

		sessionIdx, found := findChannel(sessions, channel.GuildID)

		if found == true {

			session.ChannelMessageSend(channelID, "Disconnected client from voice channel")

			sessionsLock.Lock()
			tmpSessions := bm.sessions[sessionId]

			if compare(tmpSessions, sessions) == false {
				sessionsLock.Unlock()
				continue
			}
			sessions[sessionIdx].Stop()

			sessions = append(sessions[:sessionIdx], sessions[sessionIdx+1:]...)
			if len(sessions) == 0 {
				session.ChannelMessageSend(channelID, "This was the last client in the session."+
					"Terminating session. You must create a new session if you want to use the bot again.")
				delete(bm.sessions, sessionId)
				delete(bm.sessionLocks, sessionId)
			} else {
				bm.sessions[sessionId] = sessions
			}
			sessionsLock.Unlock()
		} else {
			session.ChannelMessageSend(channelID, "Must be a part of session to disconnect from it")
		}
		success = true
	}
}

func compare(lhs, rhs []*BotInstance) bool{

	if (lhs == nil) != (rhs == nil) {
		return false
	}

	if len(lhs) != len(rhs) {
		return false
	}

	occurrences := make(map[string]int)

	for _, inst := range lhs {
		occurrences[inst.GuildID]++
	}

	for _, inst := range rhs {

		if occurrences[inst.GuildID] > 0 {
			occurrences[inst.GuildID]--
			continue
		}
		return false
	}
	return true

}

func findChannel(instances []*BotInstance, guildId string) (int, bool) {

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
			result, err := session.ChannelVoiceJoin(guild.ID, vs.ChannelID, false, false)
			if err != nil {
				return nil
			}
			return result
		}
	}
	return nil
}