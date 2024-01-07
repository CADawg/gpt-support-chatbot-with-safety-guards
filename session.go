package main

import (
	"sync"
	"time"
)

var Sessions = newSessions()

func cleaner() {
	// every hour, go through all the sessions and all that are more than a day old, delete them
	for {
		time.Sleep(1 * time.Hour)
		Sessions.Locker.Lock()
		for k, v := range Sessions.Sessions {
			if time.Now().Unix()-v.LastMessageTime > 86400 {
				delete(Sessions.Sessions, k)
			}
		}
		Sessions.Locker.Unlock()
	}
}

func init() {
	go cleaner()
}

type TSessions struct {
	Sessions map[string]TSession
	Locker   sync.RWMutex
}

func newSessions() *TSessions {
	return &TSessions{
		Sessions: make(map[string]TSession),
		Locker:   sync.RWMutex{},
	}
}

type TSession struct {
	History         []PublicMessage
	LastMessageTime int64
}
