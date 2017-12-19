// Copyright 2017 New Vector Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import "sync"

// Linearizer allows different goroutines to serialize execution of functions
// based on a string key.
type Linearizer struct {
	// protects lastMutex
	mutex sync.Mutex
	//
	lastMutex map[string]<-chan struct{}
}

// NewLinearizer creates a new Linearizer
func NewLinearizer() Linearizer {
	return Linearizer{
		lastMutex: make(map[string]<-chan struct{}),
	}
}

// Await schedules the callback to run once all previous callbacks for the given
// key have finished executing, returning once callback has completed.
func (l *Linearizer) Await(key string, callback func()) {
	l.AwaitWithHook(key, callback, nil)
}

// AwaitWithHook is the same as Await, but with an added hook channel gets
// closed once the callback has been scheduled. This is mainly useful for
// testing as any functions scheduled after hook has been closed are guaranteed
// to be run after this callback has finished. If hook is nil then it is
// ignored.
func (l *Linearizer) AwaitWithHook(key string, callback func(), hook chan<- struct{}) {
	closeChannel := make(chan struct{})
	defer close(closeChannel)

	awaitChannel := l.getAndSetLastMutex(key, closeChannel)

	if hook != nil {
		close(hook)
	}

	if awaitChannel != nil {
		<-awaitChannel
	}

	callback()

	l.cleanupKey(key, closeChannel)
}

// NumberOfActiveKeys returns the number of keys that have callbacks currently
// scheduled to be run (or are running). Used mostly in tests to ensure that map
// entries are cleaned up.
func (l *Linearizer) NumberOfActiveKeys() int {
	return len(l.lastMutex)
}

// getAndSetLastMutex replaces the current entry in lastMutex with the given
// channel, while locking the mutex. The existing entry is returned if it
// exists, otherwise returns nil.
func (l *Linearizer) getAndSetLastMutex(key string, closeChannel <-chan struct{}) <-chan struct{} {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	awaitChannel := l.lastMutex[key]
	l.lastMutex[key] = closeChannel

	return awaitChannel
}

// cleanupKey deletes the entry in the lastMutex map if the value of key in the
// map is currentChannel. If they match then its safe to delete because all
// scheduled callbacks have been run for that key.
func (l *Linearizer) cleanupKey(key string, currentChannel <-chan struct{}) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	entry, ok := l.lastMutex[key]
	if ok && entry == currentChannel {
		delete(l.lastMutex, key)
	}
}
