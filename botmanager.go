// The main bot logic
package discord_bridge

import (
	"sync"
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

type botManager struct {
	sessions     map[string]session
	sessionLocks map[string]*sync.RWMutex
	discord      *discordgo.Session
}

func newBotManager(discordSession *discordgo.Session) *botManager {

	bm := &botManager{
		sessions:     make(map[string]session),
		sessionLocks: make(map[string]*sync.RWMutex),
		discord:      discordSession,
	}

	return bm
}

func (bm *botManager) Start() error {

	err := bm.discord.Open()
	if err != nil {
		return BotManagerError{"Couldn't open session"}
	}
	return nil
}

func (bm *botManager) Stop() {

	for _, session := range bm.sessions {
		for _, botInstance := range session.Instances {
			botInstance.Stop()
		}
	}
	if bm.discord != nil {
		bm.discord.Close()
	}
}

func (bm *botManager) generate(channelID string, discordSession *discordgo.Session) {

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

func (bm *botManager) connect(sessionID string, channelID string, session *discordgo.Session, author *discordgo.User) {

	if _, ok := bm.sessions[sessionID]; ok == false {
		session.ChannelMessageSend(channelID, "SessionID not found")
		return
	}

	sessionsLock, ok := bm.sessionLocks[sessionID]
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
		botSession := bm.sessions[sessionID]
		sessionsLock.RUnlock()

		_, found := findChannel(botSession.Instances, channel.GuildID)

		if found == false {
			sessionsLock.Lock()
			tmpSessions := bm.sessions[sessionID].Instances

			if reflect.DeepEqual(botSession.Instances, tmpSessions) == false {
				sessionsLock.Unlock()
				continue
			}
			botSession.Instances = append(botSession.Instances, botInst)

			botInst.Recv = botSession.Send
			bm.sessions[sessionID] = botSession
			sessionsLock.Unlock()

			go botInst.Broadcast()
			go botInst.Listen()
		}
		success = true
	}
}

func (bm *botManager) disconnect(sessionID string, channelID string, session *discordgo.Session) {

	if _, ok := bm.sessions[sessionID]; ok == false {
		session.ChannelMessageSend(channelID, "SessionID not found")
		return
	}

	sessionsLock, ok := bm.sessionLocks[sessionID]
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
		botSession := bm.sessions[sessionID]
		sessionsLock.RUnlock()

		sessionIdx, found := findChannel(botSession.Instances, channel.GuildID)

		if found == false {
			session.ChannelMessageSend(channelID, "Must be a part of session to disconnect from it")
			break
		}

		session.ChannelMessageSend(channelID, "Disconnected client from voice channel")

		sessionsLock.Lock()
		tmpSessions := bm.sessions[sessionID].Instances

		if reflect.DeepEqual(tmpSessions, botSession.Instances) == false {
			sessionsLock.Unlock()
			continue
		}
		botSession.Instances[sessionIdx].Stop()

		botSession.Instances = append(botSession.Instances[:sessionIdx], botSession.Instances[sessionIdx+1:]...)
		if len(botSession.Instances) == 0 {
			session.ChannelMessageSend(channelID, "This was the last client in the session."+
				"Terminating session. You must create a new session if you want to use the bot again.")
			delete(bm.sessions, sessionID)
			delete(bm.sessionLocks, sessionID)
		} else {
			bm.sessions[sessionID] = botSession
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
