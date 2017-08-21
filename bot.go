// The main Bot structure
package discord_bridge

import (
	"github.com/bwmarrin/discordgo"
	"sync"
)

type BotInstance struct {

	in chan *discordgo.Packet
	out chan []byte
	stop chan bool
	siblings []*BotInstance
	lock *sync.RWMutex
}

func NewBotInstance(in chan *discordgo.Packet, out chan []byte) *BotInstance  {

	return &BotInstance{
		in: in,
		out: out,
		stop: make(chan bool, 1),
		siblings: make([]*BotInstance, 2),
		lock: &sync.RWMutex{},
	}
}

func (b *BotInstance) AddToChain(elem *BotInstance) {

	b.lock.Lock()
	for _, s := range b.siblings {
		elem.siblings = append(elem.siblings, s.siblings...)
		s.siblings = append(s.siblings, elem)
	}
	b.siblings = append(b.siblings, elem)
	elem.siblings = append(elem.siblings, b)

	b.lock.Unlock()
	elem.lock = b.lock
	elem.Start()
}

func (b *BotInstance) Start() {

	go func(b *BotInstance) {

		for {
			select {
			case packet, ok := <-b.in:
				if ok == false {
					continue
				}

				b.lock.RLock()
				siblings := make([]*BotInstance, len(b.siblings))
				copy(siblings, b.siblings)
				b.lock.RUnlock()

				for _, s := range siblings {
					s.out <- packet.Opus
				}
			case <- b.stop:
				return
			}
		}
	}(b)
}

func (b *BotInstance) Stop() {

	b.stop <- true
}
