// The main bot logic
package discord_bridge

import (
	"fmt"
	"github.com/Link512/gouuid"
	"github.com/bwmarrin/discordgo"
	"sync"
)

type session struct {
	Send      chan voicePacket
	Instances []*botInstance
}

type botManager struct {
	sessions    map[string]*sessionListener
	sessionLock sync.RWMutex
	discord     *discordgo.Session
}

func newBotManager(discordSession *discordgo.Session) *botManager {

	bm := &botManager{
		sessions:    make(map[string]*sessionListener),
		sessionLock: sync.RWMutex{},
		discord:     discordSession,
	}
	return bm
}

func (bm *botManager) Start() error {

	err := bm.discord.Open()
	if err != nil {
		return fmt.Errorf("couldn't open discord session")
	}
	return nil
}

func (bm *botManager) Stop() {

	for _, session := range bm.sessions {
		session.Stop()
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
	uuidStr := uuid.StringNoSeparator()

	if _, ok := bm.sessions[uuidStr]; ok == true {
		discordSession.ChannelMessageSend(channelID, "Generated unique id already exists. PANIC")
		return
	}
	bm.sessionLock.Lock()
	bm.sessions[uuidStr] = NewSessionListener()
	bm.sessionLock.Unlock()
	discordSession.ChannelMessageSend(channelID, "Session id generated: "+uuidStr)
}

func (bm *botManager) connect(sessionID string, channelID string, session *discordgo.Session, author *discordgo.User) {

	bm.sessionLock.Lock()
	defer bm.sessionLock.Unlock()
	if _, ok := bm.sessions[sessionID]; ok == false {
		session.ChannelMessageSend(channelID, "SessionID not found")
		return
	}

	channel, err := session.Channel(channelID)
	if err != nil {
		session.ChannelMessageSend(channelID, "Internal error")
		return
	}

	voiceConn := getVoiceConnection(session, channel, author.ID)
	if voiceConn == nil {
		session.ChannelMessageSend(channelID, "Must be in voice channel to bridge")
		return
	}

	bm.sessions[sessionID].AddBotInstance(channel.GuildID, voiceConn)
}

func (bm *botManager) disconnect(sessionID string, channelID string, session *discordgo.Session) {

	if _, ok := bm.sessions[sessionID]; ok == false {
		session.ChannelMessageSend(channelID, "SessionID not found")
		return
	}

	channel, err := session.Channel(channelID)
	if err != nil {
		session.ChannelMessageSend(channelID, "Internal error")
		return
	}

	bm.sessionLock.Lock()
	defer bm.sessionLock.Unlock()
	if err = bm.sessions[sessionID].RemoveBotInstance(channel.GuildID); err != nil {
		session.ChannelMessageSend(channelID, err.Error())
		return
	}

	if bm.sessions[sessionID].Empty() {
		session.ChannelMessageSend(channelID, "This was the last client in the session."+
			"Terminating session. You must create a new session if you want to use the bot again.")
		delete(bm.sessions, sessionID)
	}
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
