// Listener for each bot sessio. It receives voice packets and broadcasts them to all the
// bot instances
package discord_bridge

import (
	"github.com/bwmarrin/discordgo"
	"sync"
	"fmt"
)

type sessionListener struct {
	stop          chan bool
	join          chan botChannel
	leave         chan botChannel
	incoming      chan voicePacket
	botChannels   map[botChannel]bool
	instancesLock sync.Mutex
	botInstances  []*botInstance
	wait          sync.WaitGroup
}

func NewSessionListener() *sessionListener {

	sl := &sessionListener{
		make(chan bool),
		make(chan botChannel),
		make(chan botChannel),
		make(chan voicePacket),
		make(map[botChannel]bool),
		sync.Mutex{},
		make([]*botInstance, 0),
		sync.WaitGroup{},
	}
	go sl.listen()
	return sl
}

func (sl *sessionListener) AddBotInstance(botID string, voiceConn *discordgo.VoiceConnection) {

	b := NewBotInstance(botID, voiceConn, sl.join, sl.leave, sl.incoming)
	sl.instancesLock.Lock()
	defer sl.instancesLock.Unlock()
	go b.Listen()
	sl.botInstances = append(sl.botInstances, b)
}

func (sl *sessionListener) RemoveBotInstance(botID string) error {

	sl.instancesLock.Lock()
	defer sl.instancesLock.Unlock()
	idx := -1
	for i := range sl.botInstances {
		if sl.botInstances[i].ID == botID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("bot id not found")
	}
	sl.botInstances[idx].Stop()
	sl.botInstances = append(sl.botInstances[:idx], sl.botInstances[idx+1:]...)
	return nil
}

func (sl *sessionListener) Empty() bool {
	sl.instancesLock.Lock()
	defer sl.instancesLock.Unlock()
	return len(sl.botInstances) == 0
}

func (sl *sessionListener) Stop() {

	sl.instancesLock.Lock()
	for _, inst := range sl.botInstances {
		inst.Stop()
	}
	sl.instancesLock.Unlock()
	sl.wait.Wait()
	close(sl.stop)
}

func (sl *sessionListener) listen() {

	for {
		select {
		case p := <-sl.incoming:
			{
				for ch := range sl.botChannels {
					ch <- p
				}
			}
		case ch := <-sl.join:
			{
				sl.wait.Add(1)
				sl.botChannels[ch] = true
			}
		case ch := <-sl.leave:
			{
				sl.wait.Done()
				delete(sl.botChannels, ch)
			}
		case <-sl.stop:
			return
		}
	}
}
